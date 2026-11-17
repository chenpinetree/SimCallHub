package sip

// G.711 PCMU (u-law) and PCMA (a-law) codecs.
// Converts between 16-bit linear PCM (8 kHz mono, as produced by Quectel EC20 AT+QPCMV)
// and standard VoIP 8-bit companded audio (G.711u / G.711a).

var (
	linearToUlawTable [65536]byte
	ulawToLinearTable [256]int16
	linearToAlawTable [65536]byte
	alawToLinearTable [256]int16
)

func init() {
	// Initialize G.711 u-law tables
	for i := 0; i < 65536; i++ {
		linear := int16(i)
		linearToUlawTable[i] = encodeUlawSample(linear)
	}
	for i := 0; i < 256; i++ {
		ulawToLinearTable[i] = decodeUlawSample(byte(i))
	}

	// Initialize G.711 a-law tables
	for i := 0; i < 65536; i++ {
		linear := int16(i)
		linearToAlawTable[i] = encodeAlawSample(linear)
	}
	for i := 0; i < 256; i++ {
		alawToLinearTable[i] = decodeAlawSample(byte(i))
	}
}

const (
	uBias = 0x84
	uClip = 32635
)

func encodeUlawSample(sample int16) byte {
	sign := byte(0)
	if sample < 0 {
		sample = -sample
		sign = 0x80
	}
	if sample > uClip {
		sample = uClip
	}
	sample += uBias

	exponent := byte(7)
	for expMask := int16(0x4000); (sample&expMask) == 0 && exponent > 0; expMask >>= 1 {
		exponent--
	}
	mantissa := byte((sample >> (exponent + 3)) & 0x0F)
	return ^(sign | (exponent << 4) | mantissa)
}

func decodeUlawSample(ulaw byte) int16 {
	ulaw = ^ulaw
	sign := ulaw & 0x80
	exponent := (ulaw >> 4) & 0x07
	mantissa := ulaw & 0x0F
	sample := int16((mantissa << 3) + uBias)
	sample <<= exponent
	sample -= uBias
	if sign != 0 {
		return -sample
	}
	return sample
}

func encodeAlawSample(sample int16) byte {
	sign := byte(0)
	if sample < 0 {
		sample = -sample
		sign = 0x80
	}
	if sample > 32767 {
		sample = 32767
	}

	exponent := byte(7)
	for expMask := int16(0x4000); (sample&expMask) == 0 && exponent > 0; expMask >>= 1 {
		exponent--
	}

	var mantissa byte
	if exponent == 0 {
		mantissa = byte((sample >> 4) & 0x0F)
	} else {
		mantissa = byte((sample >> (exponent + 3)) & 0x0F)
	}
	return (sign | (exponent << 4) | mantissa) ^ 0xD5
}

func decodeAlawSample(alaw byte) int16 {
	alaw ^= 0xD5
	sign := alaw & 0x80
	exponent := (alaw >> 4) & 0x07
	mantissa := alaw & 0x0F

	var sample int16
	if exponent == 0 {
		sample = int16(mantissa)<<4 + 8
	} else {
		sample = (int16(mantissa)<<4 + 0x108) << (exponent - 1)
	}
	if sign != 0 {
		return -sample
	}
	return sample
}

// LinearToUlaw converts 16-bit little-endian linear PCM to G.711 u-law (PCMU).
// Input length must be even. Output length is len(pcm16) / 2.
func LinearToUlaw(pcm16 []byte) []byte {
	n := len(pcm16) / 2
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		sample := uint16(pcm16[2*i]) | (uint16(pcm16[2*i+1]) << 8)
		out[i] = linearToUlawTable[sample]
	}
	return out
}

// UlawToLinear converts G.711 u-law (PCMU) to 16-bit little-endian linear PCM.
// Output length is len(ulaw) * 2.
func UlawToLinear(ulaw []byte) []byte {
	out := make([]byte, len(ulaw)*2)
	for i, b := range ulaw {
		sample := ulawToLinearTable[b]
		out[2*i] = byte(sample)
		out[2*i+1] = byte(sample >> 8)
	}
	return out
}

// LinearToAlaw converts 16-bit little-endian linear PCM to G.711 a-law (PCMA).
func LinearToAlaw(pcm16 []byte) []byte {
	n := len(pcm16) / 2
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		sample := uint16(pcm16[2*i]) | (uint16(pcm16[2*i+1]) << 8)
		out[i] = linearToAlawTable[sample]
	}
	return out
}

// AlawToLinear converts G.711 a-law (PCMA) to 16-bit little-endian linear PCM.
func AlawToLinear(alaw []byte) []byte {
	out := make([]byte, len(alaw)*2)
	for i, b := range alaw {
		sample := alawToLinearTable[b]
		out[2*i] = byte(sample)
		out[2*i+1] = byte(sample >> 8)
	}
	return out
}
