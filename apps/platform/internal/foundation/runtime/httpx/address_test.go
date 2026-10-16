package httpx

import (
	"net/netip"
	"testing"
)

func TestAddressPolicyByFamily(t *testing.T) {
	for _, tc := range []struct {
		addr                 string
		notPublic, neverOpen bool
	}{
		{"169.254.169.254", true, true},
		{"::ffff:169.254.169.254", true, true},
		{"fe80::1", true, true},
		{"224.0.0.1", true, true},
		{"ff02::1", true, true},
		{"ff01::1", true, true},
		{"100.64.0.1", true, true},
		{"::ffff:100.64.0.1", true, true},
		{"0.0.0.0", true, true},
		{"::", true, true},
		{"0.1.2.3", true, true},
		{"::ffff:0.1.2.3", true, true},
		{"255.255.255.255", true, true},
		{"::ffff:255.255.255.255", true, true},

		{"127.0.0.1", true, false},
		{"::1", true, false},
		{"::ffff:127.0.0.1", true, false},
		{"10.0.0.5", true, false},
		{"::ffff:10.0.0.5", true, false},
		{"172.16.0.1", true, false},
		{"192.168.1.1", true, false},
		{"fd00::1", true, false},

		{"140.82.121.4", false, false},
		{"2606:2800:220:1:248:1893:25c8:1946", false, false},
	} {
		ip, err := netip.ParseAddr(tc.addr)
		if err != nil {
			t.Fatalf("%s: %v", tc.addr, err)
		}
		if got := NotPublic(ip); got != tc.notPublic {
			t.Errorf("NotPublic(%s) = %v, want %v", tc.addr, got, tc.notPublic)
		}
		if got := NeverFetchable(ip); got != tc.neverOpen {
			t.Errorf("NeverFetchable(%s) = %v, want %v", tc.addr, got, tc.neverOpen)
		}
	}
}
