package qr

import (
	"bytes"
	"image/png"
	"testing"
)

// --- Parameter table consistency -------------------------------------------

func TestECTableConsistency(t *testing.T) {
	for v := 1; v <= 40; v++ {
		for l := Low; l <= High; l++ {
			s := ecTable[v][l]
			if got, want := s.totalCodewords(), totalCodewordsPerVersion[v]; got != want {
				t.Errorf("v%d %v: total codewords %d, want %d", v, l, got, want)
			}
			if s.numBlocks() == 0 {
				t.Errorf("v%d %v: zero blocks", v, l)
			}
			// Each block must be at least as large as its EC codewords for the
			// code to be well-formed.
			if s.group1Words > 0 && s.ecPerBlock > s.group1Words+s.ecPerBlock {
				t.Errorf("v%d %v: malformed block sizing", v, l)
			}
		}
	}
}

func TestAlignmentTable(t *testing.T) {
	if len(alignmentPositions[1]) != 0 {
		t.Errorf("version 1 must have no alignment patterns")
	}
	for v := 2; v <= 40; v++ {
		p := alignmentPositions[v]
		if len(p) < 2 {
			t.Errorf("v%d: too few alignment coordinates", v)
		}
		if p[0] != 6 || p[len(p)-1] != v*4+17-7 {
			t.Errorf("v%d: alignment endpoints %v unexpected", v, p)
		}
	}
}

// --- Galois field ----------------------------------------------------------

func TestGaloisRoundTrip(t *testing.T) {
	for i := 1; i < 256; i++ {
		e := gfExp[gfLog[byte(i)]]
		if e != byte(i) {
			t.Fatalf("exp(log(%d)) = %d", i, e)
		}
	}
	if gfMul(1, 200) != 200 {
		t.Errorf("multiplicative identity failed")
	}
	for a := 1; a < 256; a++ {
		for b := 1; b < 256; b++ {
			if gfMul(byte(a), byte(b)) != gfMul(byte(b), byte(a)) {
				t.Fatalf("gfMul not commutative at %d,%d", a, b)
			}
		}
	}
}

// --- Reed-Solomon: codeword must be divisible by the generator -------------

func TestReedSolomonSyndromes(t *testing.T) {
	cases := []struct {
		data []byte
		ec   int
	}{
		{[]byte{0x20, 0x5b, 0x0b, 0x78, 0xd1, 0x72, 0xdc, 0x4d, 0x43, 0x40, 0xec, 0x11, 0xec}, 13},
		{[]byte{0x10, 0x20, 0x0c, 0x56, 0x61, 0x80, 0xec, 0x11}, 10},
		{bytes.Repeat([]byte{0xa5}, 80), 30},
	}
	for ci, c := range cases {
		ec := rsEncode(c.data, c.ec)
		if len(ec) != c.ec {
			t.Fatalf("case %d: got %d ec codewords, want %d", ci, len(ec), c.ec)
		}
		codeword := append(append([]byte{}, c.data...), ec...)
		for i := 0; i < c.ec; i++ {
			if s := gfPolyEvalHigh(codeword, gfExp[i]); s != 0 {
				t.Errorf("case %d: syndrome at alpha^%d = %d, want 0", ci, i, s)
			}
		}
	}
}

// --- BCH format and version information ------------------------------------

func TestFormatInfo(t *testing.T) {
	// Well-known anchor: level M, mask 0.
	if got := formatInfo(Medium, 0); got != 0x5412 {
		t.Errorf("formatInfo(M,0) = %#x, want 0x5412", got)
	}
	// Every format word, unmasked, must be a valid BCH(15,5) codeword.
	for l := Low; l <= High; l++ {
		for mask := 0; mask < 8; mask++ {
			w := formatInfo(l, mask) ^ 0x5412
			if rem := bchRemainder(w, 0x537, 10); rem != 0 {
				t.Errorf("format (%v,mask%d) not a valid BCH codeword (rem %#x)", l, mask, rem)
			}
		}
	}
}

func TestVersionInfo(t *testing.T) {
	if got := versionInfo(7); got != 0x07C94 {
		t.Errorf("versionInfo(7) = %#x, want 0x07C94", got)
	}
	for v := 7; v <= 40; v++ {
		info := versionInfo(v)
		if info>>12 != v {
			t.Errorf("v%d: top 6 bits = %d", v, info>>12)
		}
		if rem := bchRemainder(info, 0x1f25, 12); rem != 0 {
			t.Errorf("v%d: version info not a valid BCH codeword (rem %#x)", v, rem)
		}
	}
}

// bchRemainder returns (word mod gen) treating the low ecBits as the remainder
// region, used only by tests.
func bchRemainder(word, gen, ecBits int) int {
	hi := 14
	if ecBits == 12 {
		hi = 17
	}
	v := word
	for i := hi; i >= ecBits; i-- {
		if v&(1<<uint(i)) != 0 {
			v ^= gen << uint(i-ecBits)
		}
	}
	return v & ((1 << ecBits) - 1)
}

// --- Segment encoding vectors ----------------------------------------------

func bits(b *bitBuffer) string {
	out := make([]byte, b.nbits)
	for i := 0; i < b.nbits; i++ {
		if b.bytes[i/8]&(1<<uint(7-i%8)) != 0 {
			out[i] = '1'
		} else {
			out[i] = '0'
		}
	}
	return string(out)
}

func TestNumericSegment(t *testing.T) {
	buf := &bitBuffer{}
	encodeSegment(buf, "01234567", modeNumeric, 1)
	want := "0001" + "0000001000" + "0000001100" + "0101011001" + "1000011"
	if got := bits(buf); got != want {
		t.Errorf("numeric segment\n got %s\nwant %s", got, want)
	}
}

func TestAlphanumericSegment(t *testing.T) {
	buf := &bitBuffer{}
	encodeSegment(buf, "HELLO WORLD", modeAlpha, 1)
	want := "0010" + "000001011" +
		"01100001011" + "01111000110" + "10001011100" +
		"10110111000" + "10011010100" + "001101"
	if got := bits(buf); got != want {
		t.Errorf("alphanumeric segment\n got %s\nwant %s", got, want)
	}
}

func TestModeDetection(t *testing.T) {
	cases := []struct {
		in   string
		want mode
	}{
		{"12345", modeNumeric},
		{"HELLO WORLD", modeAlpha},
		{"Hello, world!", modeByte},
		{"https://netstar-labs.example/x", modeByte},
	}
	for _, c := range cases {
		if got := detectMode(c.in); got != c.want {
			t.Errorf("detectMode(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// --- End-to-end ------------------------------------------------------------

func TestEncodeBasics(t *testing.T) {
	q, err := Encode("HELLO WORLD", Quartile)
	if err != nil {
		t.Fatal(err)
	}
	if q.Version != 1 {
		t.Errorf("version = %d, want 1", q.Version)
	}
	if q.Size != 21 {
		t.Errorf("size = %d, want 21", q.Size)
	}
	// Finder pattern centre (dark) and corner separator (light) sanity checks.
	if !q.Module(3, 3) {
		t.Errorf("finder centre should be dark")
	}
	if !q.Module(0, 0) || !q.Module(6, 6) {
		t.Errorf("finder outer ring should be dark")
	}
	// Fixed dark module.
	if !q.Module(8, q.Size-8) {
		t.Errorf("fixed dark module missing")
	}
}

func TestEncodeAllVersionsLevels(t *testing.T) {
	payload := bytes.Repeat([]byte("A"), 10) // fits everywhere at v1
	for l := Low; l <= High; l++ {
		for v := 1; v <= 40; v++ {
			q, err := EncodeVersion(string(payload), l, v)
			if err != nil {
				t.Fatalf("v%d %v: %v", v, l, err)
			}
			if q.Size != v*4+17 {
				t.Fatalf("v%d: size %d", v, q.Size)
			}
			// Timing pattern must alternate along row 6.
			for c := 8; c < q.Size-8; c++ {
				if q.Module(c, 6) != (c%2 == 0) {
					t.Fatalf("v%d: timing pattern broken at col %d", v, c)
				}
			}
		}
	}
}

func TestTooLong(t *testing.T) {
	huge := bytes.Repeat([]byte("A"), 5000)
	if _, err := Encode(string(huge), High); err == nil {
		t.Errorf("expected error for oversized content")
	}
}

func TestPNGOutput(t *testing.T) {
	q, err := Encode("netstar-labs/qr", Medium)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := q.PNG(&buf, 4, 4); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(&buf)
	if err != nil {
		t.Fatalf("PNG did not decode: %v", err)
	}
	want := (q.Size + 8) * 4
	if b := img.Bounds(); b.Dx() != want || b.Dy() != want {
		t.Errorf("PNG dims %dx%d, want %dx%d", b.Dx(), b.Dy(), want, want)
	}
}
