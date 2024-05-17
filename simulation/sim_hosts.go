package simulation

import (
	"fmt"
	"github.com/openthread/ot-ns/event"
	"github.com/openthread/ot-ns/logger"
	"golang.org/x/net/ipv6"
	"net"
	"net/netip"
)

const (
	udpHeaderLen = 8
	protocolUdp  = 17
)

// ConnId is a unique identifier/tuple for a TCP or UDP connection between a node and a simulated host.
type ConnId struct {
	NodeIp6Addr netip.Addr
	ExtIp6Addr  netip.Addr
	NodePort    uint16
	ExtPort     uint16
}

// SimConn is a two-way connection between a node's port and a simulated host's port.
type SimConn struct {
	BrNode          *Node // assumes a single BR also handles the return traffic.
	Conn            net.Conn
	Nat66State      *event.MsgToHostEventData
	PortMapped      uint16
	BytesUpstream   uint64 // total bytes from node to sim-host (across all BRs)
	BytesDownstream uint64 // total bytes from sim-host to node (across all BRs)
}

// SimHostEndpoint represents a single endpoint (port) of a sim-host, potentially interacting with N >= 0 nodes.
type SimHostEndpoint struct {
	HostName   string
	Ip6Addr    netip.Addr
	Port       uint16 // destination UDP/TCP port as specified by the simulated node.
	PortMapped uint16 // actual sim-host port on [::1] to which specified port is mapped.
}

// SimHosts manages all connections between nodes and simulated hosts.
type SimHosts struct {
	sim   *Simulation
	Hosts map[SimHostEndpoint]struct{}
	Conns map[ConnId]*SimConn
}

func NewSimHosts() *SimHosts {
	sh := &SimHosts{
		sim:   nil,
		Hosts: make(map[SimHostEndpoint]struct{}),
		Conns: make(map[ConnId]*SimConn),
	}
	return sh
}

func (sh *SimHosts) Init(sim *Simulation) {
	sh.sim = sim
}

func (sh *SimHosts) AddHost(host SimHostEndpoint) error {
	sh.Hosts[host] = struct{}{}
	// TODO check for conflicts with existing item
	return nil
}

func (sh *SimHosts) RemoveHost(host SimHostEndpoint) {
	delete(sh.Hosts, host)
	// FIXME close all related connection state
}

func (sh *SimHosts) GetTxBytes(host *SimHostEndpoint) uint64 {
	var n uint64 = 0
	for connId, simConn := range sh.Conns {
		if host.Ip6Addr == connId.ExtIp6Addr && host.Port == connId.ExtPort {
			n += simConn.BytesDownstream
		}
	}
	return n
}

func (sh *SimHosts) GetRxBytes(host *SimHostEndpoint) uint64 {
	var n uint64 = 0
	for connId, simConn := range sh.Conns {
		if host.Ip6Addr == connId.ExtIp6Addr && host.Port == connId.ExtPort {
			n += simConn.BytesUpstream
		}
	}
	return n
}

// handleUdpFromNode handles a UDP message coming from a node and checks to which sim-host to deliver it.
func (sh *SimHosts) handleUdpFromNode(node *Node, udpMetadata *event.MsgToHostEventData, udpData []byte) {
	var host SimHostEndpoint
	var err error
	var ok bool
	var simConn *SimConn

	found := false

	// find the first matching simulated host, if any.
	for host = range sh.Hosts {
		if host.Port == udpMetadata.DstPort && host.Ip6Addr == udpMetadata.DstIp6Address {
			found = true
		}
	}
	if !found {
		logger.Debugf("SimHosts: UDP from node %d did not reach any sim-host destination", node.Id)
		return
	}
	logger.Debugf("SimHosts: UDP from node %d, to sim server [::1]:%d (%d bytes)", node.Id, host.PortMapped, len(udpData))

	// fetch existing conn object for the specific Thread node source endpoint, if any.
	connId := ConnId{
		NodeIp6Addr: udpMetadata.SrcIp6Address,
		ExtIp6Addr:  udpMetadata.DstIp6Address,
		NodePort:    udpMetadata.SrcPort,
		ExtPort:     udpMetadata.DstPort,
	}
	if simConn, ok = sh.Conns[connId]; !ok {
		// create new connection
		simConn = &SimConn{
			Conn:            nil,
			BrNode:          node,
			Nat66State:      udpMetadata,
			PortMapped:      host.PortMapped,
			BytesUpstream:   0,
			BytesDownstream: 0,
		}
		simConn.Conn, err = net.Dial("udp", fmt.Sprintf("[::1]:%d", host.PortMapped))
		if err != nil {
			logger.Warnf("SimHosts could not connect to local UDP port %d: %v", host.PortMapped, err)
			if simConn.Conn != nil {
				_ = simConn.Conn.Close()
			}
			return
		}

		// create reader thread - to process the sim-host's response traffic.
		go sh.udpReaderFunc(simConn)

		// store created connection under its unique tuple ID
		sh.Conns[connId] = simConn
	}

	var n int
	n, err = simConn.Conn.Write(udpData)
	if err != nil {
		logger.Warnf("SimHosts could not write udp data to [::1]:%d : %v", host.PortMapped, err)
		return
	}
	simConn.BytesUpstream += uint64(n)
}

// handleUdpFromSimHost handles a UDP message coming from a sim-host and checks to which node to deliver it.
func (sh *SimHosts) handleUdpFromSimHost(simConn *SimConn, udpData []byte) {
	logger.Debugf("SimHosts: UDP from sim-host [::1]:%d (%d bytes)", simConn.PortMapped, len(udpData))
	simConn.BytesDownstream += uint64(len(udpData))
	ev := &event.Event{
		Delay:  0,
		Type:   event.EventTypeUdpFromHost,
		Data:   udpData,
		NodeId: simConn.BrNode.Id,
		MsgToHostData: event.MsgToHostEventData{
			SrcPort:       simConn.Nat66State.DstPort, // simulates response back: ports reversed
			DstPort:       simConn.Nat66State.SrcPort,
			SrcIp6Address: simConn.Nat66State.DstIp6Address, // simulates response: addrs reversed
			DstIp6Address: simConn.Nat66State.SrcIp6Address,
		},
	}
	sh.sim.Dispatcher().PostEventAsync(ev)
}

func (sh *SimHosts) udpReaderFunc(simConn *SimConn) {
	buf := make([]byte, 2048) // FIXME size set
	for {
		rlen, err := simConn.Conn.Read(buf)
		if err != nil {
			panic(err) // FIXME
		}
		sh.handleUdpFromSimHost(simConn, buf[:rlen])
	}
}

func (sh *SimHosts) handleIp6(ip6Metadata *event.MsgToHostEventData, ip6Data []byte) {
	var ip6Header *ipv6.Header
	var err error

	// check if header is IPv6 + UDP?
	if ip6Header, err = ipv6.ParseHeader(ip6Data); err != nil {
		logger.Warnf("SimHosts could not parse as IPv6: %v", err)
		return
	}
	if ip6Header.Version == 6 && ip6Header.NextHeader == protocolUdp && len(ip6Data) > ipv6.HeaderLen+udpHeaderLen {
		udpData := ip6Data[ipv6.HeaderLen+udpHeaderLen:]
		sh.handleUdp(ip6Metadata, udpData)
	}
}
