package qr

// formatEcBits maps a Level to the 2-bit error-correction indicator used in
// the format information (note this is NOT the L<M<Q<H ordering).
var formatEcBits = [4]int{Low: 0b01, Medium: 0b00, Quartile: 0b11, High: 0b10}

// bch computes the (dataBits+ecBits)-bit BCH codeword for data using the given
// generator polynomial: data placed in the high bits, followed by the
// remainder of data*x^ecBits mod gen.
func bch(data, gen, dataBits, ecBits int) int {
	v := data << ecBits
	for i := dataBits + ecBits - 1; i >= ecBits; i-- {
		if v&(1<<uint(i)) != 0 {
			v ^= gen << uint(i-ecBits)
		}
	}
	return (data << ecBits) | (v & ((1 << ecBits) - 1))
}

// formatInfo returns the 15-bit format information for a level and mask,
// BCH(15,5)-encoded with generator 0x537 and XOR-masked with 0x5412.
func formatInfo(level Level, mask int) int {
	data := (formatEcBits[level] << 3) | mask
	return bch(data, 0x537, 5, 10) ^ 0x5412
}

// versionInfo returns the 18-bit version information for versions >= 7,
// BCH(18,6)-encoded with generator 0x1F25 (no XOR mask).
func versionInfo(version int) int {
	return bch(version, 0x1f25, 6, 12)
}
