package setup

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// promptAWS asks about AWS configuration.
func promptAWS() (region, accountID string) {
	region = prompt("AWS region", "us-east-1")

	accountID = detectAWSAccountID()
	if accountID != "" {
		fmt.Printf("  Detected AWS account: %s\n", accountID)
		if !confirm("  Use this account?") {
			accountID = prompt("  AWS account ID", "")
		}
		return region, accountID
	}

	fmt.Println("  Could not detect AWS account (AWS CLI not configured or not installed).")
	accountID = prompt("  AWS account ID (or press Enter to skip)", "")
	return region, accountID
}

// detectAWSAccountID runs aws sts get-caller-identity to detect the account.
func detectAWSAccountID() string {
	if _, err := exec.LookPath("aws"); err != nil {
		return ""
	}
	cmd := exec.Command("aws", "sts", "get-caller-identity", "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var identity struct {
		Account string `json:"Account"`
	}
	if json.Unmarshal(out, &identity) != nil {
		return ""
	}
	return identity.Account
}
