package runner

import (
	"bytes"
	"strings"
	"testing"
)

// TestEchoRedactsSensitiveArgValues pins the #708 contract: verbose and dry-run
// echo must never reveal the value of a sensitive argument such as --token,
// while all other arguments stay visible.
func TestEchoRedactsSensitiveArgValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "token value redacted",
			args: []string{"config.sh", "--url", "https://github.com/o/r", "--token", "EXAMPLE_TOKEN", "--labels", "x64"},
			want: "+ cmd config.sh --url https://github.com/o/r --token ******** --labels x64\n",
		},
		{
			name: "token last argument redacted",
			args: []string{"config.sh", "--token", "EXAMPLE_TOKEN"},
			want: "+ cmd config.sh --token ********\n",
		},
		{
			name: "token flag without value untouched",
			args: []string{"config.sh", "--token"},
			want: "+ cmd config.sh --token\n",
		},
		{
			name: "repeated token redacted everywhere",
			args: []string{"a", "--token", "S1", "b", "--token", "S2"},
			want: "+ cmd a --token ******** b --token ********\n",
		},
		{
			name: "no sensitive args unchanged",
			args: []string{"gh", "api", "repos/o/r"},
			want: "+ cmd gh api repos/o/r\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			r := &Runner{Stdout: &out, Verbose: true}
			if r.echo("cmd", tt.args) {
				t.Fatal("echo returned dry-run for a non-dry-run runner")
			}
			if got := out.String(); got != tt.want {
				t.Errorf("echo = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestEchoRedactionDoesNotAlterExecutionArgs guards the other half of #708:
// redaction is for diagnostics only, so the command still receives the real
// token. Executed here through dry-run absence: the args slice passed to echo
// must be unchanged after the call.
func TestEchoRedactionDoesNotAlterExecutionArgs(t *testing.T) {
	args := []string{"config.sh", "--token", "EXAMPLE_TOKEN"}
	orig := strings.Join(args, " ")

	var out bytes.Buffer
	r := &Runner{Stdout: &out, DryRun: true}
	if !r.echo("cmd", args) {
		t.Fatal("echo must report dry-run for a dry-run runner")
	}
	if got := strings.Join(args, " "); got != orig {
		t.Errorf("echo mutated the execution args: got %q, want %q", got, orig)
	}
	if strings.Contains(out.String(), "EXAMPLE_TOKEN") {
		t.Errorf("dry-run echo leaked the token: %q", out.String())
	}
}
