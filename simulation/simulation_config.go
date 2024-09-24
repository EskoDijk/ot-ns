// Copyright (c) 2020-2023, The OTNS Authors.
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

	. "github.com/openthread/ot-ns/types"
)

const (
	DefaultChannel         = 11
	DefaultChannelMask     = 0x07fff800
	DefaultExtPanid        = "dead00beef00cafe"
	DefaultMeshLocalPrefix = "fdde:ad00:beef:0::"
	DefaultNetworkKey      = "00112233445566778899aabbccddeeff"
	DefaultNetworkName     = "otns"
	DefaultPanid           = 0xface
	DefaultPskc            = "3aa55f91ca47d1e4e71a08cb35e91591"
	DefaultSecurityPolicy  = "672 onrc"
)

// DefaultNodeInitScript is an array of commands, sent to a new node by default (unless changed).
var DefaultNodeInitScript = []string{
	"networkname " + DefaultNetworkName,
	"networkkey " + DefaultNetworkKey,
	fmt.Sprintf("panid 0x%x", DefaultPanid),
	fmt.Sprintf("channel %d", DefaultChannel),
	//"routerselectionjitter 1", // jitter can be set to '1' to speed up network formation for realtime tests.
	"ifconfig up",
	"thread start",
}

type Config struct {
	InitScript       []string
	ExeConfig        ExecutableConfig
	ExeConfigDefault ExecutableConfig
	NewNodeConfig    NodeConfig
	Speed            float64
	ReadOnly         bool
	RawMode          bool
	Real             bool
	AutoGo           bool
	DispatcherHost   string
	DispatcherPort   int
	DumpPackets      bool
	RadioModel       string
	Id               int
	Channel          ChannelId
	LogLevel         WatchLogLevel
}

func DefaultConfig() *Config {
	return &Config{
		InitScript:       DefaultNodeInitScript,
		ExeConfig:        DefaultExecutableConfig,
		ExeConfigDefault: DefaultExecutableConfig,
		NewNodeConfig:    DefaultNodeConfig(),
		Speed:            1,
		ReadOnly:         false,
		RawMode:          false,
		Real:             false,
		AutoGo:           true,
		DispatcherHost:   "localhost",
		DispatcherPort:   InitialDispatcherPort,
		RadioModel:       "Ideal_Rssi",
		Id:               0,
		Channel:          DefaultChannel,
	}
}
