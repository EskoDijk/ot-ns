// Copyright (c) 2022, The OTNS Authors.
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

package radiomodel

import (
	"math"

	. "github.com/openthread/ot-ns/types"
	"github.com/simonlingoogle/go-simplelogger"
)

// RadioNode is the status of a single radio node of the radio model, used by all radio models.
type RadioNode struct {
	Id NodeId

	// TxPower contains the last Tx power used by the node.
	TxPower DbmValue

	// RxSensitivity contains the Rx sensitivity in dBm of the node.
	RxSensitivity int8

	// RadioRange is the radio range as configured by the simulation for this node.
	RadioRange float64

	// RadioState is the current radio's state; RadioTx only when physically transmitting.
	RadioState    RadioStates
	RadioSubState RadioSubStates

	// RadioChannel is the current radio's channel (For Rx, Tx, or sampling).
	RadioChannel uint8

	// Node position expressed in dimensionless units.
	X, Y float64

	// interferedBy indicates by which other node this RadioNode was interfered during current transmission.
	interferedBy map[NodeId]*RadioNode

	// receivingFrom indicates from which other node this RadioNode is correctly receiving (from the start).
	receivingFrom NodeId

	// rssiSampleMax tracks the max RSSI detected during a channel sampling operation.
	rssiSampleMax DbmValue
}

func NewRadioNode(nodeid NodeId, cfg *NodeConfig) *RadioNode {
	rn := &RadioNode{
		Id:            nodeid,
		TxPower:       defaultTxPowerDbm,
		RxSensitivity: receiveSensitivityDbm,
		X:             float64(cfg.X),
		Y:             float64(cfg.Y),
		RadioRange:    float64(cfg.RadioRange),
		RadioChannel:  DefaultChannelNumber,
		interferedBy:  make(map[NodeId]*RadioNode),
		receivingFrom: 0,
		rssiSampleMax: RssiMinusInfinity,
	}
	return rn
}

func (rn *RadioNode) SetChannel(ch ChannelId) {
	simplelogger.AssertTrue(ch >= MinChannelNumber && ch <= MaxChannelNumber)
	// if changing channel during rx, fail the rx.
	if ch != rn.RadioChannel {
		rn.receivingFrom = 0
	}
	rn.RadioChannel = ch
}

func (rn *RadioNode) SetRadioState(state RadioStates, subState RadioSubStates) {
	// if changing state during rx, fail the rx.
	if state != rn.RadioState {
		rn.receivingFrom = 0
	}
	rn.RadioState = state
	rn.RadioSubState = subState
}

func (rn *RadioNode) SetNodePos(x int, y int) {
	// simplified model: ignore pos changes during Rx.
	rn.X, rn.Y = float64(x), float64(y)
}

// GetDistanceInMeters gets the distance to another RadioNode (in dimensionless units).
func (rn *RadioNode) GetDistanceTo(other *RadioNode) (dist float64) {
	dx := other.X - rn.X
	dy := other.Y - rn.Y
	dist = math.Sqrt(dx*dx + dy*dy)
	return
}
