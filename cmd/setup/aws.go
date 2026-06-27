// Package setup contains the interactive setup wizard used to detect
// the Unreal Engine source, AWS credentials, and other prerequisites,
// then writes the initial ludus.yaml configuration.
package setup

import (
	"context"
	"fmt"

	"github.com/jpvelasco/ludus/internal/awsenv"
	"github.com/jpvelasco/ludus/internal/config"
)

// promptAWSDefault asks about AWS configuration using existing values as defaults.
func promptAWSDefault(defaultRegion string, existing *config.Config) (region, accountID string) {
	region = prompt("AWS region", defaultRegion)

	accountID = detectAWSAccountID()
	if accountID != "" {
		fmt.Printf("  Detected AWS account: %s\n", accountID)
		if !confirm("  Use this account?") {
			defaultAccount := ""
			if existing != nil {
				defaultAccount = existing.AWS.AccountID
			}
			accountID = prompt("  AWS account ID", defaultAccount)
		}
		return region, accountID
	}

	fmt.Println("  Could not detect AWS account (AWS CLI not configured or not installed).")
	defaultAccount := ""
	if existing != nil {
		defaultAccount = existing.AWS.AccountID
	}
	accountID = prompt("  AWS account ID (or press Enter to skip)", defaultAccount)
	return region, accountID
}

// detectAWSAccountID uses the centralized awsenv resolver (SDK chain → STS/IMDS)
// so detection is consistent with the rest of the system and works without the AWS CLI.
func detectAWSAccountID() string {
	id, err := awsenv.NewResolver(false).ResolveAccountID(context.Background(), &config.Config{})
	if err != nil {
		return ""
	}
	return id
}
