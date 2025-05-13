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

import "math"

// computeIndoorRssi computes the RSSI for a receiver at distance dist, using a simple indoor exponent loss model.
// See https://en.wikipedia.org/wiki/ITU_model_for_indoor_attenuation
func computeIndoorRssiItu(dist float64, txPower DbValue, modelParams *RadioModelParams) DbValue {
	pathloss := 0.0
	distMeters := dist * modelParams.MeterPerUnit
	if distMeters >= 0.01 {
		pathloss = modelParams.ExponentDb*math.Log10(distMeters) + modelParams.FixedLossDb
		if pathloss < 0.0 {
			pathloss = 0.0
		}
	}
	rssi := txPower - pathloss
	return rssi
}

// computeIndoorRssi3gpp computes the RSSI for a receiver at distance dist, using the Indoor/Office 3GPP
// model defined in 3GPP TR 38.901 V17.0.0, Table 7.4.1-1: Pathloss models.
func computeIndoorRssi3gpp(dist float64, txPower DbValue, modelParams *RadioModelParams) DbValue {
	pathloss := 0.0
	distMeters := dist * modelParams.MeterPerUnit
	if distMeters >= 0.01 {
		pathloss = modelParams.ExponentDb*math.Log10(distMeters) + modelParams.FixedLossDb
		if pathloss < 0.0 {
			pathloss = 0.0
		}
		if modelParams.NlosExponentDb > 0.0 {
			pathlossNLOS := modelParams.NlosExponentDb*math.Log10(distMeters) + modelParams.NlosFixedLossDb
			pathloss = math.Max(pathloss, pathlossNLOS)
		}
	}
	rssi := txPower - pathloss
	return rssi
}
