package simulation

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateIp6UdpDatagram(t *testing.T) {
	src := netip.MustParseAddr("fc00::1234")
	dst := netip.MustParseAddr("fe80::abcd")
	udpPayload := []byte{1, 2, 3, 4, 5}

	ip6 := CreateIp6UdpDatagram(5683, 5683, src, dst, udpPayload)
	assert.Equal(t, 64, len(ip6))
}
