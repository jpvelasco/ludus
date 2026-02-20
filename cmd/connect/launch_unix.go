//go:build !windows

package connect

import (
	"fmt"
	"os"
	"syscall"
)

func launchClient(binaryPath, platform, outputDir, connectAddr string) error {
	if platform == "Win64" {
		fmt.Println("Client was built for Windows (Win64).")
		fmt.Printf("Copy the client directory to your Windows machine:\n")
		fmt.Printf("  %s\n\n", outputDir)
		fmt.Printf("Then run:\n")
		fmt.Printf("  LyraGame.exe Lyra -game -connect=%s -log\n", connectAddr)
		return nil
	}

	fmt.Printf("Launching client: %s\n", binaryPath)
	fmt.Printf("Connecting to: %s\n", connectAddr)

	launchArgs := []string{
		binaryPath,
		"Lyra",
		"-game",
		"-connect=" + connectAddr,
		"-log",
	}

	return syscall.Exec(binaryPath, launchArgs, os.Environ())
}
