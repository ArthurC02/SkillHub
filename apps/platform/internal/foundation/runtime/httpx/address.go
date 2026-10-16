package httpx

import "net/netip"

var neverRoutable = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("255.255.255.255/32"),
}

func NeverFetchable(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return true
	}
	for _, p := range neverRoutable {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func NotPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	return NeverFetchable(ip) || ip.IsLoopback() || ip.IsPrivate()
}
