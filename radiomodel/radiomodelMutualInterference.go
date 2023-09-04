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
	"github.com/simonlingoogle/go-simplelogger"
)

// RadioModelMutualInterference is a radio model where a transmission may interfere with another transmission
// ongoing on the same channel, depending on the relative level (Rx energy in dBm) of signals. Also, CCA and
// energy scanning are supported. There is no hard stop of reception beyond the radioRange of the node; although
// the radioRange of the node represents the distance at which a minimally workable Thread link can operate, there
// is also radio reception possible beyond the radioRange. Also, devices with better Rx sensitivity will receive
// radio frames at longer distances beyond the radioRange.
type RadioModelMutualInterference struct {
	Name string

	// Whether RF signal reception is limited to the RadioRange disc of each node, or not (default false).
	// If true, the interference (e.g. RSSI sampled on channel) is confined to the disc and frame
	// reception is also confined to the disc.
	IsDiscLimit bool

	// Parameters of an indoor propagation model
	IndoorParams *IndoorModelParams

	nodes                 map[NodeId]*RadioNode
	activeTransmitters    map[ChannelId]map[NodeId]*RadioNode
	activeChannelSamplers map[ChannelId]map[NodeId]*RadioNode
	interferedBy          map[NodeId]map[NodeId]*RadioNode
	eventQ                EventQueue
}

func (rm *RadioModelMutualInterference) AddNode(nodeid NodeId, radioNode *RadioNode) {
	rm.nodes[nodeid] = radioNode
	rm.interferedBy[nodeid] = map[NodeId]*RadioNode{}
}

func (rm *RadioModelMutualInterference) DeleteNode(nodeid NodeId) {
	delete(rm.nodes, nodeid)
	for c := MinChannelNumber; c <= MaxChannelNumber; c++ {
		delete(rm.activeTransmitters[c], nodeid)
		delete(rm.activeChannelSamplers[c], nodeid)
	}
	rm.interferedBy[nodeid] = map[NodeId]*RadioNode{} // clear map
}

func (rm *RadioModelMutualInterference) CheckRadioReachable(src *RadioNode, dst *RadioNode) bool {
	if src == dst || dst.RadioState != RadioRx {
		return false
	}
	if rm.IsDiscLimit && src.GetDistanceTo(dst) > src.RadioRange {
		return false
	}
	rssi := rm.GetTxRssi(src, dst)
	return rssi >= RssiMin && rssi <= RssiMax && rssi >= dst.RxSensitivity
}

func (rm *RadioModelMutualInterference) GetTxRssi(srcNode *RadioNode, dstNode *RadioNode) DbValue {
	dist := srcNode.GetDistanceTo(dstNode)
	if rm.IsDiscLimit && dist > srcNode.RadioRange {
		return RssiMinusInfinity
	}
	rssi := computeIndoorRssi(srcNode.RadioRange, dist, srcNode.TxPower, rm.IndoorParams)
	return rssi
}

func (rm *RadioModelMutualInterference) OnEventDispatch(src *RadioNode, dst *RadioNode, evt *Event) bool {
	switch evt.Type {
	case EventTypeRadioCommStart:
		// compute the RSSI and store in the event.
		evt.RadioCommData.PowerDbm = int8(math.Round(rm.GetTxRssi(src, dst)))

	case EventTypeRadioRxDone:
		// compute the RSSI and store in the event
		evt.RadioCommData.PowerDbm = int8(math.Round(rm.GetTxRssi(src, dst)))

		// check for interference by other signals and apply to event.
		rm.applyInterference(src, dst, evt)

	case EventTypeRadioChannelSample:
		// take final channel sample
		if evt.RadioCommData.Error == OT_ERROR_NONE {
			r := rm.getRssiOnChannel(src, int(evt.RadioCommData.Channel))
			if r > src.rssiSampleMax {
				src.rssiSampleMax = r
			}
			// store the final sampled RSSI in the event
			evt.RadioCommData.PowerDbm = int8(math.Ceil(src.rssiSampleMax))
		} else {
			evt.RadioCommData.PowerDbm = int8(RssiInvalid)
		}

	default:
		break
	}
	return true
}

func (rm *RadioModelMutualInterference) HandleEvent(node *RadioNode, q EventQueue, evt *Event) {
	rm.eventQ = q

	switch evt.Type {
	case EventTypeRadioCommStart:
		rm.txStart(node, evt)
		rm.updateChannelSamplingNodes(node, evt) // all channel-sampling nodes detect the new Tx
	case EventTypeRadioTxDone:
		rm.txStop(node, evt)
	case EventTypeRadioChannelSample:
		rm.channelSampleStart(node, evt)
	default:
		break // Unknown events not handled.
	}
}

func (rm *RadioModelMutualInterference) GetName() string {
	return rm.Name
}

func (rm *RadioModelMutualInterference) init() {
	rm.nodes = map[NodeId]*RadioNode{}
	rm.activeTransmitters = map[ChannelId]map[NodeId]*RadioNode{}
	rm.activeChannelSamplers = map[ChannelId]map[NodeId]*RadioNode{}
	for c := MinChannelNumber; c <= MaxChannelNumber; c++ {
		rm.activeTransmitters[c] = map[NodeId]*RadioNode{}
		rm.activeChannelSamplers[c] = map[NodeId]*RadioNode{}
	}
	rm.interferedBy = map[NodeId]map[NodeId]*RadioNode{}
}

func (rm *RadioModelMutualInterference) getRssiAmbientNoise(node *RadioNode, channel ChannelId) DbValue {
	return rm.IndoorParams.RssiNoiseFloor
}

func (rm *RadioModelMutualInterference) getRssiOnChannel(node *RadioNode, channel ChannelId) DbValue {
	rssiMax := rm.getRssiAmbientNoise(node, channel)
	// loop all active transmitters
	for _, v := range rm.activeTransmitters[channel] {
		rssi := rm.GetTxRssi(v, node)
		if rssi == RssiInvalid {
			continue
		}
		if rssi > rssiMax {
			rssiMax = rssi // TODO combine signal energies in more realistic way.
		}
	}
	return rssiMax
}

func (rm *RadioModelMutualInterference) txStart(node *RadioNode, evt *Event) {
	// verify node doesn't already transmit or sample on this channel.
	ch := int(evt.RadioCommData.Channel) // move to the (new) channel for this Tx
	_, nodeTransmits := rm.activeTransmitters[ch][node.Id]
	_, nodeSamples := rm.activeChannelSamplers[ch][node.Id]
	if nodeTransmits || nodeSamples {
		// schedule new event dispatch to indicate tx is done with error.
		txDoneEvt := evt.Copy()
		txDoneEvt.Type = EventTypeRadioTxDone
		txDoneEvt.RadioCommData.Error = OT_ERROR_ABORT
		txDoneEvt.MustDispatch = true
		txDoneEvt.Timestamp += 1
		rm.eventQ.Add(&txDoneEvt)
		return
	}

	node.TxPower = DbValue(evt.RadioCommData.PowerDbm)
	node.SetChannel(ch)

	// reset interferedBy bookkeeping, remove data from last time.
	rm.interferedBy[node.Id] = map[NodeId]*RadioNode{} // clear map

	// mark what this new transmission will interfere with and will be interfered by.
	for id, interferingTransmitter := range rm.activeTransmitters[ch] {
		simplelogger.AssertTrue(id != node.Id) // sanity check
		rm.interferedBy[node.Id][id] = interferingTransmitter
		rm.interferedBy[id][node.Id] = node
	}

	rm.activeTransmitters[ch][node.Id] = node

	// dispatch radio event RadioComm 'start of frame Rx' to listening nodes.
	rxStartEvt := evt.Copy()
	rxStartEvt.Type = EventTypeRadioCommStart
	rxStartEvt.RadioCommData.Error = OT_ERROR_NONE
	rxStartEvt.MustDispatch = true
	rm.eventQ.Add(&rxStartEvt)

	// schedule new internal event to call txStop() at end of duration.
	txDoneEvt := evt.Copy()
	txDoneEvt.Type = EventTypeRadioTxDone
	txDoneEvt.RadioCommData.Error = OT_ERROR_NONE
	txDoneEvt.MustDispatch = false
	txDoneEvt.Timestamp += evt.RadioCommData.Duration
	rm.eventQ.Add(&txDoneEvt)
}

func (rm *RadioModelMutualInterference) txStop(node *RadioNode, evt *Event) {
	ch := int(evt.RadioCommData.Channel)
	_, nodeTransmits := rm.activeTransmitters[ch][node.Id]
	simplelogger.AssertTrue(nodeTransmits)

	// stop active transmission
	delete(rm.activeTransmitters[ch], node.Id)

	// Dispatch TxDone event back to the source, at time==now
	txDoneEvt := evt.Copy()
	txDoneEvt.Type = EventTypeRadioTxDone
	txDoneEvt.RadioCommData.Error = OT_ERROR_NONE
	txDoneEvt.MustDispatch = true
	rm.eventQ.Add(&txDoneEvt)

	// Create RxDone event, to signal nearby node(s) that the frame Rx is done, at time==now
	rxDoneEvt := evt.Copy()
	rxDoneEvt.Type = EventTypeRadioRxDone
	rxDoneEvt.RadioCommData.Error = OT_ERROR_NONE
	rxDoneEvt.MustDispatch = true
	rm.eventQ.Add(&rxDoneEvt)
}

func (rm *RadioModelMutualInterference) applyInterference(src *RadioNode, dst *RadioNode, evt *Event) {
	// Apply interference. Loop all interferers that were active during Tx by 'src'. Find the strongest.
	rssiInterfererMax := rm.getRssiAmbientNoise(dst, ChannelId(evt.RadioCommData.Channel))
	for _, interferer := range rm.interferedBy[src.Id] {
		if interferer == dst { // if dst node was at some point transmitting itself, fail the Rx
			rm.log(evt.Timestamp, dst.Id, "Detected self-transmission of Node, set Rx OT_ERROR_ABORT")
			evt.RadioCommData.Error = OT_ERROR_ABORT
			return
		}
		// calculate how strong the interferer was, as seen by dst
		rssiInterferer := rm.GetTxRssi(interferer, dst)
		if rssiInterferer > rssiInterfererMax { // TODO more accurate way of combining signal powers
			rssiInterfererMax = rssiInterferer
		}
	}

	// probabilistic BER model
	rssi := rm.GetTxRssi(src, dst)
	sirDb := rssi - rssiInterfererMax // the Signal-to-Interferer (SIR/SINR) ratio
	isLogMsg, logMsg := applyBerModel(sirDb, src.Id, evt)
	if isLogMsg {
		rm.log(evt.Timestamp, dst.Id, logMsg) // log it on dest node's log
	}
}

// update sample value for all channel-sampling nodes that may detect the new source src.
func (rm *RadioModelMutualInterference) updateChannelSamplingNodes(src *RadioNode, evt *Event) {
	simplelogger.AssertTrue(evt.Type == EventTypeRadioCommStart)
	for _, samplingNode := range rm.activeChannelSamplers[int(evt.RadioCommData.Channel)] {
		r := rm.GetTxRssi(src, samplingNode)
		if r > samplingNode.rssiSampleMax && r != RssiInvalid {
			samplingNode.rssiSampleMax = r // TODO accurate method of energy combining.
		}
	}
}

func (rm *RadioModelMutualInterference) channelSampleStart(srcNode *RadioNode, evt *Event) {
	ch := int(evt.RadioCommData.Channel)
	// verify node doesn't already transmit or sample on its channel.
	_, nodeTransmits := rm.activeTransmitters[ch][srcNode.Id]
	_, nodeSamples := rm.activeChannelSamplers[ch][srcNode.Id]
	if nodeTransmits || nodeSamples {
		evt.RadioCommData.Error = OT_ERROR_ABORT
	} else {
		// take 1st channel sample
		srcNode.SetChannel(ch)
		srcNode.rssiSampleMax = rm.getRssiOnChannel(srcNode, ch)
	}
	// dispatch event with result back to node, when channel sampling stops.
	sampleDoneEvt := evt.Copy()
	sampleDoneEvt.Type = EventTypeRadioChannelSample
	sampleDoneEvt.Timestamp += evt.RadioCommData.Duration
	sampleDoneEvt.MustDispatch = true
	rm.eventQ.Add(&sampleDoneEvt)
}

func (rm *RadioModelMutualInterference) log(ts uint64, id NodeId, msg string) {
	const hdr = "(OTNS)       [T] RadioModelMI--: "
	rm.eventQ.Add(&Event{
		Timestamp: ts,
		Type:      EventTypeRadioLog,
		NodeId:    id,
		Data:      []byte(hdr + msg),
	})
}
