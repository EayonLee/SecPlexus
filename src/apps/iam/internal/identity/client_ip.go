package identity

import (
	"net"
	"net/netip"
	"strings"
)

// ClientIP 仅在直接连接方可信时解析转发链；解析失败时使用实际连接地址。
func ClientIP(remote, forwarded string, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return ""
	}

	peer, err := netip.ParseAddr(host)
	if err != nil {
	}

	peer = peer.Unmap()
	isTrusted := func(addr netip.Addr) bool {
		for _, prefix := range trusted {
			if prefix.Contains(addr) {
				return true
			}
		}

		return false
	}
	if !isTrusted(peer) || forwarded == "" {
		return peer.String()
	}

	parts := strings.Split(forwarded, ",")
	if len(parts) > 16 {
		return peer.String()
	}

	chain := make([]netip.Addr, 0, len(parts))
	for _, part := range parts {
		address, err := netip.ParseAddr(strings.TrimSpace(part))
		if err != nil {
			return peer.String()
		}

		chain = append(chain, address.Unmap())
	}

	for index := len(chain) - 1; index >= 0; index-- {
		if !isTrusted(chain[index]) || index == 0 {
			return chain[index].String()
		}
	}

	return peer.String()
}
