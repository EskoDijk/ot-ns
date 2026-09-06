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

package energy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Energy results must land in the simulation output directory, named like every other artifact
// (<outputDir>/<simId>_<name>), rather than in a directory relative to the process working dir.
func TestSaveEnergyDataToFileUsesOutputDir(t *testing.T) {
	outputDir := t.TempDir()
	ea := NewEnergyAnalyser(outputDir, 3)
	ea.AddNode(1, 0)
	ea.StoreNetworkEnergy(1000)

	ea.SaveEnergyDataToFile("", 1000)

	assert.FileExists(t, filepath.Join(outputDir, "3_energy.txt"))
	assert.FileExists(t, filepath.Join(outputDir, "3_energy_nodes.txt"))

	// nothing may be created outside the output directory
	assert.NoDirExists(t, "energy_results")
}

// An explicit name and the simulation title are both prefixed with the simulation id.
func TestSaveEnergyDataToFileNaming(t *testing.T) {
	outputDir := t.TempDir()
	ea := NewEnergyAnalyser(outputDir, 0)
	ea.AddNode(1, 0)
	ea.StoreNetworkEnergy(1000)

	ea.SaveEnergyDataToFile("run17", 1000)
	assert.FileExists(t, filepath.Join(outputDir, "0_run17.txt"))
	assert.FileExists(t, filepath.Join(outputDir, "0_run17_nodes.txt"))

	ea.SetTitle("mytitle")
	ea.SaveEnergyDataToFile("", 1000)
	assert.FileExists(t, filepath.Join(outputDir, "0_mytitle.txt"))
}

// A name that would escape the output directory is rejected, and writes nothing.
func TestSaveEnergyDataToFileRejectsPathSeparators(t *testing.T) {
	outputDir := t.TempDir()
	ea := NewEnergyAnalyser(outputDir, 0)
	ea.AddNode(1, 0)
	ea.StoreNetworkEnergy(1000)

	for _, name := range []string{"..", ".", "sub/dir", "..\\win"} {
		ea.SaveEnergyDataToFile(name, 1000)
	}

	entries, err := os.ReadDir(outputDir)
	assert.NoError(t, err)
	assert.Empty(t, entries, "a rejected name must not create any file")
}
