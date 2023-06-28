// Copyright (c) 2023, The OTNS Authors.
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
	"path/filepath"

	"github.com/openthread/ot-ns/types"
	"github.com/simonlingoogle/go-simplelogger"
)

// cleanTmpDir cleans the tmp dir by removing only log/flash/temp/socket files associated to current simulation ID
func cleanTmpDir(simulationId int) error {
	err := types.RemoveAllFiles(fmt.Sprintf("%s/%d_*.*", types.GetTmpDir(), simulationId))
	return err
}

// getPtyFilePath gets the absolute file path of the PTY file associated to node nodeId.
func getPtyFilePath(simulationId int, nodeId int) string {
	simplelogger.AssertTrue(simulationId >= 0)
	simplelogger.AssertTrue(nodeId > 0)
	p, err := filepath.Abs(filepath.Join(types.GetTmpDir(), fmt.Sprintf("%d_%d.pty", simulationId, nodeId)))
	if err != nil {
		simplelogger.Panicf("getPtyFilePath: %v", err)
	}
	return p
}
