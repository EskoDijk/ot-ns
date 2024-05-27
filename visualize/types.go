// Copyright (c) 2022-2024, The OTNS Authors.
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

package visualize

import (
	"time"

	"github.com/openthread/ot-ns/dissectpkt/wpan"
	"github.com/openthread/ot-ns/energy"
	. "github.com/openthread/ot-ns/types"
)

type Visualizer interface {
	Init()
	Run()
	Stop()

	AddNode(nodeid NodeId, cfg *NodeConfig)
	SetNodeRloc16(nodeid NodeId, rloc16 uint16)
	SetNodeRole(nodeid NodeId, role OtDeviceRole)
	SetNodeMode(nodeid NodeId, mode NodeMode)
	Send(srcid NodeId, dstid NodeId, mvinfo *MsgVisualizeInfo)
	SetNodePartitionId(nodeid NodeId, parid uint32)
	SetSpeed(speed float64)
	AdvanceTime(ts uint64, speed float64)
	OnNodeFail(nodeId NodeId)
	OnNodeRecover(nodeId NodeId)
	SetController(ctrl SimulationController)
	SetNodePos(nodeid NodeId, x, y, z int)
	DeleteNode(id NodeId)
	AddRouterTable(id NodeId, extaddr uint64)
	RemoveRouterTable(id NodeId, extaddr uint64)
	AddChildTable(id NodeId, extaddr uint64)
	RemoveChildTable(id NodeId, extaddr uint64)
	ShowDemoLegend(x int, y int, title string)
	CountDown(duration time.Duration, text string)
	SetParent(id NodeId, extaddr uint64)
	OnExtAddrChange(id NodeId, extaddr uint64)
	SetTitle(titleInfo TitleInfo)
	SetNetworkInfo(networkInfo NetworkInfo)
	UpdateNodesEnergy(node []*energy.NodeEnergy, timestamp uint64, updateView bool)
	SetEnergyAnalyser(ea *energy.EnergyAnalyser)
	UpdateNodeStats(nodeStatsInfo NodeStatsInfo)
}

type MsgVisualizeInfo struct {
	Channel         uint8
	FrameControl    wpan.FrameControl
	Seq             uint8
	DstAddrShort    uint16
	DstAddrExtended uint64
	SendDurationUs  uint32
	PowerDbm        int8
	FrameSizeBytes  uint16
}

type TitleInfo struct {
	Title    string
	X        int
	Y        int
	FontSize int
}

func DefaultTitleInfo() TitleInfo {
	return TitleInfo{
		Title:    "",
		X:        0,
		Y:        20,
		FontSize: 20,
	}
}

type NetworkInfo struct {
	Real          bool
	Version       string
	Commit        string
	NodeId        int
	ThreadVersion uint16
}

func DefaultNetworkInfo() NetworkInfo {
	return NetworkInfo{
		Real:          false,
		Version:       "",
		Commit:        "",
		NodeId:        InvalidNodeId,
		ThreadVersion: InvalidThreadVersion,
	}
}

type NodeStats struct {
	NumNodes      int
	NumLeaders    int
	NumPartitions int
	NumRouters    int
	NumEndDevices int
	NumDetached   int
	NumDisabled   int
	NumSleepy     int
	NumFailed     int
}

type NodeStatsInfo struct {
	TimeUs    uint64
	NodeStats NodeStats
}
