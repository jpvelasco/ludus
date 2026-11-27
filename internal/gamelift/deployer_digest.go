package gamelift

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
)

// ecrHostPattern matches an ECR registry host and captures its account ID.
var ecrHostPattern = regexp.MustCompile(`^(\d{12})\.dkr\.ecr(?:-fips)?\.[a-z0-9-]+\.amazonaws\.com(?:\.cn)?$`)

// imageRef is an ECR image reference split into the parts DescribeImages needs.
type imageRef struct {
	registryID string
	repository string
	tag        string
	digest     string // set when the reference is pinned with @sha256:...
}

// parseImageURI splits "<acct>.dkr.ecr.<region>.amazonaws.com/<repo>[:<tag>|@<digest>]".
// An untagged reference means "latest", as it does for docker.
func parseImageURI(uri string) (imageRef, error) {
	host, path, found := strings.Cut(uri, "/")
	if !found || path == "" {
		return imageRef{}, fmt.Errorf("image URI %q is not registry/repository[:tag]", uri)
	}
	match := ecrHostPattern.FindStringSubmatch(host)
	if match == nil {
		return imageRef{}, fmt.Errorf("image URI host %q is not an ECR registry", host)
	}
	ref := imageRef{registryID: match[1]}
	if repo, digest, pinned := strings.Cut(path, "@"); pinned {
		ref.repository, ref.digest = repo, digest
		return ref, nil
	}
	ref.repository, ref.tag = path, "latest"
	if i := strings.LastIndex(path, ":"); i >= 0 {
		ref.repository, ref.tag = path[:i], path[i+1:]
	}
	return ref, nil
}

// desiredImageDigest resolves the configured image to its digest: a digest-
// pinned URI is its own answer, a tag is looked up in ECR.
func (d *Deployer) desiredImageDigest(ctx context.Context) (string, error) {
	ref, err := parseImageURI(d.opts.ImageURI)
	if err != nil {
		return "", err
	}
	if ref.digest != "" {
		return ref.digest, nil
	}
	if d.ecrClient == nil {
		return "", errors.New("no ECR client configured")
	}
	out, err := d.ecrClient.DescribeImages(ctx, &ecr.DescribeImagesInput{
		RegistryId:     aws.String(ref.registryID),
		RepositoryName: aws.String(ref.repository),
		ImageIds:       []ecrtypes.ImageIdentifier{{ImageTag: aws.String(ref.tag)}},
	})
	if err != nil {
		return "", fmt.Errorf("reading the digest of %s:%s from ECR: %w", ref.repository, ref.tag, err)
	}
	if len(out.ImageDetails) == 0 || aws.ToString(out.ImageDetails[0].ImageDigest) == "" {
		return "", fmt.Errorf("ECR has no image tagged %s in repository %s", ref.tag, ref.repository)
	}
	return aws.ToString(out.ImageDetails[0].ImageDigest), nil
}

// digestComparison is how a definition's snapshotted image compares with the
// image currently behind the tag.
type digestComparison int

const (
	digestUnknown digestComparison = iota // one side has no digest to compare
	digestSame
	digestDiffers
)

func compareDigest(def *gltypes.ContainerGroupDefinition, ecrDigest string) digestComparison {
	if def == nil || def.GameServerContainerDefinition == nil {
		return digestUnknown
	}
	snapshotted := aws.ToString(def.GameServerContainerDefinition.ResolvedImageDigest)
	switch {
	case ecrDigest == "" || snapshotted == "":
		return digestUnknown
	case snapshotted == ecrDigest:
		return digestSame
	}
	return digestDiffers
}

// definitionFit reports whether a definition already serves the desired
// image. unverified means the URI and settings match but the digest could not
// be compared, so a re-pushed tag would go unnoticed.
type definitionFit struct {
	fits       bool
	unverified bool
	reason     string // why it does not fit
}

func assessDefinition(def *gltypes.ContainerGroupDefinition, desired *gamelift.CreateContainerGroupDefinitionInput, ecrDigest string) definitionFit {
	if !definitionMatches(def, desired) {
		return definitionFit{reason: "the container group definition changed (image, SDK version, limits or ports)"}
	}
	switch compareDigest(def, ecrDigest) {
	case digestDiffers:
		return definitionFit{reason: "the image tag was re-pushed (its digest changed)"}
	case digestUnknown:
		return definitionFit{fits: true, unverified: true}
	}
	return definitionFit{fits: true}
}

// updateAction is what an existing fleet needs to reach the desired image.
type updateAction int

const (
	updateNone       updateAction = iota // the fleet already runs the desired image
	updateRollOnly                       // the latest version fits; point the fleet at it
	updateNewVersion                     // cut a new definition version, then roll
)

type updateDecision struct {
	action     updateAction
	reason     string
	unverified bool
}

// chooseUpdate decides what an existing fleet needs. "What the fleet runs" is
// judged from running (the version the fleet is on), not latest, so a failed
// rollout followed by reverting the tag does not cut a pointless new version.
func chooseUpdate(running, latest *gltypes.ContainerGroupDefinition, desired *gamelift.CreateContainerGroupDefinitionInput, ecrDigest string, force bool) updateDecision {
	if force {
		return updateDecision{action: updateNewVersion, reason: "--force was given"}
	}
	current := assessDefinition(running, desired, ecrDigest)
	if current.fits {
		return updateDecision{action: updateNone, unverified: current.unverified}
	}
	newest := assessDefinition(latest, desired, ecrDigest)
	behind := aws.ToString(latest.ContainerGroupDefinitionArn) != aws.ToString(running.ContainerGroupDefinitionArn)
	if newest.fits && behind {
		return updateDecision{
			action:     updateRollOnly,
			reason:     "the latest definition version already matches but the fleet is still on an older one",
			unverified: newest.unverified,
		}
	}
	return updateDecision{action: updateNewVersion, reason: current.reason}
}
