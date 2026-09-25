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
	SkinPreset       string // name of the last selected skin preset, see VisualizationSkinPresets
}

const (
	VisualizationSkinThread   = "thread"   // Thread network diagram style
	VisualizationSkinClassic  = "classic"  // the original OTNS style
	VisualizationSkinOffice3D = "office3d" // 3D view of the field, classic node colors
	DefaultVisualizationSkin  = VisualizationSkinClassic
)

// VisualizationSkins lists the skins of the web visualization; each must be implemented in
// web/site/js/vis/skins/ and registered in web/site/js/vis/skins/index.js.
var VisualizationSkins = []string{VisualizationSkinThread, VisualizationSkinClassic, VisualizationSkinOffice3D}

func IsVisualizationSkin(name string) bool {
	for _, s := range VisualizationSkins {
		if s == name {
			return true
		}
	}
	return false
}

// VisualizationSkinPreset is a named combination of a skin and values of visualization options,
// selectable with the CLI command 'cv skin <name>'. Selecting a preset applies its options; all
// other options keep their current value.
type VisualizationSkinPreset struct {
	Name    string          // preset name, as used in 'cv skin <name>'
	Skin    string          // one of VisualizationSkins
	Options map[string]bool // option values to apply, keyed by the 'cv' option name: bro, uni, ack, rtb, ctb, pid
}

// VisualizationSkinPresets defines the available skin presets. To add one: append it here,
// and document it in cli/README.md ('cv' command) and GUIDE.md.
var VisualizationSkinPresets = []VisualizationSkinPreset{
	{Name: "thread", Skin: VisualizationSkinThread, Options: map[string]bool{"pid": false, "ack": false}},
	{Name: "thread+", Skin: VisualizationSkinThread, Options: map[string]bool{"pid": true, "ack": false}},
	{Name: "classic", Skin: VisualizationSkinClassic, Options: map[string]bool{"pid": true, "ack": false}},
	{Name: "clas_ack", Skin: VisualizationSkinClassic, Options: map[string]bool{"pid": true, "ack": true}},
	{Name: "space", Skin: VisualizationSkinOffice3D, Options: map[string]bool{"pid": true, "ack": false}},
}

const DefaultVisualizationSkinPreset = "classic"

// FindVisualizationSkinPreset returns the preset with the given name, or nil if it doesn't exist.
func FindVisualizationSkinPreset(name string) *VisualizationSkinPreset {
	for i := range VisualizationSkinPresets {
		if VisualizationSkinPresets[i].Name == name {
			return &VisualizationSkinPresets[i]
		}
	}
	return nil
}

// VisualizationSkinPresetNames returns the names of all presets, in definition order.
func VisualizationSkinPresetNames() []string {
	names := make([]string, 0, len(VisualizationSkinPresets))
	for _, p := range VisualizationSkinPresets {
		names = append(names, p.Name)
	}
	return names
}

// Apply writes the preset's skin and option values into opts.
func (p *VisualizationSkinPreset) Apply(opts *VisualizationOptions) {
	opts.Skin = p.Skin
	opts.SkinPreset = p.Name
	for k, v := range p.Options {
		opts.SetOption(k, v)
	}
}

// SetOption sets the option with the given 'cv' option name (bro, uni, ack, rtb, ctb, pid).
// It returns false if the name is unknown.
func (opts *VisualizationOptions) SetOption(name string, on bool) bool {
	switch name {
	case "bro":
		opts.BroadcastMessage = on
	case "uni":
		opts.UnicastMessage = on
	case "ack":
		opts.AckMessage = on
	case "rtb":
		opts.RouterTable = on
	case "ctb":
		opts.ChildTable = on
	case "pid":
		opts.PartitionId = on
	default:
		return false
	}
	return true
}

func DefaultVisualizationOptions() VisualizationOptions {
	opts := VisualizationOptions{
		BroadcastMessage: true,
		UnicastMessage:   true,
		AckMessage:       false,
		RouterTable:      true,
		ChildTable:       true,
		PartitionId:      false,
		Skin:             DefaultVisualizationSkin,
	}
	FindVisualizationSkinPreset(DefaultVisualizationSkinPreset).Apply(&opts) // may override the above
	return opts
}
