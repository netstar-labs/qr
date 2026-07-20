package qr

// rsGenerator returns the Reed-Solomon generator polynomial of the given
// degree, i.e. the product (x - alpha^0)(x - alpha^1)...(x - alpha^(deg-1))
// over GF(256). Coefficients are returned highest-degree first. Every
// coefficient of a generator polynomial is non-zero.
func rsGenerator(degree int) []byte {
	g := []byte{1}
	for i := 0; i < degree; i++ {
		next := make([]byte, len(g)+1)
		factor := gfExp[i]
		for j := 0; j < len(g); j++ {
			next[j] ^= g[j]                  // coefficient of x
			next[j+1] ^= gfMul(g[j], factor) // constant term
		}
		g = next
	}
	return g
}

// rsEncode returns ecLen error-correction codewords for data, computed as the
// remainder of data*x^ecLen divided by the degree-ecLen generator polynomial.
func rsEncode(data []byte, ecLen int) []byte {
	gen := rsGenerator(ecLen)
	res := make([]byte, len(data)+ecLen)
	copy(res, data)
	for i := 0; i < len(data); i++ {
		coef := res[i]
		if coef == 0 {
			continue
		}
		lead := int(gfLog[coef])
		for j := 0; j < len(gen); j++ {
			res[i+j] ^= gfExp[int(gfLog[gen[j]])+lead]
		}
	}
	return res[len(data):]
}
