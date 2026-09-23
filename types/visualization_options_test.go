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

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVisualizationSkinPresets(t *testing.T) {
	for _, p := range VisualizationSkinPresets {
		assert.True(t, IsVisualizationSkin(p.Skin), p.Name)
		for k := range p.Options {
			var opts VisualizationOptions
			assert.True(t, opts.SetOption(k, true), p.Name+": unknown option "+k)
		}
	}
	assert.NotNil(t, FindVisualizationSkinPreset(DefaultVisualizationSkinPreset))
	assert.Nil(t, FindVisualizationSkinPreset("nonexistent"))
	assert.Equal(t, len(VisualizationSkinPresets), len(VisualizationSkinPresetNames()))

	opts := DefaultVisualizationOptions()
	assert.Equal(t, "thread", opts.SkinPreset)
	assert.Equal(t, VisualizationSkinThread, opts.Skin)
	assert.False(t, opts.PartitionId)
	assert.False(t, opts.AckMessage)

	FindVisualizationSkinPreset("clas_ack").Apply(&opts)
	assert.Equal(t, "clas_ack", opts.SkinPreset)
	assert.Equal(t, VisualizationSkinClassic, opts.Skin)
	assert.True(t, opts.PartitionId)
	assert.True(t, opts.AckMessage)

	FindVisualizationSkinPreset("thread").Apply(&opts)
	assert.Equal(t, VisualizationSkinThread, opts.Skin)
	assert.False(t, opts.PartitionId)
	assert.True(t, opts.AckMessage) // not part of the preset: unchanged

	assert.False(t, opts.SetOption("nope", true))
}
