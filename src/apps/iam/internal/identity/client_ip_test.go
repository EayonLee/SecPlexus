package identity

import (
	"net/netip"
	"testing"
)

func TestClientIPTrustBoundary(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8")}
	for _, tc := range []struct{ remote, forwarded, want string }{{"192.0.2.10:1234", "198.51.100.2", "192.0.2.10"}, {"127.0.0.1:1234", "198.51.100.2,10.0.0.1", "198.51.100.2"}, {"127.0.0.1:1234", "spoofed", "127.0.0.1"}, {"[::1]:1234", "198.51.100.2", "::1"}} {
		if got := ClientIP(tc.remote, tc.forwarded, trusted); got != tc.want {
			t.Errorf("want %s, got %s", tc.want, got)
		}
	}
}
