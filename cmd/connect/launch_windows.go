//go:build windows

package connect

import (
	"fmt"
	"os"
	"os/exec"
)

func launchClient(binaryPath, platform, outputDir, connectAddr, clientTarget string) error {
	if platform != "Win64" {
		fmt.Println("Client was built for Linux.")
		fmt.Println("To connect from a Linux machine, run ludus connect there.")
		return nil
	}

	fmt.Printf("Launching client: %s\n", binaryPath)
	fmt.Printf("Connecting to: %s\n", connectAddr)

	cmd := exec.Command(binaryPath,
		connectAddr,
		"-game",
		"-log",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch client: %w", err)
	}

	fmt.Printf("Client launched (PID %d)\n", cmd.Process.Pid)
	return nil
}
