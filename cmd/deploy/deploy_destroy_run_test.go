package deploy

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/spf13/cobra"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
)

func TestRunDestroyDryRun(t *testing.T) {
	tests := []struct {
		name    string
		sweep   bool
		durable bool
		want    string
	}{
		{"active target only", false, false, "the active target"},
		{"sweep all targets", true, false, "every deploy target"},
		{"purge durable artifacts", false, true, "purge durable artifacts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globals.SetGlobals(t, testGameliftCfg(), globals.WithDryRun(true))
			saveDeployFlags(t)
			destroyAllTgts = tt.sweep
			destroyPurge = tt.durable

			var target *fakeTarget
			swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
				target = &fakeTarget{name: "gamelift"}
				return target, nil
			})

			var runErr error
			out := captureStdout(func() { runErr = runDestroy(newCommand(), nil) })
			if runErr != nil {
				t.Fatalf("runDestroy() dry-run error = %v", runErr)
			}
			if target != nil && target.destroyCalls != 0 {
				t.Errorf("Destroy called %d times, want 0", target.destroyCalls)
			}
			if !strings.Contains(out, "Dry run") {
				t.Errorf("output %q missing Dry run", out)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("output %q missing %q", out, tt.want)
			}
		})
	}
}

func TestDestroyDryRunMessage(t *testing.T) {
	tests := []struct {
		name  string
		scope destroyScope
		want  string
	}{
		{"active ephemeral", destroyScope{}, "the active target"},
		{"sweep ephemeral", destroyScope{sweep: true}, "every deploy target"},
		{"active purge", destroyScope{durable: true}, "purge durable artifacts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := destroyDryRunMessage(tt.scope)
			if !strings.Contains(got, tt.want) {
				t.Errorf("destroyDryRunMessage() = %q, want substring %q", got, tt.want)
			}
			if !strings.Contains(got, "no AWS calls made") {
				t.Errorf("destroyDryRunMessage() = %q, want no-AWS notice", got)
			}
		})
	}
}

func TestRunDestroyDeclinesPurge(t *testing.T) {
	globals.SetGlobals(t, testGameliftCfg())
	saveDeployFlags(t)
	destroyPurge = true

	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("n\n"))
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runDestroy(cmd, nil); err != nil {
		t.Fatalf("runDestroy() error = %v", err)
	}
	if !strings.Contains(out.String(), "Aborted") {
		t.Errorf("runDestroy() output missing Aborted message: %q", out.String())
	}
}

func TestRunDestroyPurgeAcceptedThenActiveTargetError(t *testing.T) {
	globals.SetGlobals(t, testGameliftCfg())
	saveDeployFlags(t)
	destroyPurge = true
	destroyYes = true
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return nil, fmt.Errorf("resolve boom")
	})
	if err := runDestroy(newCommand(), nil); err == nil {
		t.Fatal("runDestroy() expected destroyActiveTarget error")
	}
}

func TestRunDestroyActiveTargetSuccess(t *testing.T) {
	globals.SetGlobals(t, testGameliftCfg())
	saveDeployFlags(t)
	region = "eu-west-1"
	destroyPurge = false
	destroyAllTgts = false

	var target *fakeTarget
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		target = &fakeTarget{name: "gamelift"}
		return target, nil
	})
	if err := runDestroy(newCommand(), nil); err != nil {
		t.Fatalf("runDestroy() error = %v", err)
	}
	if target.destroyCalls != 1 {
		t.Errorf("destroyActiveTarget called Destroy %d times, want 1", target.destroyCalls)
	}
}

func TestRunDestroyDurableSuccessWithCleanupLoadFailure(t *testing.T) {
	globals.SetGlobals(t, testGameliftCfg())
	t.Setenv("AWS_SDK_LOAD_CONFIG", "1")
	t.Setenv("AWS_CONFIG_FILE", t.TempDir())
	saveDeployFlags(t)
	destroyPurge = true
	destroyYes = true
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "gamelift"}, nil
	})
	if err := runDestroy(newCommand(), nil); err != nil {
		t.Fatalf("runDestroy() error = %v", err)
	}
}

func TestRunDestroySweep(t *testing.T) {
	globals.SetGlobals(t, testGameliftCfg())
	saveDeployFlags(t)
	destroyPurge = false
	destroyAllTgts = true
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "ok"}, nil
	})
	if err := runDestroy(newCommand(), nil); err != nil {
		t.Fatalf("runDestroy() sweep error = %v", err)
	}
}

func TestRunDestroySweepResolveFailure(t *testing.T) {
	globals.SetGlobals(t, testGameliftCfg())
	saveDeployFlags(t)
	destroyPurge = false
	destroyAllTgts = true
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return nil, fmt.Errorf("resolve boom")
	})
	if err := runDestroy(newCommand(), nil); err != nil {
		t.Fatalf("runDestroy() sweep error = %v", err)
	}
}

func TestDestroyAllTargets(t *testing.T) {
	tests := []struct {
		name    string
		verbose bool
		factory func(context.Context, *config.Config, string) (deploy.Target, error)
		want    string
	}{
		{
			name:    "resolve failures skipped silently",
			verbose: false,
			factory: func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
				return nil, fmt.Errorf("boom")
			},
			want: "No resources found to destroy.",
		},
		{
			name:    "resolve failures reported when verbose",
			verbose: true,
			factory: func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
				return nil, fmt.Errorf("boom")
			},
			want: "No resources found to destroy.",
		},
		{
			name:    "destroy failures continue and count none",
			verbose: false,
			factory: func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
				return &fakeTarget{name: "x", destroyErr: fmt.Errorf("boom")}, nil
			},
			want: "No resources found to destroy.",
		},
		{
			name:    "all targets destroyed",
			verbose: false,
			factory: func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
				return &fakeTarget{name: "x"}, nil
			},
			want: "Destroyed resources across 5 target(s).",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globals.SetGlobals(t, &config.Config{}, globals.WithVerbose(tt.verbose))
			swapTargetFactory(t, tt.factory)
			out := captureStdout(func() {
				destroyAllTargets(context.Background(), &config.Config{})
			})
			if !strings.Contains(out, tt.want) {
				t.Errorf("destroyAllTargets() output %q missing %q", out, tt.want)
			}
		})
	}
}

func TestDestroyActiveTarget(t *testing.T) {
	t.Run("destroy error propagates", func(t *testing.T) {
		globals.SetGlobals(t, testGameliftCfg())
		saveDeployFlags(t)
		swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
			return &fakeTarget{name: "gamelift", destroyErr: fmt.Errorf("destroy boom")}, nil
		})

		err := destroyActiveTarget(newCommand())
		if err == nil {
			t.Fatal("destroyActiveTarget() expected error")
		}
		if !strings.Contains(err.Error(), "destroy boom") {
			t.Errorf("destroyActiveTarget() error = %v, want destroy boom", err)
		}
	})

	t.Run("success prints confirmation", func(t *testing.T) {
		globals.SetGlobals(t, testGameliftCfg())
		saveDeployFlags(t)
		swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
			return &fakeTarget{name: "gamelift"}, nil
		})

		var runErr error
		out := captureStdout(func() { runErr = destroyActiveTarget(newCommand()) })
		if runErr != nil {
			t.Fatalf("destroyActiveTarget() error = %v", runErr)
		}
		if !strings.Contains(out, "resources destroyed") {
			t.Errorf("destroyActiveTarget() output missing confirmation: %q", out)
		}
	})
}

func TestResolveAccountIDConfigured(t *testing.T) {
	got := resolveAccountID(context.Background(), aws.Config{}, "123456789012")
	if got != "123456789012" {
		t.Errorf("resolveAccountID() = %q, want configured value", got)
	}
}
