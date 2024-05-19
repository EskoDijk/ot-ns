// Copyright (c) 2020-2024, The OTNS Authors.
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
// 1. Redistributions of source code must retain the above copyright
//    notice, this list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimer in the
//    documentation and/or other materials provided with the distribution.
// 3. Neither the name of the copyright holder nor the
//    names of its contributors may be used to endorse or promote products
//    derived from this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
// ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
// LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
// CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
// SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
// CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
// ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// POSSIBILITY OF SUCH DAMAGE.

package simulation

import (
	"encoding/binary"
	"golang.org/x/net/ipv6"
	"math/rand"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
)

func removeAllFiles(globPath string) error {
	files, err := filepath.Glob(globPath)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	return nil
}

func getCommitFromOtVersion(ver string) string {
	if strings.HasPrefix(ver, "OPENTHREAD/") && len(ver) >= 13 {
		commit := ver[11:]
		idx := strings.Index(commit, ";")
		if idx > 0 {
			commit = commit[0:idx]
			return commit
		}
	}
	return ""
}

func mergeNodeCounters(counters ...NodeCounters) NodeCounters {
	res := make(NodeCounters)
	for _, c := range counters {
		for k, v := range c {
			res[k] = v
		}
	}
	return res
}

func randomString(length int) string {
	chars := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	b := make([]byte, length)
	for i := 0; i < length; i++ {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

// SerializeIp6Header serializes an IPv6 header.
func SerializeIp6Header(ipv6 *ipv6.Header, payloadLen int) []byte {
	data := make([]byte, 40)
	data[0] = uint8((ipv6.Version)<<4) | uint8(ipv6.TrafficClass>>4)
	data[1] = uint8(ipv6.TrafficClass<<4) | uint8(ipv6.FlowLabel>>16)
	binary.BigEndian.PutUint16(data[2:], uint16(ipv6.FlowLabel))
	binary.BigEndian.PutUint16(data[4:], uint16(payloadLen))
	data[6] = byte(ipv6.NextHeader)
	data[7] = byte(ipv6.HopLimit)
	copy(data[8:], ipv6.Src)
	copy(data[24:], ipv6.Dst)

	return data
}

// SerializeUdpHeader serializes a UDP datagram header and calculates the checksum.
func SerializeUdpHeader(udpHdr *UdpHeader) []byte {
	data := make([]byte, 8)
	binary.BigEndian.PutUint16(data, uint16(udpHdr.SrcPort))
	binary.BigEndian.PutUint16(data[2:], uint16(udpHdr.DstPort))
	binary.BigEndian.PutUint16(data[4:], udpHdr.Length)
	binary.BigEndian.PutUint16(data[6:], udpHdr.Checksum)
	return data
}

func CalculateUdpChecksum(srcPort uint16, dstPort uint16, srcIp6Addr netip.Addr, dstIp6Addr netip.Addr, msg []byte) uint16 {
	sum := uint32(0)

	pseudoHeader := make([]byte, 48)
	copy(pseudoHeader[0:16], srcIp6Addr.AsSlice())
	copy(pseudoHeader[16:32], dstIp6Addr.AsSlice())
	binary.BigEndian.PutUint32(pseudoHeader[32:36], uint32(len(msg)+8))
	pseudoHeader[39] = 17 // UDP next-header
	binary.BigEndian.PutUint16(pseudoHeader[40:42], srcPort)
	binary.BigEndian.PutUint16(pseudoHeader[42:44], dstPort)
	binary.BigEndian.PutUint16(pseudoHeader[44:46], uint16(len(msg)))

	data := append(pseudoHeader, msg...)

	for ; len(data) >= 2; data = data[2:] {
		sum += uint32(data[0])<<8 | uint32(data[1])
	}
	if len(data) > 0 {
		sum += uint32(data[0]) << 8
	}
	for sum > 0xffff {
		sum = (sum >> 16) + (sum & 0xffff)
	}
	csum := ^uint16(sum)
	if csum == 0 {
		csum = 0xffff
	}
	return csum
}

// CreateIp6UdpDatagram creates an IPv6+UDP datagram, including UDP payload.
func CreateIp6UdpDatagram(srcPort uint16, dstPort uint16, srcIp6Addr netip.Addr, dstIp6Addr netip.Addr, udpPayload []byte) []byte {
	udpHeader := &UdpHeader{
		SrcPort:  srcPort,
		DstPort:  dstPort,
		Length:   uint16(len(udpPayload)),
		Checksum: CalculateUdpChecksum(srcPort, dstPort, srcIp6Addr, dstIp6Addr, udpPayload),
	}
	udpHeaderSer := SerializeUdpHeader(udpHeader)

	var ip6Header *ipv6.Header
	payloadLen := len(udpPayload) + 8
	ip6Header = &ipv6.Header{
		Version:      6,
		TrafficClass: 0,
		FlowLabel:    0,
		PayloadLen:   payloadLen,
		NextHeader:   17, // UDP next-header id
		HopLimit:     64, // FIXME
		Src:          srcIp6Addr.AsSlice(),
		Dst:          dstIp6Addr.AsSlice(),
	}
	ip6Datagram := SerializeIp6Header(ip6Header, payloadLen)
	ip6Datagram = append(ip6Datagram, udpHeaderSer...)
	ip6Datagram = append(ip6Datagram, udpPayload...)

	return ip6Datagram
}
