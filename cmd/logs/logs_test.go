package logs

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestLogFilesFiltersAndSortsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	setLogsConfig(t, dir)

	writeLogFile(t, dir, "older.log", "old", time.Unix(100, 0))
	writeLogFile(t, dir, "newer.log", "new", time.Unix(200, 0))
	writeLogFile(t, dir, "ignored.txt", "text", time.Unix(300, 0))
	if err := os.Mkdir(filepath.Join(dir, "directory.log"), 0o755); err != nil {
		t.Fatal(err)
	}

	files, gotDir, err := logFiles()
	if err != nil {
		t.Fatal(err)
	}
	if gotDir != dir {
		t.Fatalf("directory = %q, want %q", gotDir, dir)
	}
	got := make([]string, 0, len(files))
	for _, file := range files {
		got = append(got, file.Name())
	}
	want := []string{"newer.log", "older.log"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("files = %v, want %v", got, want)
	}
}

func TestLogFilesMissingDirectoryIsEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	setLogsConfig(t, dir)

	files, gotDir, err := logFiles()
	if err != nil {
		t.Fatal(err)
	}
	if files != nil || gotDir != dir {
		t.Fatalf("logFiles() = (%v, %q), want (nil, %q)", files, gotDir, dir)
	}
}

func TestLogCommands(t *testing.T) {
	dir := t.TempDir()
	setLogsConfig(t, dir)
	writeLogFile(t, dir, "latest.log", "build output\n", time.Unix(200, 0))

	tests := []struct {
		name string
		run  func() error
		want []string
	}{
		{"path", func() error { return runPath(nil, nil) }, []string{dir}},
		{"list", func() error { return runList(nil, nil) }, []string{"latest.log", "0 KB"}},
		{"tail", func() error { return runTail(nil, nil) }, []string{"latest.log", "build output"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := captureStdout(t, tt.run)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("output %q does not contain %q", output, want)
				}
			}
		})
	}
}

func TestRunListReportsEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	setLogsConfig(t, dir)

	output, err := captureStdout(t, func() error { return runList(nil, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if want := "No build logs in " + dir; !strings.Contains(output, want) {
		t.Fatalf("output = %q, want it to contain %q", output, want)
	}
}

func setLogsConfig(t *testing.T, dir string) {
	t.Helper()
	previous := globals.Cfg
	globals.Cfg = &config.Config{}
	globals.Cfg.Observability.Logs.Dir = dir
	t.Cleanup(func() { globals.Cfg = previous })
}

func writeLogFile(t *testing.T, dir, name, content string, modTime time.Time) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func captureStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	runErr := run()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = previous
	t.Cleanup(func() { os.Stdout = previous })
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
}
