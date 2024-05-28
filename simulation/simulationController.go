// Copyright (c) 2020, The OTNS Authors.
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
	"strings"

	"github.com/openthread/ot-ns/visualize"
	"github.com/pkg/errors"
)

type simulationController struct {
	sim *Simulation
}

func (sc *simulationController) Command(cmd string) ([]string, error) {
	var outputBuilder strings.Builder

	sim := sc.sim
	err := sim.cmdRunner.RunCommand(cmd, &outputBuilder)
	if err != nil {
		return nil, err
	}
	output := strings.Split(outputBuilder.String(), "\n")
	if output[len(output)-1] == "" {
		output = output[:len(output)-1]
	}
	return output, nil
}

func (sc *simulationController) UpdateNodeStats(nodeStatsInfo visualize.NodeStatsInfo) {
	if sc.sim.vis != nil {
		sc.sim.vis.UpdateNodeStats(nodeStatsInfo)
	}
}

type readonlySimulationController struct {
	sim *Simulation
}

var readonlySimulationError = errors.Errorf("simulation is readonly")

func (rc *readonlySimulationController) Command(cmd string) (output []string, err error) {
	return nil, readonlySimulationError
}

func (rc *readonlySimulationController) UpdateNodeStats(nodeStatsInfo visualize.NodeStatsInfo) {
	if rc.sim.vis != nil {
		rc.sim.vis.UpdateNodeStats(nodeStatsInfo)
	}
}

func NewSimulationController(sim *Simulation) visualize.SimulationController {
	if !sim.cfg.ReadOnly {
		return &simulationController{sim}
	} else {
		return &readonlySimulationController{sim}
	}
}
