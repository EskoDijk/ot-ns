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

package dispatcher

import (
	"testing"

	"math/rand"
	"time"

	. "github.com/openthread/ot-ns/types"
	"github.com/openthread/ot-ns/visualize"
	"github.com/stretchr/testify/assert"
)

type mockDispatcherCallback struct {
}

func (m mockDispatcherCallback) OnNodeFail(nodeid NodeId) {
}

func (m mockDispatcherCallback) OnNodeRecover(nodeid NodeId) {
}

func (m mockDispatcherCallback) OnUartWrite(nodeid NodeId, data []byte) {
}

func (m mockDispatcherCallback) OnUartWritesComplete(nodeid NodeId) {
}

func (m mockDispatcherCallback) OnLogMessage(nodeid NodeId, level WatchLogLevel, nodeIsWatched bool, msg string) {
}

func (m mockDispatcherCallback) OnNextEventTime(curTimeUs uint64, nextTimeUs uint64) {
}

func TestFailureCtrlNonFailure(t *testing.T) {
	node1 := &Node{
		Id: 0x1,
	}
	node1.failureCtrl = newFailureCtrl(node1, NonFailTime)

	for i := 0; i < 10; i++ {
		oldTime := node1.CurTime
		node1.CurTime += 1000000
		node1.failureCtrl.OnTimeAdvanced(oldTime)
		assert.False(t, node1.IsFailed())
	}

	node1.isFailed = true
	for i := 0; i < 10; i++ {
		oldTime := node1.CurTime
		node1.CurTime += 1000000
		node1.failureCtrl.OnTimeAdvanced(oldTime)
		assert.True(t, node1.IsFailed())
	}

	node1.isFailed = false
	for i := 0; i < 10; i++ {
		oldTime := node1.CurTime
		node1.CurTime += 1000000
		node1.failureCtrl.OnTimeAdvanced(oldTime)
		assert.False(t, node1.IsFailed())
	}
}

func TestFailureCtrlFailingHalfOfTheTime(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	node1 := &Node{
		Id: 0x1,
	}
	ft := FailTime{
		FailDuration: 30 * 1e6,
		FailInterval: 60 * 1e6,
	}
	node1.failureCtrl = newFailureCtrl(node1, ft)
	node1.D = &Dispatcher{
		cbHandler: &mockDispatcherCallback{},
		vis:       visualize.NewNopVisualizer(),
	}

	failCount := 0
	worksCount := 0

	// simulate a 10-hour period
	for i := 0; i < 360000; i++ {
		oldTime := node1.CurTime
		node1.CurTime += 100000
		node1.D.CurTime = node1.CurTime
		node1.failureCtrl.OnTimeAdvanced(oldTime)
		if node1.IsFailed() {
			failCount++
		} else {
			worksCount++
		}
	}

	// verify that failure percentage is roughly 50%
	failPerc := float64(failCount) / float64(failCount+worksCount)
	assert.True(t, failPerc > 0.46)
	assert.True(t, failPerc < 0.54)
}

func TestFailureCtrlFailingMostOfTheTime(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	node1 := &Node{
		Id: 0x1,
	}
	ft := FailTime{
		FailDuration: 9 * 1e6,
		FailInterval: 10 * 1e6,
	}
	node1.failureCtrl = newFailureCtrl(node1, ft)
	node1.D = &Dispatcher{
		cbHandler: &mockDispatcherCallback{},
		vis:       visualize.NewNopVisualizer(),
	}

	failCount := 0
	worksCount := 0

	// simulate a 10-hour period
	for i := 0; i < 360000; i++ {
		oldTime := node1.CurTime
		node1.CurTime += 100000
		node1.D.CurTime = node1.CurTime
		node1.failureCtrl.OnTimeAdvanced(oldTime)
		if node1.IsFailed() {
			failCount++
		} else {
			worksCount++
		}
	}

	// verify that failure percentage is roughly 90%
	failPerc := float64(failCount) / float64(failCount+worksCount)
	assert.True(t, failPerc > 0.88)
	assert.True(t, failPerc < 0.92)
}

func TestFailureCtrlAddedOnAlreadyFailedNode(t *testing.T) {
	node1 := &Node{
		Id: 0x1,
	}
	node1.D = &Dispatcher{
		cbHandler: &mockDispatcherCallback{},
		vis:       visualize.NewNopVisualizer(),
	}
	ft := FailTime{
		FailDuration: 3 * 1e6,
		FailInterval: 35 * 1e6,
	}
	node1.failureCtrl = newFailureCtrl(node1, ft)
	node1.isFailed = true
	for i := 0; i < 10; i++ {
		oldTime := node1.CurTime
		node1.CurTime += 100000
		node1.failureCtrl.OnTimeAdvanced(oldTime)
	}
}
