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

package radiomodel

import (
	"github.com/simonlingoogle/go-simplelogger"
	"math"
	"math/rand"
)

type shadowFading struct {
	rndSeed int64
}

func newShadowFading() *shadowFading {
	sf := &shadowFading{
		rndSeed: rand.Int63(),
	}
	return sf
}

// computeShadowFading calculates shadow fading (SF) for a radio link based on a simple random process.
// It models a fixed, position-dependent radio signal power attenuation (SF>0) or increase (SF<0) due to multipath effects
// and static obstacles. In the dB domain it is modeled as a normal distribution (mu=0, sigma).
// See https://en.wikipedia.org/wiki/Fading and 3GPP TR 38.901 V17.0.0, section 7.4.1 and 7.4.4, and
// Table 7.5-6 Part-2.
// TODO: implement the autocorrelation of SF over a correlation length d_cor = 6 m (NLOS case)
func (sf *shadowFading) computeShadowFading(src *RadioNode, dst *RadioNode, dist float64, params *IndoorModelParams) DbValue {
	simplelogger.AssertTrue(src.RadioRange == dst.RadioRange)
	if params.ShadowFadingSigmaDb <= 0 {
		return 0.0
	}

	// calc node positions in rounded grid units of 1 m
	x1 := int64(math.Round(src.X / src.RadioRange * params.RangeInMeters))
	y1 := int64(math.Round(src.Y / src.RadioRange * params.RangeInMeters))
	x2 := int64(math.Round(dst.X / src.RadioRange * params.RangeInMeters))
	y2 := int64(math.Round(dst.Y / src.RadioRange * params.RangeInMeters))
	xL := x2
	yL := y2
	xR := x1
	yR := y1

	// use left-most node (and in case of doubt, top-most) - screen coordinates
	if x1 < x2 || (x1 == x2 && y1 < y2) {
		xL = x1
		yL = y1
		xR = x2
		yR = y2
	}

	// give each (xL,yL) & (xR,yR) coordinate combination its own fixed seed-value.
	seed := sf.rndSeed + xL + yL<<16 + xR<<32 + yR<<48
	rndSource := rand.NewSource(seed)
	rnd := rand.New(rndSource)
	// draw a single (reproducible) random number based on the position coordinates.
	v := rnd.NormFloat64() * params.ShadowFadingSigmaDb
	return v
}
