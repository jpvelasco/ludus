package pricing

import (
	"strings"
	"testing"
)

func TestEstimateCost(t *testing.T) {
	tests := []struct {
		name         string
		instanceType string
		wantOK       bool
		wantPrice    float64
	}{
		{"known c6i.large", "c6i.large", true, 0.085},
		{"known c6g.large", "c6g.large", true, 0.068},
		{"known c7g.large", "c7g.large", true, 0.072},
		{"unknown type", "z99.mega", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price, ok := EstimateCost(tt.instanceType)
			if ok != tt.wantOK {
				t.Errorf("ok: got %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && price != tt.wantPrice {
				t.Errorf("price: got %f, want %f", price, tt.wantPrice)
			}
		})
	}
}

func TestFormatEstimate(t *testing.T) {
	tests := []struct {
		name         string
		instanceType string
		wantEmpty    bool
		wantContain  string
	}{
		{"known type", "c6i.large", false, "$0.085/hr"},
		{"unknown type", "z99.mega", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatEstimate(tt.instanceType)
			if tt.wantEmpty && result != "" {
				t.Errorf("expected empty string, got %q", result)
			}
			if !tt.wantEmpty && !strings.Contains(result, tt.wantContain) {
				t.Errorf("expected result to contain %q, got %q", tt.wantContain, result)
			}
		})
	}
}

func TestDefaultInstanceType(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"amd64", "c6i.large"},
		{"arm64", "c7g.large"},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			got := DefaultInstanceType(tt.arch)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInstanceArch(t *testing.T) {
	tests := []struct {
		instanceType string
		want         string
	}{
		{"c6i.large", "amd64"},
		{"c6g.large", "arm64"},
		{"c7g.xlarge", "arm64"},
		{"m5.large", "amd64"},
		{"m6g.large", "arm64"},
		{"r5.large", "amd64"},
		{"z99.mega", ""},
	}

	for _, tt := range tests {
		t.Run(tt.instanceType, func(t *testing.T) {
			got := InstanceArch(tt.instanceType)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAutoSwitch(t *testing.T) {
	tests := []struct {
		name         string
		instanceType string
		arch         string
		wantResolved string
		wantSwitched bool
	}{
		{"match amd64", "c6i.large", "amd64", "c6i.large", false},
		{"match arm64", "c7g.large", "arm64", "c7g.large", false},
		{"mismatch amd64 to arm64", "c6i.large", "arm64", "c7g.large", true},
		{"mismatch arm64 to amd64", "c7g.large", "amd64", "c6i.large", true},
		{"unknown instance", "z99.mega", "arm64", "z99.mega", false},
		{"empty instance", "", "amd64", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, switched := AutoSwitch(tt.instanceType, tt.arch)
			if resolved != tt.wantResolved {
				t.Errorf("resolved: got %q, want %q", resolved, tt.wantResolved)
			}
			if switched != tt.wantSwitched {
				t.Errorf("switched: got %v, want %v", switched, tt.wantSwitched)
			}
		})
	}
}

func TestFormatSuggestion(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		arch      string
		wantEmpty bool
		wantPart  string
	}{
		{"arm64 no suggestion", "c6g.large", "arm64", true, ""},
		{"amd64 with alternative", "c6i.large", "amd64", false, "c6g.large"},
		{"amd64 c5 with alternative", "c5.large", "amd64", false, "c6g.large"},
		{"amd64 no alternative", "r5.large", "amd64", true, ""},
		{"unknown type", "z99.mega", "amd64", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatSuggestion(tt.current, tt.arch)
			if tt.wantEmpty && result != "" {
				t.Errorf("expected empty, got %q", result)
			}
			if !tt.wantEmpty && !strings.Contains(result, tt.wantPart) {
				t.Errorf("expected to contain %q, got %q", tt.wantPart, result)
			}
		})
	}
}

func TestFormatGuidance(t *testing.T) {
	t.Run("amd64 default", func(t *testing.T) {
		result := FormatGuidance("c6i.large", "amd64")
		if !strings.Contains(result, "c6i.large") {
			t.Error("should include c6i.large")
		}
		if !strings.Contains(result, "current") {
			t.Error("should mark current instance")
		}
		if !strings.Contains(result, "Graviton") {
			t.Error("should mention Graviton alternatives for amd64")
		}
	})

	t.Run("arm64", func(t *testing.T) {
		result := FormatGuidance("c7g.large", "arm64")
		if !strings.Contains(result, "c7g.large") {
			t.Error("should include c7g.large")
		}
		if !strings.Contains(result, "c6g.large") {
			t.Error("should include c6g instances for arm64")
		}
	})

	t.Run("no current type", func(t *testing.T) {
		result := FormatGuidance("", "")
		if strings.Contains(result, "current:") {
			t.Error("should not show current type when empty")
		}
	})
}

func TestInstanceCatalogConsistency(t *testing.T) {
	// Every instance in the catalog should have a price in the map
	for _, inst := range instances {
		price, ok := EstimateCost(inst.Type)
		if !ok {
			t.Errorf("instance %s missing from price map", inst.Type)
		}
		if price != inst.PriceUSD {
			t.Errorf("instance %s: price map %f != catalog %f", inst.Type, price, inst.PriceUSD)
		}
	}
}

func TestInstanceArchValues(t *testing.T) {
	for _, inst := range instances {
		if inst.Arch != "amd64" && inst.Arch != "arm64" {
			t.Errorf("instance %s has invalid arch %q", inst.Type, inst.Arch)
		}
	}
}
