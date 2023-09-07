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

	. "github.com/openthread/ot-ns/types"
)

type DbValue = float64

// IEEE 802.15.4-2015 related parameters for 2.4 GHz O-QPSK PHY
const (
	MinChannelNumber     ChannelId = 0 // below 11 are sub-Ghz channels for 802.15.4-2015
	MaxChannelNumber     ChannelId = 26
	DefaultChannelNumber ChannelId = 11
	TimeUsPerBit                   = 4
)

// default radio & simulation parameters
const (
	receiveSensitivityDbm DbValue = -100.0 // TODO for now MUST be manually kept equal to OT: SIM_RECEIVE_SENSITIVITY
	defaultTxPowerDbm     DbValue = 0.0    // Default, RadioTxEvent msg will override it. OT: SIM_TX_POWER

	// Handtuned - for indoor model, how many meters r is RadioRange disc until Link
	// quality drops below 2 (10 dB margin).
	radioRangeIndoorDistInMeters = 26.70

	noiseFloorIndoorDbm = -95.0 // Indoor model ambient noise floor (dBm), RSSI equivalent
)

// RSSI parameter encodings
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

	// init initializes the RadioModel.
	init()
}

// IndoorModelParams stores model parameters for the simple indoor path loss model.
type IndoorModelParams struct {
	ExponentDb          DbValue // the exponent (dB) in the model
	FixedLossDb         DbValue // the fixed loss (dB) term in the model
	RangeInMeters       float64 // the range in meters represented by the "radio range" parameter of a node.
	NoiseFloorDbm       DbValue // the noise floor (ambient noise, in dBm)
	SnrMinThresholdDb   DbValue // the minimal value an SNR/SINR should be, to have a non-zero frame success probability.
	ShadowFadingSigmaDb DbValue // sigma (stddev) parameter for Shadow Fading (SF), in dB
}

// NewRadioModel creates a new RadioModel with given name, or nil if model not found.
func NewRadioModel(modelName string) RadioModel {
	var model RadioModel
	switch modelName {
	case "Ideal", "I", "1":
		model = &RadioModelIdeal{
			Name:      "Ideal",
			FixedRssi: -60,
		}
	case "Ideal_Rssi", "IR", "2", "default":
		model = &RadioModelIdeal{
			Name:            "Ideal_Rssi",
			UseVariableRssi: true,
			IndoorParams: &IndoorModelParams{
				ExponentDb:    35.0,
				FixedLossDb:   40.0,
				RangeInMeters: radioRangeIndoorDistInMeters,
			},
		}
	case "MutualInterference", "MI", "M", "3":
		model = &RadioModelMutualInterference{
			Name: "MutualInterference",
			IndoorParams: &IndoorModelParams{
				ExponentDb:          35.0,
				FixedLossDb:         40.0,
				RangeInMeters:       radioRangeIndoorDistInMeters,
				NoiseFloorDbm:       noiseFloorIndoorDbm,
				SnrMinThresholdDb:   -4.0, // see calcber.m Octave file
				ShadowFadingSigmaDb: 6.0,
			},
			shadowFading: newShadowFading(),
		}
	case "MIDisc", "MID", "4":
		model = &RadioModelMutualInterference{
			Name:        "MIDisc",
			IsDiscLimit: true,
			IndoorParams: &IndoorModelParams{
				ExponentDb:          30.0,
				FixedLossDb:         40.0,
				RangeInMeters:       radioRangeIndoorDistInMeters,
				NoiseFloorDbm:       noiseFloorIndoorDbm,
				SnrMinThresholdDb:   -4.0, // see calcber.m Octave file
				ShadowFadingSigmaDb: 6.0,
			},
			shadowFading: newShadowFading(),
		}
	default:
		model = nil
	}
	if model != nil {
		model.init()
	}
	return model
}

// computeIndoorRssi computes the RSSI for a receiver at distance dist, using a simple indoor exponent loss model.
// See https://en.wikipedia.org/wiki/ITU_model_for_indoor_attenuation
func computeIndoorRssi(srcRadioRange float64, dist float64, txPower DbValue, modelParams *IndoorModelParams) DbValue {
	pathloss := 0.0
	distMeters := dist * modelParams.RangeInMeters / srcRadioRange
	if distMeters >= 0.072 {
		pathloss = modelParams.ExponentDb*math.Log10(distMeters) + modelParams.FixedLossDb
	}
	rssi := txPower - pathloss

	// constrain RSSI value to range and return it. If RSSI is lower, return RssiMinusInfinity.
	if rssi > RssiMax {
		rssi = RssiMax
	} else if rssi < RssiMin {
		rssi = RssiMinusInfinity
	}
	return rssi
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
