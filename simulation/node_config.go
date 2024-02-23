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
	"os"
	"path/filepath"
	"strings"

	"github.com/openthread/ot-ns/logger"
	"github.com/openthread/ot-ns/prng"
	. "github.com/openthread/ot-ns/types"
)

type ExecutableConfig struct {
	Version     string
	Ftd         string
	Mtd         string
	Br          string
	SearchPaths []string
}

type NodeAutoPlacer struct {
	X, Y, Z         int
	Xref, Yref      int
	Xmax            int
	NodeDeltaCoarse int
	NodeDeltaFine   int
	fineCount       int
	isReset         bool
}

var DefaultExecutableConfig ExecutableConfig = ExecutableConfig{
	Version:     "",
	Ftd:         "ot-cli-ftd",
	Mtd:         "ot-cli-mtd",
	Br:          "ot-cli-ftd_br",
	SearchPaths: []string{".", "./ot-rfsim/ot-versions", "./build/bin"},
}

// defaultNodeInitScript is an array of commands, sent to a new node by default (unless changed).
var defaultNodeInitScript = []string{
	"networkname " + DefaultNetworkName,
	"networkkey " + DefaultNetworkKey,
	fmt.Sprintf("panid 0x%x", DefaultPanid),
	fmt.Sprintf("channel %d", DefaultChannel),
	//"routerselectionjitter 1", // jitter can be set to '1' to speed up network formation for realtime tests.
	"ifconfig up",
	"thread start",
}

// defaultBrScript is an array of commands, sent to a new BR by default (unless changed).
var defaultBrScript = []string{
	"routerselectionjitter 1",                              // BR wants to become Router early on.
	"routerdowngradethreshold 33",                          // BR never wants to downgrade.
	"routerupgradethreshold 33",                            // BR always wants to upgrade.
	"netdata publish prefix fd00:f00d:cafe::/64 paros med", // OMR prefix from DHCPv6-PD delegation (ULA infra)
	"netdata publish route fc00::/7 s med",                 // route to ULA-based AIL
	"netdata publish route 64:ff9b::/96 sn med",            // infrastructure-defined NAT64 translation
	"bbr enable",
	"srp server enable",
	"br init 1 1", // see https://github.com/openthread/openthread/blob/main/src/cli/README_BR.md
	"br enable",
}

var defaultCslScript = []string{
	fmt.Sprintf("csl period %d", DefaultCslPeriodUs),
}

var defaultLegacyCslScript = []string{
	fmt.Sprintf("csl period %d", DefaultCslPeriod),
}

var defaultRadioRange = 220

func DefaultNodeConfig() NodeConfig {
	return NodeConfig{
		ID:             -1, // < 0 for the next available nodeid
		Type:           ROUTER,
		Version:        "",
		X:              0,
		Y:              0,
		Z:              0,
		IsAutoPlaced:   true,
		IsRaw:          false,
		IsRouter:       true,
		IsMtd:          false,
		IsBorderRouter: false,
		RxOffWhenIdle:  false,
		NodeLogFile:    true,
		RadioRange:     defaultRadioRange,
		ExecutablePath: "",
		Restore:        false,
		InitScript:     defaultNodeInitScript,
		RandomSeed:     0, // 0 means not specified, i.e. truly unpredictable.
	}
}

// NodeConfigFinalize finalizes the configuration for a new Node before it's used to create it. This is not
// mandatory to call, but a convenience method for the caller to avoid setting all details itself.
func (s *Simulation) NodeConfigFinalize(nodeCfg *NodeConfig) {
	if nodeCfg.ID <= 0 {
		nodeCfg.ID = s.genNodeId()
	}

	nodeCfg.UpdateNodeConfigFromType()
	nodeCfg.ExecutablePath = s.cfg.ExeConfig.FindExecutableBasedOnConfig(nodeCfg)

	// check for an implicit Thread-version setting in the executable-selection.
	if len(s.cfg.ExeConfig.Version) > 0 && len(nodeCfg.Version) == 0 {
		nodeCfg.Version = s.cfg.ExeConfig.Version
	}

	// in case of specified simulation random seed, each node gets a PRNG-predictable random seed assigned.
	if s.cfg.RandomSeed != 0 {
		nodeCfg.RandomSeed = prng.NewNodeRandomSeed()
	}

	// for a BR, do extra init steps to set prefix/routes/etc.
	if nodeCfg.IsBorderRouter {
		nodeCfg.InitScript = append(nodeCfg.InitScript, defaultBrScript...)
	}

	// for SSED, do extra CSL init command.
	if nodeCfg.Type == SSED {
		cslScript := defaultCslScript
		if len(nodeCfg.Version) > 0 && nodeCfg.Version <= "v13" {
			cslScript = defaultLegacyCslScript // older nodes use different parameter unit
		}
		nodeCfg.InitScript = append(nodeCfg.InitScript, cslScript...)
	}
}

func (cfg *ExecutableConfig) SearchPathsString() string {
	s := "["
	logger.AssertTrue(len(cfg.SearchPaths) >= 1)
	for _, sp := range cfg.SearchPaths {
		s += "\"" + sp + "\", "
	}
	return s[0:len(s)-2] + "]"
}

// SetVersion sets all executables to the defaults associated to the given Thread version number.
// The given defaultConfig is used as a base to derive the versioned executables from.
func (cfg *ExecutableConfig) SetVersion(version string, defaultConfig *ExecutableConfig) {
	logger.AssertTrue(strings.HasPrefix(version, "v1") && len(version) >= 3 && len(version) <= 4)
	cfg.Ftd = defaultConfig.Ftd + "_" + version
	cfg.Mtd = defaultConfig.Mtd + "_" + version
	cfg.Br = defaultConfig.Br // BR is currently not adapted to versions.
	cfg.Version = version
}

func isFile(exePath string) bool {
	if fileInfo, err := os.Stat(exePath); err == nil {
		return !fileInfo.IsDir()
	}
	return false
}

// FindExecutable returns a full path to the named executable, by searching in standard
// search paths if needed. If the given exeName is already a full path itself, it will be returned itself.
func (cfg *ExecutableConfig) FindExecutable(exeName string) string {
	if filepath.IsAbs(exeName) || exeName[0] == '.' {
		return exeName
	}
	for _, sp := range cfg.SearchPaths {
		exePath := filepath.Join(sp, exeName)
		if isFile(exePath) {
			if filepath.IsAbs(exePath) || exePath[0] == '.' {
				return exePath
			}
			return "./" + exePath
		}
	}
	// if not found, try to relay on OS $PATH to find the executables.
	return exeName
}

// FindExecutableBasedOnConfig gets the executable based on NodeConfig information.
func (cfg *ExecutableConfig) FindExecutableBasedOnConfig(nodeCfg *NodeConfig) string {
	if len(nodeCfg.ExecutablePath) > 0 {
		return nodeCfg.ExecutablePath
	}
	exeName := cfg.Ftd
	if nodeCfg.IsMtd {
		exeName = cfg.Mtd
	}
	if nodeCfg.IsBorderRouter {
		exeName = cfg.Br
	} else if len(nodeCfg.Version) > 0 {
		exeName += "_" + nodeCfg.Version
	}

	return cfg.FindExecutable(exeName)
}

func NewNodeAutoPlacer() *NodeAutoPlacer {
	return &NodeAutoPlacer{
		Xref:            100,
		Yref:            100,
		Xmax:            1450,
		X:               100,
		Y:               100,
		Z:               0,
		NodeDeltaCoarse: 100,
		NodeDeltaFine:   40,
		fineCount:       0,
		isReset:         true,
	}
}

// UpdateXReference updates the reference X position of the NodeAutoPlacer to 'x'. It starts placing from there.
func (nap *NodeAutoPlacer) UpdateXReference(x int) {
	nap.Xref = x
	nap.X = x
}

// UpdateYReference updates the reference Y position of the NodeAutoPlacer to 'y'. It starts placing from there.
func (nap *NodeAutoPlacer) UpdateYReference(y int) {
	nap.Yref = y
	nap.Y = y
}

// UpdateReference updates the reference position of the NodeAutoPlacer to 'x', 'y'. It starts placing from there.
func (nap *NodeAutoPlacer) UpdateReference(x, y, z int) {
	nap.Xref = x
	nap.X = x
	nap.Yref = y
	nap.Y = y
	nap.Z = z
	nap.isReset = false
}

// NextNodePosition lets the autoplacer pick the next position for a new node to be placed.
func (nap *NodeAutoPlacer) NextNodePosition(isBelowParent bool) (int, int, int) {
	var x, y, z int

	if isBelowParent {
		y = nap.Y + nap.NodeDeltaCoarse/2
		x = nap.X + nap.fineCount*nap.NodeDeltaFine - nap.NodeDeltaFine
		nap.fineCount++
	} else {
		if !nap.isReset {
			nap.X += nap.NodeDeltaCoarse
			if nap.X > nap.Xmax {
				nap.X = nap.Xref
				nap.Y += nap.NodeDeltaCoarse
			}
		}
		nap.isReset = false
		nap.fineCount = 0
		x = nap.X
		y = nap.Y
	}
	z = nap.Z
	return x, y, z
}

// ReuseNextNodePosition instructs the autoplacer to re-use the NextNodePosition() that was given out in the
// last call to this method.
func (nap *NodeAutoPlacer) ReuseNextNodePosition() {
	nap.isReset = true
}
