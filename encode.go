package qr

import "errors"

// mode is the QR data mode; the numeric values are the 4-bit mode indicators
// defined by ISO/IEC 18004.
type mode int

const (
	modeNumeric mode = 0b0001
	modeAlpha   mode = 0b0010
	modeByte    mode = 0b0100
)

// alphaSet is the 45-symbol alphanumeric character set in value order
// (ISO/IEC 18004 Table 5) — the single definition the encoder, the decoder,
// and the tests all share.
const alphaSet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

// alphaValue maps a byte to its value in the alphanumeric set, or -1.
var alphaValue [256]int

func init() {
	for i := range alphaValue {
		alphaValue[i] = -1
	}
	for i := 0; i < len(alphaSet); i++ {
		alphaValue[alphaSet[i]] = i
	}
}

// bitBuffer accumulates a big-endian bit stream.
type bitBuffer struct {
	bytes []byte
	nbits int
}

func (b *bitBuffer) len() int { return b.nbits }

func (b *bitBuffer) appendBit(bit int) {
	if b.nbits%8 == 0 {
		b.bytes = append(b.bytes, 0)
	}
	if bit&1 != 0 {
		b.bytes[b.nbits/8] |= 1 << uint(7-b.nbits%8)
	}
	b.nbits++
}

// appendBits appends the low n bits of v, most significant first.
func (b *bitBuffer) appendBits(v uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		b.appendBit(int((v >> uint(i)) & 1))
	}
}

// detectMode chooses the most compact mode that can represent content.
func detectMode(content string) mode {
	numeric, alpha := true, true
	for i := 0; i < len(content); i++ {
		c := content[i]
		if c < '0' || c > '9' {
			numeric = false
		}
		if alphaValue[c] < 0 {
			alpha = false
		}
	}
	switch {
	case numeric:
		return modeNumeric
	case alpha:
		return modeAlpha
	default:
		return modeByte
	}
}

// charCountBits returns the length of the character-count indicator for a
// mode at a given version (ISO/IEC 18004 Table 3).
func charCountBits(version int, m mode) int {
	switch {
	case version <= 9:
		switch m {
		case modeNumeric:
			return 10
		case modeAlpha:
			return 9
		default:
			return 8
		}
	case version <= 26:
		switch m {
		case modeNumeric:
			return 12
		case modeAlpha:
			return 11
		default:
			return 16
		}
	default:
		switch m {
		case modeNumeric:
			return 14
		case modeAlpha:
			return 13
		default:
			return 16
		}
	}
}

// dataBitCount returns the number of bits the payload itself occupies in a
// mode (excluding mode indicator and character count). The per-count math
// lives in dataBitCountN, shared with the decoder's claim check.
func dataBitCount(content string, m mode) int {
	return dataBitCountN(len(content), m)
}

// encodeSegment writes the mode indicator, character count and payload for
// content into buf using the given mode and version.
func encodeSegment(buf *bitBuffer, content string, m mode, version int) {
	buf.appendBits(uint32(m), 4)
	buf.appendBits(uint32(len(content)), charCountBits(version, m))
	switch m {
	case modeNumeric:
		i := 0
		for ; i+3 <= len(content); i += 3 {
			buf.appendBits(uint32(atoi3(content[i:i+3])), 10)
		}
		switch len(content) - i {
		case 2:
			buf.appendBits(uint32(atoi3(content[i:i+2])), 7)
		case 1:
			buf.appendBits(uint32(content[i]-'0'), 4)
		}
	case modeAlpha:
		i := 0
		for ; i+2 <= len(content); i += 2 {
			v := alphaValue[content[i]]*45 + alphaValue[content[i+1]]
			buf.appendBits(uint32(v), 11)
		}
		if i < len(content) {
			buf.appendBits(uint32(alphaValue[content[i]]), 6)
		}
	default:
		for i := 0; i < len(content); i++ {
			buf.appendBits(uint32(content[i]), 8)
		}
	}
}

func atoi3(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

var errTooLong = errors.New("qr: content too long for any version at this error-correction level")

// chooseVersion returns the smallest version (1..40) whose data capacity at
// the given level holds content in mode m.
func chooseVersion(content string, m mode, level Level) (int, error) {
	payload := dataBitCount(content, m)
	for v := 1; v <= 40; v++ {
		if 4+charCountBits(v, m)+payload <= dataCodewords(v, level)*8 {
			return v, nil
		}
	}
	return 0, errTooLong
}

// buildCodewords produces the final data codeword stream for content: mode
// segment, terminator, bit padding to a byte boundary and alternating pad
// bytes to fill the version's data capacity.
func buildCodewords(content string, m mode, version int, level Level) []byte {
	capacityBits := dataCodewords(version, level) * 8
	buf := &bitBuffer{}
	encodeSegment(buf, content, m, version)

	// Terminator: up to four 0 bits, not exceeding capacity.
	term := 4
	if rem := capacityBits - buf.len(); rem < term {
		term = rem
	}
	for i := 0; i < term; i++ {
		buf.appendBit(0)
	}
	// Pad to a byte boundary.
	for buf.len()%8 != 0 {
		buf.appendBit(0)
	}
	// Alternating pad bytes 0xEC, 0x11.
	pad := []byte{0xEC, 0x11}
	for i := 0; buf.len() < capacityBits; i++ {
		buf.appendBits(uint32(pad[i%2]), 8)
	}
	return buf.bytes
}
