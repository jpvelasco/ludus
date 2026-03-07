//go:build !windows

package connect

import (
	"fmt"
	"os"
	"syscall"
)

func launchClient(binaryPath, platform, outputDir, connectAddr, projectName, clientTarget string) error {
	if platform == "Win64" {
		fmt.Println("Client was built for Windows (Win64).")
		fmt.Printf("Copy the client directory to your Windows machine:\n")
		fmt.Printf("  %s\n\n", outputDir)
		fmt.Printf("Then run:\n")
		fmt.Printf("  %s.exe %s -game -connect=%s -log\n", clientTarget, projectName, connectAddr)
		return nil
	}

	fmt.Printf("Launching client: %s\n", binaryPath)
	fmt.Printf("Connecting to: %s\n", connectAddr)

	launchArgs := []string{
		binaryPath,
		projectName,
		"-game",
		"-connect=" + connectAddr,
		"-log",
	}

	return syscall.Exec(binaryPath, launchArgs, os.Environ())
}
