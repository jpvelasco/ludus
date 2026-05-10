package runner

import (
	"testing"
)

func TestEnviron(t *testing.T) {
	t.Run("nil when Env is empty", func(t *testing.T) {
		r := NewRunner(false, false)
		if env := r.environ(); env != nil {
			t.Errorf("expected nil, got %v", env)
		}
	})

	t.Run("adds new variables", func(t *testing.T) {
		r := NewRunner(false, false)
		r.Env = []string{"LUDUS_TEST_VAR=hello"}
		env := r.environ()
		if env == nil {
			t.Fatal("expected non-nil env")
		}
		found := false
		for _, kv := range env {
			if kv == "LUDUS_TEST_VAR=hello" {
				found = true
				break
			}
		}
		if !found {
			t.Error("LUDUS_TEST_VAR=hello not found in merged env")
		}
	})

	t.Run("overrides existing variables", func(t *testing.T) {
		r := NewRunner(false, false)
		// PATH is virtually guaranteed to exist in the parent environment.
		r.Env = []string{"PATH=/override/path"}
		env := r.environ()
		if env == nil {
			t.Fatal("expected non-nil env")
		}
		count := 0
		for _, kv := range env {
			if len(kv) >= 5 && kv[:5] == "PATH=" {
				count++
				if kv != "PATH=/override/path" {
					t.Errorf("expected PATH=/override/path, got %s", kv)
				}
			}
		}
		if count != 1 {
			t.Errorf("expected exactly 1 PATH entry, got %d", count)
		}
	})

	t.Run("preserves parent env alongside overrides", func(t *testing.T) {
		r := NewRunner(false, false)
		r.Env = []string{"LUDUS_TEST_ONLY=1"}
		env := r.environ()
		// The merged env should contain at least the parent env entries plus
		// the new variable.
		if len(env) < 2 {
			t.Errorf("merged env too small: %d entries", len(env))
		}
	})
}
