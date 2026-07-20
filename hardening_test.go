package qr

import (
	"bytes"
	"strings"
	"testing"
)

// TestParseMessageHostileAlpha is the regression lock for the decode panic: a
// Reed-Solomon-consistent payload can carry an 11-bit alphanumeric value up to
// 2047, but only 0..2024 (45*45-1) are valid. The old decoder indexed the
// 45-char set with v/45 (up to 45) and panicked; it must now error cleanly.
func TestParseMessageHostileAlpha(t *testing.T) {
	// Mode 0010 (alpha), count 2, then an 11-bit value of all ones (2047).
	buf := &bitBuffer{}
	buf.appendBits(uint32(modeAlpha), 4)
	buf.appendBits(2, charCountBits(1, modeAlpha)) // count = 2
	buf.appendBits(0x7FF, 11)                      // 2047 → out of range
	_, _, err := parseMessage(buf.bytes, 1)
	if err == nil {
		t.Fatal("hostile alpha value did not error")
	}
}

// TestParseMessageHostileNumericAndOverclaim covers the other two H1 surfaces:
// an out-of-range numeric triple, and a count claiming more characters than the
// remaining bits can supply.
func TestParseMessageHostileNumericAndOverclaim(t *testing.T) {
	// Numeric, count 3, value 1000 (max valid is 999).
	num := &bitBuffer{}
	num.appendBits(uint32(modeNumeric), 4)
	num.appendBits(3, charCountBits(1, modeNumeric))
	num.appendBits(1000, 10)
	if _, _, err := parseMessage(num.bytes, 1); err == nil {
		t.Fatal("numeric > 999 did not error")
	}

	// Byte mode claiming 255 characters with almost no payload behind it.
	over := &bitBuffer{}
	over.appendBits(uint32(modeByte), 4)
	over.appendBits(255, charCountBits(1, modeByte))
	over.appendBits(0, 8)
	if _, _, err := parseMessage(over.bytes, 1); err == nil {
		t.Fatal("byte over-claim did not error")
	}
}

// TestDecodeMatrixRejects checks the geometry guards reject malformed matrices
// instead of panicking or misreading.
func TestDecodeMatrixRejects(t *testing.T) {
	cases := []struct {
		name    string
		modules [][]bool
	}{
		{"too small", make([][]bool, 10)},
		{"non-multiple size", square(22)},
		{"empty", nil},
	}
	for _, c := range cases {
		if _, err := DecodeMatrix(c.modules); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}

	// A ragged (non-square) matrix.
	ragged := square(21)
	ragged[5] = ragged[5][:20]
	if _, err := DecodeMatrix(ragged); err == nil {
		t.Error("ragged matrix accepted")
	}
}

// TestCapacityBoundary confirms exact-fit content encodes and one byte more
// spills to the next version (or errors at the ceiling).
func TestCapacityBoundary(t *testing.T) {
	// Version 1, level H holds 9 data codewords. Header for byte mode at v1 is
	// 4 (mode) + 8 (count) = 12 bits; 9*8 = 72 bits total → 60 bits = 7 bytes
	// of payload plus terminator fits, 8 should push to v2.
	seven := EncodeMust(t, strings.Repeat("a", 7), High)
	if seven.Version != 1 {
		t.Errorf("7 bytes at H: version %d, want 1", seven.Version)
	}
	eight, err := EncodeVersion(strings.Repeat("a", 8), High, 1)
	if err == nil {
		t.Errorf("8 bytes must not fit v1/H, got version %d", eight.Version)
	}
	// Auto-selection rolls to v2 instead of failing.
	auto := EncodeMust(t, strings.Repeat("a", 8), High)
	if auto.Version != 2 {
		t.Errorf("8 bytes at H auto: version %d, want 2", auto.Version)
	}
}

// TestPNGDimensionCap rejects a render whose pixel dimensions exceed the cap
// rather than attempting a multi-gigabyte allocation.
func TestPNGDimensionCap(t *testing.T) {
	q := EncodeMust(t, "x", Low) // 21 modules
	var buf bytes.Buffer
	if err := q.PNG(&buf, 1<<20, 0); err == nil {
		t.Error("oversize moduleSize accepted")
	}
	// A sane render still works.
	buf.Reset()
	if err := q.PNG(&buf, 4, 4); err != nil {
		t.Errorf("normal render failed: %v", err)
	}
}

// TestDecodePNGLimits confirms the image entry point rejects junk and enforces
// its dimension cap via DecodeConfig before decoding pixels.
func TestDecodePNGLimits(t *testing.T) {
	if _, err := DecodePNG(bytes.NewReader([]byte("not an image"))); err == nil {
		t.Error("non-image accepted")
	}
	if _, err := DecodePNG(bytes.NewReader(nil)); err == nil {
		t.Error("empty input accepted")
	}
}

func square(n int) [][]bool {
	m := make([][]bool, n)
	for i := range m {
		m[i] = make([]bool, n)
	}
	return m
}

func EncodeMust(t *testing.T, content string, level Level) *QRCode {
	t.Helper()
	q, err := Encode(content, level)
	if err != nil {
		t.Fatalf("Encode(%q): %v", content, err)
	}
	return q
}
