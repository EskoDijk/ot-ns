package radiomodel

import "math"

const (
	defaultSymbolsPerOctet = 2
)

// Parameters used for a simulated IEEE 802.15.4 PHY radio.
type phyParameters struct {
	kbps              float64
	symbolTimeUs      float64
	symbolsPerOctet   float64
	aMaxSifsFrameSize uint
	phyHeaderSize     uint
	ccaTimeUs         uint64
	aifsTimeUs        uint64
	lifsTimeUs        uint64
	sifsTimeUs        uint64
	turnaroundTimeUs  uint64
}

func (phy *phyParameters) adaptKbps(kbps float64) {
	symbTimeUs := 8000.0 / (kbps * phy.symbolsPerOctet)
	phy.symbolTimeUs = symbTimeUs
	phy.kbps = 8000.0 / (symbTimeUs * phy.symbolsPerOctet)
}

func toUs(t float64) uint64 {
	return uint64(math.Round(t))
}

// IEEE 802.15.4-2015 O-QPSK generic PHY, based on parameter symbol time.
func getPhyParametersOQPSK(symbTimeUs float64) phyParameters {
	return phyParameters{
		kbps:              8000.0 / (symbTimeUs * defaultSymbolsPerOctet),
		symbolTimeUs:      symbTimeUs,
		symbolsPerOctet:   defaultSymbolsPerOctet,
		aMaxSifsFrameSize: 18,
		phyHeaderSize:     6,
		ccaTimeUs:         toUs(symbTimeUs * 8),
		aifsTimeUs:        toUs(symbTimeUs * 12),
		lifsTimeUs:        toUs(symbTimeUs * 20),
		sifsTimeUs:        toUs(symbTimeUs * 12),
		turnaroundTimeUs:  toUs(symbTimeUs * 6),
	}
}

// Thread Link Type 2 PHY, O-QPSK 2.4 Ghz, 250 kbps
func getPhyParameters_TL2() phyParameters {
	return getPhyParametersOQPSK(16.0)
}

// O-QPSK Sub-GHz (868 MHz) 100 kbps
func getPhyParameters_OQPSK868() phyParameters {
	return getPhyParametersOQPSK(40.0)
}
