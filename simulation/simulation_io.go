// Copyright (c) 2020-2026, The OTNS Authors.
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
	"fmt"
	"gopkg.in/yaml.v3"
	"os"

	"github.com/openthread/ot-ns/logger"
)

// ExportNetwork exports config info of network to a YAML-friendly object.
func (s *Simulation) ExportNetwork() YamlNetworkConfig {
	// include radio-range most likely to be used per node
	rr := s.cfg.NewNodeConfig.RadioRange

	res := YamlNetworkConfig{
		Position:   [3]int{0, 0, 0}, // when exporting, always a 0-offset (node pos shift) is used.
		RadioRange: &rr,
	}
	return res
}

// ExportNodes exports config/position info of all nodes to a YAML-friendly object.
func (s *Simulation) ExportNodes(nwConfig *YamlNetworkConfig) []YamlNodeConfig {
	nodes := s.GetNodes()
	res := make([]YamlNodeConfig, 0)

	for _, nodeId := range nodes {
		node := s.nodes[nodeId]
		dNode := s.Dispatcher().GetNode(nodeId)
		var rr *int = nil
		var ver *string = nil

		// include radio-range for node, if different from network-wide setting
		if nwConfig.RadioRange == nil || node.cfg.RadioRange != *nwConfig.RadioRange {
			rr = &node.cfg.RadioRange
		}

		// include version if non-empty
		if len(node.cfg.Version) > 0 {
			ver = &node.cfg.Version
		}

		cfg := YamlNodeConfig{
			ID:         nodeId,
			Type:       node.cfg.Type,
			Position:   [3]int{dNode.X, dNode.Y, dNode.Z},
			RadioRange: rr,
			Version:    ver,
		}
		res = append(res, cfg)
	}
	return res
}

func (s *Simulation) ImportNodes(nwConfig YamlNetworkConfig, nodes []YamlNodeConfig) error {
	allOk := true
	rr := defaultRadioRange
	if nwConfig.RadioRange != nil {
		rr = *nwConfig.RadioRange
	}
	posOffset := nwConfig.Position
	nodeIdOffset := 0
	if nwConfig.BaseId != nil {
		nodeIdOffset = *nwConfig.BaseId
	}

	for _, node := range nodes {
		cfg := DefaultNodeConfig()

		// fill config with entries from YAML 'node'
		cfg.ID = node.ID + nodeIdOffset
		if node.RadioRange != nil {
			cfg.RadioRange = *node.RadioRange
		} else {
			cfg.RadioRange = rr
		}
		cfg.IsAutoPlaced = false
		cfg.X = node.Position[0] + posOffset[0]
		cfg.Y = node.Position[1] + posOffset[1]
		cfg.Z = node.Position[2] + posOffset[2]
		cfg.Type = node.Type
		if node.Version != nil {
			cfg.Version = *node.Version
		}

		s.NodeConfigFinalize(&cfg)
		_, err := s.AddNode(&cfg)
		if err != nil {
			logger.Warnf("Import node %d: %v", cfg.ID, err)
			allOk = false // continue trying to import remaining nodes
		}
	}

	if !allOk {
		return fmt.Errorf("not all nodes could be imported - see error log above")
	}
	return nil
}

// LoadTopologyFile loads the nodes of a YAML topology file (see etc/mesh-topologies) into the
// simulation. With add=true, node IDs from the file are shifted above the existing node IDs.
// Must be called from the simulation goroutine (e.g. via PostAsync()).
func (s *Simulation) LoadTopologyFile(filename string, add bool) error {
	b, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("could not load file '%s': %w", filename, err)
	}
	cfgFile := YamlConfigFile{}
	if err = yaml.Unmarshal(b, &cfgFile); err != nil {
		return fmt.Errorf("error in YAML file '%s': %w", filename, err)
	}
	if len(cfgFile.NodesList) == 0 {
		return fmt.Errorf("no nodes defined in YAML file '%s'", filename)
	}
	if add {
		nodeIdOffset := s.MaxNodeId() + 1 - cfgFile.MinNodeId()
		cfgFile.NetworkConfig.BaseId = &nodeIdOffset
	}
	return s.ImportNodes(cfgFile.NetworkConfig, cfgFile.NodesList)
}
