// Package logs implements the `ludus logs` command for inspecting the build
// logs written under .ludus/logs.
package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/spf13/cobra"
)

// Cmd is the top-level logs command group.
var Cmd = &cobra.Command{
	Use:   "logs",
	Short: "View persisted build logs",
	Long: `Commands for inspecting build logs written under .ludus/logs.

Each build command (engine, game, container, run) tees its output to a
timestamped log file. Use --no-logs to disable, or observability.logs in
ludus.yaml to configure the directory and retention.

  ludus logs list    List recent build logs (newest first)
  ludus logs path    Print the build-logs directory
  ludus logs tail    Print the most recent build log`,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent build logs (newest first)",
	RunE:  runList,
}

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the build-logs directory",
	RunE:  runPath,
}

var tailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Print the most recent build log",
	RunE:  runTail,
}

func init() {
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(pathCmd)
	Cmd.AddCommand(tailCmd)
}

// logFiles returns the *.log files in the logs dir, newest first.
func logFiles() ([]os.DirEntry, string, error) {
	dir := globals.LogsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, dir, nil
		}
		return nil, dir, fmt.Errorf("reading logs dir %s: %w", dir, err)
	}
	logs := make([]os.DirEntry, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".log" {
			logs = append(logs, e)
		}
	}
	sort.Slice(logs, func(i, j int) bool {
		ii, _ := logs[i].Info()
		ji, _ := logs[j].Info()
		return ii.ModTime().After(ji.ModTime())
	})
	return logs, dir, nil
}

func runPath(_ *cobra.Command, _ []string) error {
	fmt.Println(globals.LogsDir())
	return nil
}

func runList(_ *cobra.Command, _ []string) error {
	logs, dir, err := logFiles()
	if err != nil {
		return err
	}
	if len(logs) == 0 {
		fmt.Printf("No build logs in %s\n", dir)
		return nil
	}
	for _, e := range logs {
		info, err := e.Info()
		if err != nil {
			continue
		}
		fmt.Printf("%s  %6d KB  %s\n",
			info.ModTime().Format("2006-01-02 15:04:05"),
			info.Size()/1024,
			e.Name())
	}
	return nil
}

func runTail(_ *cobra.Command, _ []string) error {
	logs, dir, err := logFiles()
	if err != nil {
		return err
	}
	if len(logs) == 0 {
		fmt.Printf("No build logs in %s\n", dir)
		return nil
	}
	latest := filepath.Join(dir, logs[0].Name())
	data, err := os.ReadFile(latest)
	if err != nil {
		return fmt.Errorf("reading %s: %w", latest, err)
	}
	fmt.Printf("==> %s <==\n", latest)
	_, _ = os.Stdout.Write(data)
	return nil
}
