// Copyright (c) 2024, The OTNS Authors.
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
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/openthread/ot-ns/logger"
	"github.com/openthread/ot-ns/radiomodel"
	. "github.com/openthread/ot-ns/types"
)

type KpiManager struct {
	sim       *Simulation
	data      *Kpi
	isRunning bool
}

// NewKpiManager creates a new KPI manager/bookkeeper for a particular simulation.
func NewKpiManager() *KpiManager {
	km := &KpiManager{}
	return km
}

// Init inits the KPI manager for the given simulation.
func (km *KpiManager) Init(sim *Simulation) {
	logger.AssertNil(km.sim)
	logger.AssertFalse(km.isRunning)
	km.sim = sim
	km.data = &Kpi{}
}

func (km *KpiManager) Start() {
	logger.AssertFalse(km.isRunning)
	km.data.TimeUs.StartTimeUs = km.sim.Dispatcher().CurTime
	km.isRunning = true
	km.SaveFile(km.getDefaultSaveFileName())
}

func (km *KpiManager) Stop() {
	logger.AssertTrue(km.isRunning)
	km.calculateKpis()
	km.isRunning = false
	km.SaveFile(km.getDefaultSaveFileName())
}

func (km *KpiManager) SaveFile(fn string) {
	logger.AssertNotNil(km.sim)
	if km.isRunning {
		km.calculateKpis()
	}

	km.data.FileTime = time.Now().Format(time.RFC3339)
	json, err := json.MarshalIndent(km.data, "", "    ")
	if err != nil {
		logger.Fatalf("Could not marshal KPI JSON data: %v", err)
		return
	}

	err = os.WriteFile(fn, json, 0644)
	if err != nil {
		logger.Errorf("Could not write  KPI JSON file %s: %v", fn, err)
		return
	}
}

func (km *KpiManager) calculateKpis() {
	// time
	km.data.TimeUs.EndTimeUs = km.sim.Dispatcher().CurTime
	km.data.TimeUs.PeriodUs = km.data.TimeUs.EndTimeUs - km.data.TimeUs.StartTimeUs
	km.data.TimeSec.StartTimeSec = float64(km.data.TimeUs.StartTimeUs) / 1e6
	km.data.TimeSec.EndTimeSec = float64(km.data.TimeUs.EndTimeUs) / 1e6
	km.data.TimeSec.PeriodSec = float64(km.data.TimeUs.PeriodUs) / 1e6

	// channels
	km.data.Channels = make(map[ChannelId]KpiChannel)
	if km.data.TimeUs.PeriodUs > 0 {
		for ch := radiomodel.MinChannelNumber; ch < radiomodel.MaxChannelNumber; ch++ {
			stats := km.sim.Dispatcher().GetRadioModel().GetChannelStats(ch, km.sim.Dispatcher().CurTime)
			if stats != nil {
				chanKpi := KpiChannel{
					TxTimeUs:     stats.TxTimeUs,
					TxPercentage: 100.0 * float64(stats.TxTimeUs) / float64(km.data.TimeUs.PeriodUs),
				}
				km.data.Channels[ch] = chanKpi
			}
		}
	}

	// counters mac
	km.data.Mac.NoAckPercentage = make(map[NodeId]float64)
	km.data.Mac.NumAckRequested = make(map[NodeId]int)
	km.data.Mac.Message = "MAC counters not included due to interrupted simulation"

	if km.sim.IsStopping() {
		return
	}
	for _, nid := range km.sim.GetNodes() {
		counters := km.sim.nodes[nid].GetCounters("mac")

		noAckPercent := 100.0 - 100.0*float64(counters["TxAcked"])/float64(counters["TxAckRequested"])
		if math.IsNaN(noAckPercent) {
			noAckPercent = 0.0
		}
		km.data.Mac.NoAckPercentage[nid] = noAckPercent
		km.data.Mac.NumAckRequested[nid] = counters["TxAckRequested"]
	}
	km.data.Mac.Message = "ok"
}

func (km *KpiManager) getDefaultSaveFileName() string {
	return fmt.Sprintf("%s/%d_kpi.json", km.sim.cfg.OutputDir, km.sim.cfg.Id)
}
