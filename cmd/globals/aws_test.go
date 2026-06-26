package globals

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/internal/awsenv"
)

func TestResolveAWSAccountID(t *testing.T) {
	t.Run("returns configured value without invoking aws", func(t *testing.T) {
		got, err := ResolveAWSAccountID(context.Background(), "123456789012")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "123456789012" {
			t.Errorf("got %q, want 123456789012", got)
		}
	})

	t.Run("dry-run returns placeholder without invoking aws", func(t *testing.T) {
		prev := DryRun
		DryRun = true
		defer func() { DryRun = prev }()

		got, err := ResolveAWSAccountID(context.Background(), "")
		if err != nil {
			t.Fatalf("dry-run should not error, got: %v", err)
		}
		if got != awsenv.PlaceholderAccountID {
			t.Errorf("got %q, want placeholder %q", got, awsenv.PlaceholderAccountID)
		}
	})
}

func TestResolveAWSRegion(t *testing.T) {
	t.Run("returns configured value", func(t *testing.T) {
		got, err := ResolveAWSRegion("us-west-2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "us-west-2" {
			t.Errorf("got %q, want us-west-2", got)
		}
	})

	t.Run("falls back to env var", func(t *testing.T) {
		t.Setenv("AWS_REGION", "eu-central-1")
		t.Setenv("AWS_DEFAULT_REGION", "")
		got, err := ResolveAWSRegion("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "eu-central-1" {
			t.Errorf("got %q, want eu-central-1", got)
		}
	})
}
