// Copyright (c) 2024, The OTNS Authors.
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

package prng

import (
	"math/rand"
	"time"

	"github.com/openthread/ot-ns/logger"
)

type RandomSeed int64

var newNodeRandSeedGenerator *rand.Rand
var newRadioModelRandSeedGenerator *rand.Rand
var newFailTimeRandGenerator *rand.Rand
var newRandomProbGenerator *rand.Rand
var cnt uint64 = 0

func PrngInit(rootSeed int64) {
	if rootSeed == 0 {
		rootSeed = time.Now().UnixNano() // TODO: from go 1.20 onwards, this is not needed and deprecated.
	}
	rand.Seed(rootSeed)

	newNodeRandSeedGenerator = rand.New(rand.NewSource(rootSeed + int64(rand.Intn(1e10)))) // TODO check which range is possible
	newRadioModelRandSeedGenerator = rand.New(rand.NewSource(rootSeed + int64(rand.Intn(1e10))))
	newFailTimeRandGenerator = rand.New(rand.NewSource(rootSeed + int64(rand.Intn(1e10))))
	newRandomProbGenerator = rand.New(rand.NewSource(rootSeed + int64(rand.Intn(1e10))))
}

// NewNodeRandomSeed generates unique random-seeds for newly created nodes.
func NewNodeRandomSeed() int32 {
	return newNodeRandSeedGenerator.Int31()
}

// NewRadioModelRandomSeed generates unique random-seeds for newly created radio models.
func NewRadioModelRandomSeed() RandomSeed {
	return RandomSeed(newRadioModelRandSeedGenerator.Int63())
}

func NewFailTime(failStartTimeMax int) uint64 {
	return uint64(newFailTimeRandGenerator.Intn(failStartTimeMax))
}

func NewRandomProb() float64 {
	cnt++
	r := newRandomProbGenerator.Float64()
	logger.Debugf("Generated n=%d: %f", cnt, r)
	return r
}
