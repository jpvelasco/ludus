package prereq

import (
	"fmt"
	"strings"

	"github.com/jpvelasco/ludus/internal/powershell"
)

func visualStudioInstallPowerShellCommand(setupPath string, setupArgs []string) string {
	arguments := make([]string, len(setupArgs))
	quoteNextArgument := false
	for i, arg := range setupArgs {
		if quoteNextArgument {
			arg = `"` + arg + `"`
		}
		arguments[i] = powershell.QuoteSingleQuoted(arg)
		quoteNextArgument = arg == "--installPath"
	}
	return fmt.Sprintf("Start-Process -FilePath %s -ArgumentList @(%s) -Verb RunAs -Wait",
		powershell.QuoteSingleQuoted(setupPath),
		strings.Join(arguments, ", "))
}
