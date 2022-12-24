// Copyright (c) 2020-2022, The OTNS Authors.
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

	"github.com/openthread/ot-ns/threadconst"
)

const (
	DefaultNetworkName = "OTSIM"
	DefaultNetworkKey  = "00112233445566778899aabbccddeeff"
	DefaultPanid       = 0xface
	DefaultChannel     = 11
)

// The init script is an array of commands, sent to a new node.
var DefaultNodeInitScript = []string{
	"networkname " + DefaultNetworkName,
	"networkkey " + DefaultNetworkKey,
	fmt.Sprintf("panid 0x%x", DefaultPanid),
	fmt.Sprintf("channel %d", DefaultChannel),
	"routerselectionjitter 1",
	"ifconfig up",
	"thread start",
}

type Config struct {
	InitScript     []string
	OtCliPath      string
	OtBrPath       string
	Speed          float64
	ReadOnly       bool
	RawMode        bool
	Real           bool
	AutoGo         bool
	DispatcherHost string
	DispatcherPort int
	DumpPackets    bool
	RadioModel     string
}

func DefaultConfig() *Config {
	return &Config{
		InitScript:     DefaultNodeInitScript,
		Speed:          1,
		ReadOnly:       false,
		RawMode:        false,
		OtCliPath:      "./ot-cli-ftd",
		OtBrPath:       "./otbr-sim.sh",
		Real:           false,
		AutoGo:         true,
		DispatcherHost: "localhost",
		DispatcherPort: threadconst.InitialDispatcherPort,
		RadioModel:     "Ideal_Rssi",
	}
}
