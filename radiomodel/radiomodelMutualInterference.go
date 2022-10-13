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
	// Configured minimum Signal-to-Interference (SIR) ratio in dB that is required to receive a signal
	// in presence of at least one interfering, other signal.
	MinSirDb DbmValue

	nodes                 map[NodeId]*RadioNode
	activeTransmitters    map[ChannelId]map[NodeId]*RadioNode
	activeChannelSamplers map[ChannelId]map[NodeId]*RadioNode
	interferedBy          map[NodeId]map[NodeId]*RadioNode
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
	if src != dst && dst.RadioState == RadioRx {
		rssi := rm.GetTxRssi(src, dst)
		return rssi >= RssiMin && rssi <= RssiMax && rssi >= dst.RxSensitivity
	}
	return false
}

func (rm *RadioModelMutualInterference) GetTxRssi(srcNode *RadioNode, dstNode *RadioNode) DbmValue {
	simplelogger.AssertTrue(srcNode != dstNode)
	rssi := computeIndoorRssi(srcNode.RadioRange, srcNode.GetDistanceTo(dstNode), srcNode.TxPower, dstNode.RxSensitivity)
	return rssi
}

func (rm *RadioModelMutualInterference) OnEventDispatch(src *RadioNode, dst *RadioNode, evt *Event) bool {
	switch evt.Type {
	case EventTypeRadioCommStart:
		// compute the RSSI and store in the event.
		evt.RadioCommData.PowerDbm = rm.GetTxRssi(src, dst)

	case EventTypeRadioRxDone:
		// compute the RSSI and store in the event
		evt.RadioCommData.PowerDbm = rm.GetTxRssi(src, dst)

		// check for interference by other signals and apply to event.
		rm.applyInterference(src, dst, evt)

	case EventTypeRadioChannelSample:
		// take final channel sample
		r := rm.getRssiOnChannel(src, src.RadioChannel)
		if r > src.rssiSampleMax {
			src.rssiSampleMax = r
		}
		// store the final sampled RSSI in the event
		evt.RadioCommData.PowerDbm = src.rssiSampleMax

	default:
		break
	}
	return true
}

func (rm *RadioModelMutualInterference) HandleEvent(node *RadioNode, q EventQueue, evt *Event) {
	switch evt.Type {
	case EventTypeRadioCommStart:
		rm.txStart(node, q, evt)
		rm.updateChannelSamplingNodes(node, evt) // all channel-sampling nodes detect the new Tx
	case EventTypeRadioTxDone:
		rm.txStop(node, q, evt)
	case EventTypeRadioChannelSample:
		rm.channelSampleStart(node, q, evt)
	default:
		break // Unknown events not handled.
	}
}

func (rm *RadioModelMutualInterference) GetName() string {
	return "MutualInterference"
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

func (rm *RadioModelMutualInterference) getRssiOnChannel(node *RadioNode, channel uint8) int8 {
	rssiMax := RssiMinusInfinity
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

func (rm *RadioModelMutualInterference) txStart(node *RadioNode, q EventQueue, evt *Event) {
	// verify node doesn't already transmit on its current channel.
	_, nodeTransmits := rm.activeTransmitters[node.RadioChannel][node.Id]
	simplelogger.AssertFalse(nodeTransmits)

	node.TxPower = evt.RadioCommData.PowerDbm
	node.SetChannel(evt.RadioCommData.Channel)

	// verify node doesn't already transmit on its new channel.
	_, nodeTransmits = rm.activeTransmitters[node.RadioChannel][node.Id]
	simplelogger.AssertFalse(nodeTransmits)

	// reset interferedBy bookkeeping, remove data from last time.
	rm.interferedBy[node.Id] = map[NodeId]*RadioNode{} // clear map

	// mark what this new transmission will interfere with and will be interfered by.
	for id, interferingTransmitter := range rm.activeTransmitters[node.RadioChannel] {
		simplelogger.AssertTrue(id != node.Id) // sanity check
		rm.interferedBy[node.Id][id] = interferingTransmitter
		rm.interferedBy[id][node.Id] = node
	}

	rm.activeTransmitters[node.RadioChannel][node.Id] = node

	// dispatch radio event RadioComm 'start of frame Rx' to listening nodes.
	rxStartEvt := evt.Copy()
	rxStartEvt.Type = EventTypeRadioCommStart
	rxStartEvt.RadioCommData.Error = OT_ERROR_NONE
	rxStartEvt.MustDispatch = true
	q.Add(&rxStartEvt)

	// schedule new internal event to call txStop() at end of duration.
	txDoneEvt := evt.Copy()
	txDoneEvt.Type = EventTypeRadioTxDone
	txDoneEvt.RadioCommData.Error = OT_ERROR_NONE
	txDoneEvt.MustDispatch = false
	txDoneEvt.Timestamp += evt.RadioCommData.Duration
	q.Add(&txDoneEvt)
}

func (rm *RadioModelMutualInterference) txStop(node *RadioNode, q EventQueue, evt *Event) {
	simplelogger.AssertTrue(node.RadioChannel == evt.RadioCommData.Channel) // basic sanity check
	_, nodeTransmits := rm.activeTransmitters[node.RadioChannel][node.Id]
	simplelogger.AssertTrue(nodeTransmits)

	// stop active transmission
	delete(rm.activeTransmitters[node.RadioChannel], node.Id)

	// Dispatch TxDone event back to the source, at time==now
	txDoneEvt := evt.Copy()
	txDoneEvt.Type = EventTypeRadioTxDone
	txDoneEvt.RadioCommData.Error = OT_ERROR_NONE
	txDoneEvt.MustDispatch = true
	q.Add(&txDoneEvt)

	// Create RxDone event, to signal nearby node(s) that the frame Rx is done, at time==now
	rxDoneEvt := evt.Copy()
	rxDoneEvt.Type = EventTypeRadioRxDone
	rxDoneEvt.RadioCommData.Error = OT_ERROR_NONE
	rxDoneEvt.MustDispatch = true
	q.Add(&rxDoneEvt)
}

func (rm *RadioModelMutualInterference) applyInterference(src *RadioNode, dst *RadioNode, evt *Event) {
	// Apply interference. Loop all interferers that were active during Tx by 'src'.
	for _, interferer := range rm.interferedBy[src.Id] {
		if interferer == dst { // if dst node was at some point transmitting itself, fail the Rx
			evt.RadioCommData.Error = OT_ERROR_ABORT
			return
		}
		// calculate how strong the interferer was, as seen by dst
		rssiInterferer := int(rm.GetTxRssi(interferer, dst))
		rssi := int(evt.RadioCommData.PowerDbm) // the wanted-signal's RSSI as seen at dst
		sirDb := rssi - rssiInterferer          // the Signal-to-Interferer (SIR) ratio
		if sirDb < int(rm.MinSirDb) {
			// interfering signal gets too close to the wanted-signal rssi: impacts the signal.
			evt.Data = interferePsduData(evt.Data, float64(sirDb))
			evt.RadioCommData.Error = OT_ERROR_FCS
		}
	}
}

// update sample value for all channel-sampling nodes that may detect the new source src.
func (rm *RadioModelMutualInterference) updateChannelSamplingNodes(src *RadioNode, evt *Event) {
	simplelogger.AssertTrue(evt.Type == EventTypeRadioCommStart)
	for _, samplingNode := range rm.activeChannelSamplers[evt.RadioCommData.Channel] {
		r := rm.GetTxRssi(src, samplingNode)
		if r > samplingNode.rssiSampleMax && r != RssiInvalid {
			samplingNode.rssiSampleMax = r // TODO accurate method of energy combining.
		}
	}
}

func (rm *RadioModelMutualInterference) channelSampleStart(srcNode *RadioNode, q EventQueue, evt *Event) {
	srcNode.SetChannel(evt.RadioCommData.Channel)
	// take 1st channel sample
	srcNode.rssiSampleMax = rm.getRssiOnChannel(srcNode, srcNode.RadioChannel)

	// dispatch event with result back to node, when channel sampling stops.
	sampleDoneEvt := evt.Copy()
	sampleDoneEvt.Type = EventTypeRadioChannelSample
	sampleDoneEvt.Timestamp += evt.RadioCommData.Duration
	sampleDoneEvt.MustDispatch = true
	q.Add(&sampleDoneEvt)
}
