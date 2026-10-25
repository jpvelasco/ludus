package cache

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedHitCache chdirs to a temp dir and records a successful build for stage
// under the given hash.
func seedHitCache(t *testing.T, stage StageKey, hash string) {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := Save(&Cache{Entries: map[StageKey]*Entry{
		stage: {Hash: hash, BuiltAt: "2026-01-01T00:00:00Z"},
	}}); err != nil {
		t.Fatal(err)
	}
}

// capture prints issued while fn runs on stdout.
func capture(t *testing.T, fn func()) string {
	t.Helper()
	prev := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = prev
	t.Cleanup(func() { os.Stdout = prev })
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// runUsableHit invokes c.UsableHit while capturing stdout and returns the
// decision plus the captured output.
func runUsableHit(c *Cache, stage StageKey, hash string, output OutputExists) (bool, string) {
	prev := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return false, ""
	}
	os.Stdout = w
	got := c.UsableHit(stage, hash, output)
	if err := w.Close(); err != nil {
		return got, ""
	}
	os.Stdout = prev
	data, err := io.ReadAll(r)
	if err != nil {
		return got, ""
	}
	return got, string(data)
}

// TestUsableHit pins the #739 contract for accepting a cache hit: the recorded
// hash must match AND, when an output validator is supplied, the stage output
// must still exist. A hit whose output is missing is discarded and reported.
func TestUsableHit(t *testing.T) {
	const hash = "server-hash"
	seedHitCache(t, StageGameServer, hash)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tests := []struct {
		name        string
		hash        string
		output      OutputExists
		want        bool
		wantMissing bool
	}{
		{name: "matching hit without validator", hash: hash, want: true},
		{name: "matching hit with passing validator", hash: hash, output: func() bool { return true }, want: true},
		{name: "matching hit with failing validator", hash: hash, output: func() bool { return false }, wantMissing: true},
		{name: "different hash with passing validator", hash: "other", output: func() bool { return true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, out := runUsableHit(c, StageGameServer, tt.hash, tt.output)
			if got != tt.want {
				t.Errorf("UsableHit() = %v, want %v", got, tt.want)
			}
			if !tt.wantMissing {
				return
			}
			if !strings.Contains(out, OutputMissingMessage) {
				t.Errorf("output = %q, want %q printed when the output is missing", out, OutputMissingMessage)
			}
		})
	}
}

// checkSkipOutputCase runs one CheckSkip output-validation case: it builds
// the validator from the case flags, asserts the skip decision, and checks
// the printed message.
func checkSkipOutputCase(t *testing.T, tt struct {
	name      string
	noCache   bool
	noOutput  bool
	dirExists bool
	want      bool
	wantMsg   string
}, hash string) {
	t.Helper()
	seedHitCache(t, StageEngine, hash)
	dir := filepath.Join(t.TempDir(), "out")
	if tt.dirExists {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	output := DirExists(dir)
	if tt.noOutput || tt.noCache {
		output = nil
	}

	out := capture(t, func() {
		if got := CheckSkip(StageEngine, hash, "Engine", tt.noCache, output); got != tt.want {
			t.Errorf("CheckSkip() = %v, want %v", got, tt.want)
		}
	})
	if tt.wantMsg != "" && !strings.Contains(out, tt.wantMsg) {
		t.Errorf("output = %q, want it to contain %q", out, tt.wantMsg)
	}
}

// TestCheckSkipOutputValidation covers the CLI skip decision with an output
// validator: hit + missing output must rebuild (and explain why), hit +
// present output must skip, and no validator keeps the current behavior.
func TestCheckSkipOutputValidation(t *testing.T) {
	const hash = "engine-hash"
	tests := []struct {
		name      string
		noCache   bool
		noOutput  bool
		dirExists bool
		want      bool
		wantMsg   string
	}{
		{
			name:      "hit with present output skips",
			dirExists: true,
			want:      true,
			wantMsg:   "up to date (cached), skipping",
		},
		{
			name:    "hit with missing output rebuilds",
			wantMsg: OutputMissingMessage,
		},
		{
			name:      "hit without validator keeps current behavior",
			noOutput:  true,
			dirExists: true,
			want:      true,
			wantMsg:   "up to date (cached), skipping",
		},
		{
			name:    "noCache bypasses the check",
			noCache: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkSkipOutputCase(t, tt, hash)
		})
	}
}

func TestDirExists(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(existing, "bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		wantNil bool
		want    bool
	}{
		{name: "existing directory", path: existing, want: true},
		{name: "missing directory", path: filepath.Join(dir, "absent")},
		{name: "file is not a directory", path: filepath.Join(existing, "bin")},
		{name: "empty path yields no validator", path: "", wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DirExists(tt.path)
			if tt.wantNil {
				if got != nil {
					t.Errorf("DirExists(%q) = %v, want nil", tt.path, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("DirExists(%q) = nil, want a validator", tt.path)
			}
			if got() != tt.want {
				t.Errorf("DirExists(%q)() = %v, want %v", tt.path, got(), tt.want)
			}
		})
	}
}

// TestCheckSkipMissReasonUnchanged pins that a plain input change still prints
// the miss reason and rebuilds when no output validator is involved.
func TestCheckSkipMissReasonUnchanged(t *testing.T) {
	seedHitCache(t, StageEngine, "old-hash")

	out := capture(t, func() {
		if got := CheckSkip(StageEngine, "new-hash", "Engine", false, nil); got {
			t.Error("CheckSkip() = true for changed inputs, want false")
		}
	})
	if !strings.Contains(out, "inputs changed since last build") {
		t.Errorf("output = %q, want the miss reason printed", out)
	}
}
