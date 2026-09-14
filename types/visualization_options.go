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

package types

// VisualizationOptions defines which items the visualizer(s) show, and in which style.
type VisualizationOptions struct {
	BroadcastMessage bool
	UnicastMessage   bool
	AckMessage       bool
	RouterTable      bool
	ChildTable       bool
	PartitionId      bool   // show the partition ID of each node (as a colored dot)
	Skin             string // visual style of the network visualization, one of VisualizationSkins
}

const (
	VisualizationSkinThread  = "thread"  // Thread network diagram style
	VisualizationSkinClassic = "classic" // the original OTNS style
	DefaultVisualizationSkin = VisualizationSkinThread
)

// VisualizationSkins lists the skins of the web visualization; each must be implemented in
// web/site/js/vis/skins/ and registered in web/site/js/vis/skins/index.js.
var VisualizationSkins = []string{VisualizationSkinThread, VisualizationSkinClassic}

func IsVisualizationSkin(name string) bool {
	for _, s := range VisualizationSkins {
		if s == name {
			return true
		}
	}
	return false
}

func DefaultVisualizationOptions() VisualizationOptions {
	return VisualizationOptions{
		BroadcastMessage: true,
		UnicastMessage:   true,
		AckMessage:       false,
		RouterTable:      true,
		ChildTable:       true,
		PartitionId:      true,
		Skin:             DefaultVisualizationSkin,
	}
}
