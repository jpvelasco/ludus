package prereq

import "testing"

func TestVisualStudioInstallPowerShellCommandQuotesPaths(t *testing.T) {
	setupPath := `C:\Users\a'b\Visual Studio\Installer\setup.exe`
	installPath := `C:\Program Files\O'Brien\Visual Studio 2022`
	args := []string{"modify", "--installPath", installPath, "--passive", "--norestart"}

	got := visualStudioInstallPowerShellCommand(setupPath, args)
	want := `Start-Process -FilePath 'C:\Users\a''b\Visual Studio\Installer\setup.exe' -ArgumentList @('modify', '--installPath', '"C:\Program Files\O''Brien\Visual Studio 2022"', '--passive', '--norestart') -Verb RunAs -Wait`
	if got != want {
		t.Errorf("visualStudioInstallPowerShellCommand() = %q, want %q", got, want)
	}
}
