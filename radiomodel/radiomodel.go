// Copyright (c) 2022-2023, The OTNS Authors.
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

	. "github.com/openthread/ot-ns/event"
	. "github.com/openthread/ot-ns/types"
)

type DbValue = float64

const UndefinedDbValue = math.MaxFloat64

// IEEE 802.15.4-2015 related parameters for 2.4 GHz O-QPSK PHY
const (
	MinChannelNumber     ChannelId = 0 // below 11 are sub-Ghz channels for 802.15.4-2015
	MaxChannelNumber     ChannelId = 26
	DefaultChannelNumber ChannelId = 11
	TimeUsPerBit                   = 4
)

// default radio & simulation parameters
const (
	defaultNoiseFloorIndoorDbm DbValue = -95.0 // Indoor model ambient noise floor (dBm)
	defaultMeterPerUnit        float64 = 0.10  // Default distance equivalent in meters of one grid/pixel distance unit.
)

// RSSI parameter encodings for communication with OT node (maps to int8)
const (
	RssiInvalid       DbValue = 127.0
	RssiMax           DbValue = 126.0
	RssiMin           DbValue = -126.0
	RssiMinusInfinity DbValue = -127.0
)

// EventQueue is the abstraction of the queue where the radio model sends its outgoing (new) events to.
type EventQueue interface {
	Add(*Event)
}

// RadioModel provides access to any type of radio model.
type RadioModel interface {

	// AddNode registers a (new) RadioNode to the model.
	AddNode(nodeid NodeId, radioNode *RadioNode)

	// DeleteNode removes a RadioNode from the model.
	DeleteNode(nodeid NodeId)

	// CheckRadioReachable checks if the srcNode radio can reach the dstNode radio, now, with a >0 probability.
	CheckRadioReachable(srcNode *RadioNode, dstNode *RadioNode) bool

	// GetTxRssi calculates at what RSSI level a radio frame Tx would be received by
	// dstNode, according to the radio model, in the ideal case of no other transmitters/interferers.
	// It returns the expected RSSI value at dstNode, or RssiMinusInfinity if the RSSI value will
	// fall below the minimum Rx sensitivity of the dstNode.
	GetTxRssi(srcNode *RadioNode, dstNode *RadioNode) DbValue

	// OnEventDispatch is called when the Dispatcher sends an Event to a particular dstNode. The method
	// implementation may e.g. apply interference to a frame in transit, prior to delivery of the
	// frame at a single receiving radio dstNode, or apply loss of the frame, or set additional info
	// in the event. Returns true if event can be dispatched, false if not (e.g. due to Rx radio not
	// able to detect the frame).
	OnEventDispatch(srcNode *RadioNode, dstNode *RadioNode, evt *Event) bool

	// HandleEvent handles all radio-model events coming out of the simulator event queue.
	// node is the RadioNode object equivalent to evt.NodeId. Newly generated events may be put back into
	// the EventQueue q for scheduled processing.
	HandleEvent(node *RadioNode, q EventQueue, evt *Event)

	// GetName gets the display name of this RadioModel.
	GetName() string

	// GetParameters gets the parameters of this RadioModel. These may be modified during operation.
	GetParameters() *RadioModelParams

	// init initializes the RadioModel.
	init()
}

// RadioModelParams stores model parameters for the radio model.
type RadioModelParams struct {
	MeterPerUnit        float64 // the distance in meters, equivalent to a single distance unit(pixel)
	IsDiscLimit         bool    // If true, RF signal Tx range is limited to the RadioRange set for each node
	RssiMinDbm          DbValue // Lowest RSSI value (dBm) that can be returned, overriding other calculations
	RssiMaxDbm          DbValue // Highest RSSI value (dBm) that can be returned, overriding other calculations
	ExponentDb          DbValue // the exponent (dB) in the regular/LOS model
	FixedLossDb         DbValue // the fixed loss (dB) term in the regular/LOS model
	NlosExponentDb      DbValue // the exponent (dB) in the NLOS model
	NlosFixedLossDb     DbValue // the fixed loss (dB) term in the NLOS model
	NoiseFloorDbm       DbValue // the noise floor (ambient noise, in dBm)
	SnrMinThresholdDb   DbValue // the minimal value an SNR/SINR should be, to have a non-zero frame success probability.
	ShadowFadingSigmaDb DbValue // sigma (stddev) parameter for Shadow Fading (SF), in dB
}

// newRadioModelParams gets a new set of parameters with default values, as a basis to configure further.
func newRadioModelParams() *RadioModelParams {
	return &RadioModelParams{
		MeterPerUnit:        defaultMeterPerUnit,
		IsDiscLimit:         false,
		RssiMinDbm:          RssiMin,
		RssiMaxDbm:          RssiMax,
		ExponentDb:          UndefinedDbValue,
		FixedLossDb:         UndefinedDbValue,
		NlosExponentDb:      UndefinedDbValue,
		NlosFixedLossDb:     UndefinedDbValue,
		NoiseFloorDbm:       UndefinedDbValue,
		SnrMinThresholdDb:   UndefinedDbValue,
		ShadowFadingSigmaDb: UndefinedDbValue,
	}
}

// NewRadioModel creates a new RadioModel with given name, or nil if model not found.
func NewRadioModel(modelName string) RadioModel {
	var model RadioModel
	switch modelName {
	case "Ideal", "I", "1":
		model = &RadioModelIdeal{name: "Ideal", params: newRadioModelParams()}
		p := model.GetParameters()
		p.IsDiscLimit = true
		p.RssiMinDbm = -60.0
		p.RssiMaxDbm = -60.0

	case "Ideal_Rssi", "IR", "2", "default":
		model = &RadioModelIdeal{
			name:   "Ideal_Rssi",
			params: newRadioModelParams(),
		}
		p := model.GetParameters()
		setIndoorModelParamsItu(p)
		p.IsDiscLimit = true
	case "MutualInterference", "MI", "M", "3":
		model = &RadioModelMutualInterference{
			name:         "MutualInterference",
			params:       newRadioModelParams(),
			shadowFading: newShadowFading(),
		}
		setIndoorModelParams3gpp(model.GetParameters())
	case "MIDisc", "MID", "4":
		model = &RadioModelMutualInterference{
			name:         "MIDisc",
			params:       newRadioModelParams(),
			shadowFading: newShadowFading(),
		}
		p := model.GetParameters()
		setIndoorModelParams3gpp(p)
		p.IsDiscLimit = true
	case "Outdoor", "5":
		model = &RadioModelMutualInterference{
			name:         "Outdoor",
			params:       newRadioModelParams(),
			shadowFading: newShadowFading(),
		}
		setOutdoorModelParams(model.GetParameters())
	default:
		model = nil
	}
	if model != nil {
		model.init()
	}
	return model
}

// addSignalPowersDbm calculates signal power in dBm of two added, uncorrelated, signals with powers p1 and p2 (dBm).
func addSignalPowersDbm(p1 DbValue, p2 DbValue) DbValue {
	if p1 > p2+15.0 {
		return p1
	}
	if p2 > p1+15.0 {
		return p2
	}
	return 10.0 * math.Log10(math.Pow(10, p1/10.0)+math.Pow(10, p2/10.0))
}

// clipRssi clips the RSSI value (in dBm, as DbValue) to int8 range for return to OT nodes.
func clipRssi(rssi DbValue) int8 {
	if rssi > RssiMax {
		rssi = RssiMax
	} else if rssi < RssiMin {
		rssi = RssiMinusInfinity
	}
	return int8(math.Round(rssi))
}
