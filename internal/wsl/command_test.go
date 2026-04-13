package wsl

import (
	"testing"
)

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name   string
		distro string
		args   []string
		want   []string
	}{
		{
			"simple command",
			"Ubuntu",
			[]string{"ls", "-la"},
			[]string{"-d", "Ubuntu", "-e", "ls", "-la"},
		},
		{
			"single arg",
			"Ubuntu-24.04",
			[]string{"whoami"},
			[]string{"-d", "Ubuntu-24.04", "-e", "whoami"},
		},
		{
			"no args",
			"Debian",
			nil,
			[]string{"-d", "Debian", "-e"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildArgs(tt.distro, tt.args)
			if len(got) != len(tt.want) {
				t.Fatalf("buildArgs() len = %d, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("buildArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBuildBashArgs(t *testing.T) {
	tests := []struct {
		name   string
		distro string
		script string
		want   []string
	}{
		{
			"simple script",
			"Ubuntu",
			"echo hello",
			[]string{"-d", "Ubuntu", "bash", "-c", "echo hello"},
		},
		{
			"complex script",
			"Ubuntu-24.04",
			"cd /engine && bash Setup.sh && make -j4",
			[]string{"-d", "Ubuntu-24.04", "bash", "-c", "cd /engine && bash Setup.sh && make -j4"},
		},
		{
			"script with env var",
			"Debian",
			"export UE-LocalDataCachePath=/ddc && echo $UE-LocalDataCachePath",
			[]string{"-d", "Debian", "bash", "-c", "export UE-LocalDataCachePath=/ddc && echo $UE-LocalDataCachePath"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildBashArgs(tt.distro, tt.script)
			if len(got) != len(tt.want) {
				t.Fatalf("buildBashArgs() len = %d, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("buildBashArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
