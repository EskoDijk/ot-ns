// Copyright (c) 2023-2024, The OTNS Authors.
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

package visualize_statslog

import (
	"fmt"
	"os"

	"github.com/openthread/ot-ns/logger"
	. "github.com/openthread/ot-ns/types"
	"github.com/openthread/ot-ns/visualize"
)

type statslogVisualizer struct {
	visualize.NopVisualizer

	simController visualize.SimulationController
	logFile       *os.File
	logFileName   string
	isFileEnabled bool
	changed       bool   // flag to track if some node stats changed
	timestampUs   uint64 // last node stats timestamp (= last log entry)
	stats         NodeStats
	oldStats      NodeStats
}

// NewStatslogVisualizer creates a new Visualizer that writes a log of network stats to file.
func NewStatslogVisualizer(outputDir string, simulationId int) visualize.Visualizer {
	return &statslogVisualizer{
		logFileName:   getStatsLogFileName(outputDir, simulationId),
		isFileEnabled: true,
		changed:       true,
	}
}

func (sv *statslogVisualizer) Init() {
	sv.createLogFile()
}

func (sv *statslogVisualizer) Stop() {
	// add a final entry with final status
	sv.writeLogEntry(sv.timestampUs, sv.stats)
	sv.close()
	logger.Debugf("statslogVisualizer stopped and CSV log file closed.")
}

func (sv *statslogVisualizer) UpdateNodeStats(info *visualize.NodeStatsInfo) {
	sv.oldStats = sv.stats
	sv.stats = info.NodeStats
	sv.timestampUs = info.TimeUs
	sv.writeLogEntry(sv.timestampUs, sv.stats)
}

func (sv *statslogVisualizer) SetController(simController visualize.SimulationController) {
	sv.simController = simController
}

func (sv *statslogVisualizer) createLogFile() {
	logger.AssertNil(sv.logFile)

	var err error
	_ = os.Remove(sv.logFileName)

	sv.logFile, err = os.OpenFile(sv.logFileName, os.O_CREATE|os.O_WRONLY, 0664)
	if err != nil {
		logger.Errorf("creating new stats log file %s failed: %+v", sv.logFileName, err)
		sv.isFileEnabled = false
		return
	}
	sv.writeLogFileHeader()
	logger.Debugf("Stats log file '%s' created.", sv.logFileName)
}

func (sv *statslogVisualizer) writeLogFileHeader() {
	// RFC 4180 CSV file: no leading or trailing spaces in header field names
	header := "timeSec,nNodes,nPartitions,nLeaders,nRouters,nChildren,nDetached,nDisabled,nSleepy,nFailed"
	_ = sv.writeToLogFile(header)
}

/*
func (sv *statslogVisualizer) calcStats() NodeStats {
	s := NodeStats{
		NumNodes:      len(sv.nodeRoles),
		NumLeaders:    countRole(&sv.nodeRoles, OtDeviceRoleLeader),
		NumPartitions: countUniquePts(&sv.nodePartitions),
		NumRouters:    countRole(&sv.nodeRoles, OtDeviceRoleRouter),
		NumEndDevices: countRole(&sv.nodeRoles, OtDeviceRoleChild),
		NumDetached:   countRole(&sv.nodeRoles, OtDeviceRoleDetached),
		NumDisabled:   countRole(&sv.nodeRoles, OtDeviceRoleDisabled),
		NumSleepy:     countSleepy(&sv.nodeModes),
		NumFailed:     len(sv.nodesFailed),
	}
	return s
}

func (sv *statslogVisualizer) checkLogEntryChange() bool {
	sv.stats = sv.calcStats()
	return sv.stats != sv.oldStats
}
*/

func (sv *statslogVisualizer) writeLogEntry(ts uint64, stats NodeStats) {
	timeSec := float64(ts) / 1e6
	entry := fmt.Sprintf("%12.6f, %3d,%3d,%3d,%3d,%3d,%3d,%3d,%3d,%3d", timeSec, stats.NumNodes, stats.NumPartitions,
		stats.NumLeaders, stats.NumRouters, stats.NumEndDevices, stats.NumDetached, stats.NumDisabled,
		stats.NumSleepy, stats.NumFailed)
	_ = sv.writeToLogFile(entry)
	logger.Debugf("statslog entry added: %s", entry)
}

func (sv *statslogVisualizer) writeToLogFile(line string) error {
	if !sv.isFileEnabled {
		return nil
	}
	_, err := sv.logFile.WriteString(line + "\n")
	if err != nil {
		sv.close()
		sv.isFileEnabled = false
		logger.Errorf("couldn't write to node log file (%s), closing it", sv.logFileName)
	}
	return err
}

func (sv *statslogVisualizer) close() {
	if sv.logFile != nil {
		_ = sv.logFile.Close()
		sv.logFile = nil
		sv.isFileEnabled = false
	}
}

func getStatsLogFileName(outputDir string, simId int) string {
	return fmt.Sprintf("%s/%d_stats.csv", outputDir, simId)
}
