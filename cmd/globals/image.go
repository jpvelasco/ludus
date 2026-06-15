package globals

import (
	"fmt"
	"strings"

	"github.com/jpvelasco/ludus/internal/config"
)

// ResolveEngineImageParts returns the repository/name and tag of the engine
// image that should be pushed. Untagged references use Docker's "latest" tag.
func ResolveEngineImageParts(cfg *config.Config) (string, string, error) {
	imageRef, err := ResolveEngineImage(cfg, false)
	if err != nil {
		return "", "", err
	}
	return splitImageReference(imageRef)
}

func splitImageReference(imageRef string) (string, string, error) {
	if imageRef == "" {
		return "", "", fmt.Errorf("engine image reference is empty")
	}
	if strings.Contains(imageRef, "@") {
		return "", "", fmt.Errorf("engine image reference %q uses a digest; configure a tagged image for push", imageRef)
	}

	lastSlash := strings.LastIndex(imageRef, "/")
	lastColon := strings.LastIndex(imageRef, ":")
	if lastColon <= lastSlash {
		return imageRef, "latest", nil
	}

	name := imageRef[:lastColon]
	tag := imageRef[lastColon+1:]
	if name == "" || tag == "" {
		return "", "", fmt.Errorf("invalid engine image reference %q", imageRef)
	}
	return name, tag, nil
}
