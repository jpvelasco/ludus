package config

import "testing"

func TestIsUnrestrictedCIDR(t *testing.T) {
	tests := []struct {
		name string
		cidr string
		want bool
	}{
		{name: "IPv4 default route", cidr: "0.0.0.0/0", want: true},
		{name: "IPv6 default route", cidr: "::/0", want: true},
		{name: "non-canonical IPv4 default route", cidr: "192.0.2.1/0", want: true},
		{name: "restricted IPv4 network", cidr: "10.0.0.0/8"},
		{name: "restricted IPv6 network", cidr: "2001:db8::/32"},
		{name: "invalid network", cidr: "not-a-cidr"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUnrestrictedCIDR(tt.cidr); got != tt.want {
				t.Errorf("IsUnrestrictedCIDR(%q) = %t, want %t", tt.cidr, got, tt.want)
			}
		})
	}
}
