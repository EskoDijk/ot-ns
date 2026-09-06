// Copyright (c) 2026, The OTNS Authors.
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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openthread/ot-ns/dispatcher"
	. "github.com/openthread/ot-ns/types"
)

// Every output path is built as "<OutputDir>/<file>", so we need to ensure that
// <OutputDir> is not empty.
func TestDefaultConfigOutputDirIsUsable(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, DefaultOutputDir, cfg.OutputDir)
	assert.NotEmpty(t, cfg.OutputDir)
	assert.False(t, filepath.IsAbs(cfg.OutputDir))
}

// The dispatcher writes output files of its own (pcap, node logs, the unix socket) and names
// them after the simulation id, so these settings must always be taken from the simulation
// config - a caller-supplied value that disagrees is corrected, not trusted.
func TestSyncDispatcherConfigTakesSettingsFromSimulation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Id = 7
	cfg.OutputDir = "results"
	cfg.Speed = 42
	cfg.DumpPackets = true

	given := dispatcher.DefaultConfig()
	given.SimulationId = 999
	given.OutputDir = "somewhere-else"

	dcfg := syncDispatcherConfig(cfg, given)

	assert.Equal(t, 7, dcfg.SimulationId)
	assert.Equal(t, "results", dcfg.OutputDir)
	assert.Equal(t, float64(42), dcfg.Speed)
	assert.True(t, dcfg.DumpPackets)
	assert.False(t, dcfg.Realtime)

	// settings the simulation does not own are left as the caller set them
	assert.Equal(t, dispatcher.DefaultConfig().PcapEnabled, dcfg.PcapEnabled)
}

// A realtime simulation always runs the dispatcher at speed 1, whatever cfg.Speed says.
func TestSyncDispatcherConfigRealtimeForcesSpeed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Realtime = true
	cfg.Speed = 42

	dcfg := syncDispatcherConfig(cfg, dispatcher.DefaultConfig())
	assert.True(t, dcfg.Realtime)
	assert.Equal(t, float64(1), dcfg.Speed)
}
