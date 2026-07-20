package qr

// This file adds the decoding half of the Reed-Solomon code: syndrome
// computation, the Berlekamp-Massey algorithm for the error locator, a Chien
// search for error positions and Forney's algorithm for error magnitudes.
// Generator roots are alpha^0 .. alpha^(ecLen-1), matching rsEncode.

func gfInv(a byte) byte {
	// a must be non-zero.
	return gfExp[255-int(gfLog[a])]
}

func gfDiv(a, b byte) byte {
	if a == 0 {
		return 0
	}
	return gfExp[(int(gfLog[a])-int(gfLog[b])+255)%255]
}

// alphaPow returns alpha^k for any integer k (negative allowed).
func alphaPow(k int) byte {
	return gfExp[((k%255)+255)%255]
}

// evalLow evaluates a low-degree-first polynomial (p[0] is the constant term).
func evalLow(p []byte, x byte) byte {
	var y byte
	// Horner from the highest degree down.
	for i := len(p) - 1; i >= 0; i-- {
		y = gfMul(y, x) ^ p[i]
	}
	return y
}

func polyScale(p []byte, s byte) []byte {
	out := make([]byte, len(p))
	for i, v := range p {
		out[i] = gfMul(v, s)
	}
	return out
}

// polyShift multiplies a low-first polynomial by x^n.
func polyShift(p []byte, n int) []byte {
	return append(make([]byte, n), p...)
}

func polyAdd(a, b []byte) []byte {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	out := make([]byte, n)
	for i := 0; i < len(a); i++ {
		out[i] ^= a[i]
	}
	for i := 0; i < len(b); i++ {
		out[i] ^= b[i]
	}
	return out
}

// polyMul multiplies two low-first polynomials.
func polyMul(a, b []byte) []byte {
	out := make([]byte, len(a)+len(b)-1)
	for i := range a {
		if a[i] == 0 {
			continue
		}
		for j := range b {
			out[i+j] ^= gfMul(a[i], b[j])
		}
	}
	return out
}

func polyDeg(p []byte) int {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] != 0 {
			return i
		}
	}
	return 0
}

// rsDecode corrects up to ecLen/2 errors in recv (data followed by EC,
// high-degree-first) in place, returning the corrected data portion and the
// number of errors corrected. ok is false if the received word is
// uncorrectable.
func rsDecode(recv []byte, ecLen int) (data []byte, nerr int, ok bool) {
	n := len(recv)
	// Syndromes S_i = R(alpha^i), i = 0..ecLen-1, low-first in `syn`.
	syn := make([]byte, ecLen)
	hasErr := false
	for i := 0; i < ecLen; i++ {
		syn[i] = gfPolyEvalHigh(recv, gfExp[i])
		if syn[i] != 0 {
			hasErr = true
		}
	}
	if !hasErr {
		return recv[:n-ecLen], 0, true
	}

	// Berlekamp-Massey for the error locator polynomial lambda (low-first).
	lambda := []byte{1}
	b := []byte{1}
	L := 0
	m := 1
	for i := 0; i < ecLen; i++ {
		delta := syn[i]
		for j := 1; j <= L && j < len(lambda); j++ {
			delta ^= gfMul(lambda[j], syn[i-j])
		}
		if delta == 0 {
			m++
			continue
		}
		term := polyShift(polyScale(b, delta), m)
		if 2*L <= i {
			t := append([]byte(nil), lambda...)
			lambda = polyAdd(lambda, term)
			L = i + 1 - L
			b = polyScale(t, gfInv(delta))
			m = 1
		} else {
			lambda = polyAdd(lambda, term)
			m++
		}
	}

	if polyDeg(lambda) != L {
		return nil, 0, false
	}

	// Chien search: error positions p where lambda(alpha^-p) == 0.
	positions := make([]int, 0, L)
	for p := 0; p < n; p++ {
		if evalLow(lambda, alphaPow(-p)) == 0 {
			positions = append(positions, p)
		}
	}
	if len(positions) != L {
		return nil, 0, false
	}

	// Error evaluator omega(x) = S(x)*lambda(x) mod x^ecLen (low-first).
	omega := polyMul(syn, lambda)
	if len(omega) > ecLen {
		omega = omega[:ecLen]
	}

	// Forney: e_p = X * omega(X^-1) / lambda'(X^-1), with X = alpha^p
	// (generator's first consecutive root is alpha^0).
	corrected := append([]byte(nil), recv...)
	for _, p := range positions {
		x := alphaPow(p)
		xinv := alphaPow(-p)
		num := gfMul(x, evalLow(omega, xinv))
		den := evalLow(formalDeriv(lambda), xinv)
		if den == 0 {
			return nil, 0, false
		}
		mag := gfDiv(num, den)
		corrected[n-1-p] ^= mag
	}

	// Verify all syndromes now vanish.
	for i := 0; i < ecLen; i++ {
		if gfPolyEvalHigh(corrected, gfExp[i]) != 0 {
			return nil, 0, false
		}
	}
	return corrected[:n-ecLen], len(positions), true
}

// gfPolyEvalHigh evaluates a high-degree-first polynomial (p[0] is the highest
// degree coefficient).
func gfPolyEvalHigh(p []byte, x byte) byte {
	y := p[0]
	for i := 1; i < len(p); i++ {
		y = gfMul(y, x) ^ p[i]
	}
	return y
}

// formalDeriv returns the formal derivative of a low-first polynomial over
// GF(2): only odd-degree terms survive, shifted down by one.
func formalDeriv(p []byte) []byte {
	if len(p) <= 1 {
		return []byte{0}
	}
	out := make([]byte, len(p)-1)
	for j := 1; j < len(p); j++ {
		if j%2 == 1 {
			out[j-1] = p[j]
		}
	}
	return out
}
