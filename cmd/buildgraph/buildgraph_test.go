package buildgraph

import (
	"os"
	"path/filepath"
	"testing"
)

var writeBuildGraphTests = []struct {
	name        string
	outPath     string // relative to temp dir; empty = test default resolution
	projectPath string // relative to temp dir; used when outPath is empty
	wantFile    string // expected output file relative to temp dir
}{
	{
		name:     "explicit path",
		outPath:  "output.xml",
		wantFile: "output.xml",
	},
	{
		name:        "default path from project",
		projectPath: filepath.Join("MyGame", "MyGame.uproject"),
		wantFile:    filepath.Join("MyGame", "Build", "BuildGraph.xml"),
	},
	{
		name:     "creates intermediate directories",
		outPath:  filepath.Join("deep", "nested", "output.xml"),
		wantFile: filepath.Join("deep", "nested", "output.xml"),
	},
	{
		name:     "empty project path resolves to cwd-relative default",
		wantFile: filepath.Join(".", "Build", "BuildGraph.xml"),
	},
}

func TestWriteBuildGraph(t *testing.T) {
	data := []byte("<BuildGraph>test</BuildGraph>")

	for _, tt := range writeBuildGraphTests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			outPath := tt.outPath
			if outPath != "" {
				outPath = filepath.Join(dir, outPath)
			}
			projectPath := tt.projectPath
			if projectPath != "" {
				projectPath = filepath.Join(dir, projectPath)
			}

			// For the empty-path fallback, chdir so relative output lands in temp dir
			if tt.outPath == "" && tt.projectPath == "" {
				t.Chdir(dir)
			}

			if err := writeBuildGraph(data, outPath, projectPath); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			wantFile := filepath.Join(dir, tt.wantFile)
			got, err := os.ReadFile(wantFile)
			if err != nil {
				t.Fatalf("expected file at %s: %v", wantFile, err)
			}
			if string(got) != string(data) {
				t.Errorf("content = %q, want %q", got, data)
			}
		})
	}
}
