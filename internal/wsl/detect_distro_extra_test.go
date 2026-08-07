package wsl

import "testing"

func TestParseDistroLine_TooFewFields(t *testing.T) {
	if d, ok := parseDistroLine("Ubuntu"); ok || d != (DistroInfo{}) {
		t.Fatalf("parseDistroLine(short) = (%+v, %v), want empty false", d, ok)
	}
}

func TestParseDistroLine_NonNumericVersion(t *testing.T) {
	if d, ok := parseDistroLine("Ubuntu Running latest"); ok || d != (DistroInfo{}) {
		t.Fatalf("parseDistroLine(bad version) = (%+v, %v), want empty false", d, ok)
	}
}

func TestParseDistroLine_NameWithSpaces(t *testing.T) {
	d, ok := parseDistroLine("* Ubuntu Server LTS          Stopped         2")
	if !ok || d.Name != "Ubuntu Server LTS" || d.Version != 2 || d.Running || !d.Default {
		t.Fatalf("parseDistroLine(spaces) = %+v, %v", d, ok)
	}
}
