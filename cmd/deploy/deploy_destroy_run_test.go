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

// TestRunDestroyDurableReportsCleanupLoadFailure pins the purge wiring: a
// cleanup configuration failure must surface as a command error instead of a
// printed warning and exit 0. AWS_PROFILE is pointed at a profile no host can
// have so the SDK config load fails deterministically.
func TestRunDestroyDurableReportsCleanupLoadFailure(t *testing.T) {
	globals.SetGlobals(t, testGameliftCfg())
	t.Setenv("AWS_PROFILE", "ludus-no-such-profile")
	saveDeployFlags(t)
	destroyPurge = true
	destroyYes = true
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "gamelift"}, nil
	})

	if err := runDestroy(newCommand(), nil); err == nil {
		t.Fatal("runDestroy() error = nil, want the purge load failure to surface")
	}
}

// TestRunDestroySweepAndPurgeAggregates pins the --all-targets --purge path:
// a failing sweep must not hide a purge failure (and vice versa) — both
// surface in one error.
func TestRunDestroySweepAndPurgeAggregates(t *testing.T) {
	globals.SetGlobals(t, testGameliftCfg())
	t.Setenv("AWS_PROFILE", "ludus-no-such-profile")
	saveDeployFlags(t)
	destroyAllTgts = true
	destroyPurge = true
	destroyYes = true
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "x", destroyErr: fmt.Errorf("destroy boom")}, nil
	})

	err := runDestroy(newCommand(), nil)
	if err == nil {
		t.Fatal("runDestroy() error = nil, want aggregated sweep and purge failures")
	}
	for _, want := range []string{"destroy sweep failed", "loading AWS config"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("runDestroy() error = %v, want it to mention %q", err, want)
		}
	}
}

// TestRunDestroySweepFailurePropagates pins the sweep-only path: destroy
// failures must fail the command instead of printing and exiting 0.
func TestRunDestroySweepFailurePropagates(t *testing.T) {
	globals.SetGlobals(t, testGameliftCfg())
	saveDeployFlags(t)
	destroyAllTgts = true
	destroyPurge = false
	swapTargetFactory(t, func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
		return &fakeTarget{name: "x", destroyErr: fmt.Errorf("destroy boom")}, nil
	})

	if err := runDestroy(newCommand(), nil); err == nil {
		t.Fatal("runDestroy() error = nil, want the sweep failure to surface")
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
		wantErr string
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
			name:    "destroy failures continue and are reported",
			verbose: false,
			factory: func(_ context.Context, _ *config.Config, _ string) (deploy.Target, error) {
				return &fakeTarget{name: "x", destroyErr: fmt.Errorf("boom")}, nil
			},
			want:    "0 target(s); 5 failed.",
			wantErr: "destroy sweep failed for 5 target(s)",
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
			var got error
			out := captureStdout(func() {
				got = destroyAllTargets(context.Background(), &config.Config{})
			})
			if !strings.Contains(out, tt.want) {
				t.Errorf("destroyAllTargets() output %q missing %q", out, tt.want)
			}
			if tt.wantErr == "" {
				if got != nil {
					t.Errorf("destroyAllTargets() error = %v, want nil", got)
				}
			} else if got == nil || !strings.Contains(got.Error(), tt.wantErr) {
				t.Errorf("destroyAllTargets() error = %v, want it to mention %q", got, tt.wantErr)
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
