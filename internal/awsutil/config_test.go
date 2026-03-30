package awsutil

import (
	"context"
	"testing"
)

func TestLoadAWSConfig(t *testing.T) {
	tests := []struct {
		name   string
		region string
	}{
		{"valid region", "us-east-1"},
		{"empty region", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadAWSConfig(context.Background(), tt.region)
			if err != nil {
				t.Fatalf("LoadAWSConfig(%q) returned error: %v", tt.region, err)
			}
			if tt.region != "" && cfg.Region != tt.region {
				t.Errorf("Region = %q, want %q", cfg.Region, tt.region)
			}
		})
	}
}
