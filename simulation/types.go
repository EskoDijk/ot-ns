// Copyright (c) 2020-2024, The OTNS Authors.
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
	"io"
	"regexp"
)

var (
	CommandInterruptedError = fmt.Errorf("command interrupted due to simulation exit")
)

var (
	doneOrErrorRegexp = regexp.MustCompile(`(Done|Error \d+: .*)`)
)

type NodeUartType int

const (
	nodeUartTypeUndefined   NodeUartType = iota
	nodeUartTypeRealTime    NodeUartType = iota
	nodeUartTypeVirtualTime NodeUartType = iota
)

// CmdRunner can point to an external package that can run a user's CLI commands.
type CmdRunner interface {
	RunCommand(cmd string, output io.Writer) error
}

// NodeCounters keeps track of a node's internal diagnostic counters.
type NodeCounters map[string]int

// YamlNodeConfig is a node config that can be loaded/saved in YAML.
type YamlNodeConfig struct {
	ID         int     `yaml:"id"`
	Type       string  `yaml:"type"`              // Node type (router, sed, fed, br, etc.)
	Version    *string `yaml:"version,omitempty"` // Thread version string or "" for default
	Position   []int   `yaml:"pos,flow"`
	RadioRange *int    `yaml:"radio-range,omitempty"`
}

// YamlNetworkConfig is global network config that can be loaded/saved in YAML.
type YamlNetworkConfig struct {
	Position   []int `yaml:"pos-shift,omitempty,flow"` // provides an optional 3D position shift of all nodes.
	RadioRange *int  `yaml:"radio-range,omitempty"`    // provides optional default radio-range.
}
