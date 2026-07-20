package qr

import (
	"bytes"
	"strings"
	"testing"
)

// FuzzEncodeDecode round-trips arbitrary content: whatever encodes must decode
// back to exactly the same bytes, at the version and level chosen. This is the
// core correctness invariant across every mode and size.
func FuzzEncodeDecode(f *testing.F) {
	seeds := []string{
		"", "0", "42", "HELLO WORLD", "netstar-labs/qr",
		"Hello, world!", strings.Repeat("A", 300), strings.Repeat("9", 120),
		"https://example/x", " $%*+-./:", "\x00\x01\xff",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, content string) {
		q, err := Encode(content, Medium)
		if err != nil {
			return // too long for Medium — a valid outcome, not a bug
		}
		dec, err := q.Decode()
		if err != nil {
			t.Fatalf("encode ok but decode failed: %v (content %q)", err, content)
		}
		if dec.Text != content {
			t.Fatalf("round-trip mismatch: got %q want %q", dec.Text, content)
		}
		if dec.Version != q.Version || dec.Level != q.Level {
			t.Fatalf("metadata drift: v%d/%v vs v%d/%v", dec.Version, dec.Level, q.Version, q.Level)
		}
	})
}

// FuzzDecodeMatrix feeds arbitrary bit-packed matrices to the decoder. A hostile
// matrix (any consistent Reed-Solomon codeword passes correction) must produce a
// value or an error — never a panic. This is the guard the H1 range checks and
// the geometry guards exist to prove.
func FuzzDecodeMatrix(f *testing.F) {
	// Seed with a couple of real symbols flattened to (size, bytes).
	for _, s := range []string{"HELLO WORLD", "12345", strings.Repeat("Z", 60)} {
		if q, err := Encode(s, Low); err == nil {
			f.Add(q.Size, flattenModules(q.modules))
		}
	}
	f.Fuzz(func(t *testing.T, size int, packed []byte) {
		if size < 0 || size > 200 {
			return // keep matrices small; DecodeMatrix range-checks size anyway
		}
		modules := unflattenModules(size, packed)
		// Must not panic; result is ignored.
		_, _ = DecodeMatrix(modules)
	})
}

// FuzzParseMessage targets the bit-stream parser directly with arbitrary
// "corrected" payloads — the exact surface where a hostile alphanumeric or
// numeric value used to index out of range. Must return cleanly, never panic.
func FuzzParseMessage(f *testing.F) {
	f.Add([]byte{0x20, 0x5b, 0x0b, 0x78, 0xd1, 0x72}, 1)
	f.Add([]byte{0xff, 0xff, 0xff, 0xff}, 1)  // mode 0b1111 → unsupported, clean error
	f.Add([]byte{0x40, 0x1f, 0x00}, 5)        // byte mode, count 1
	f.Add([]byte{0x20, 0xff, 0xff, 0xff}, 40) // alpha mode, hostile 11-bit values
	f.Fuzz(func(t *testing.T, message []byte, version int) {
		if version < 1 || version > 40 {
			return
		}
		// Must not panic regardless of content.
		_, _, _ = parseMessage(message, version)
	})
}

// FuzzDecodePNG feeds arbitrary bytes to the image entry point. Non-images
// error, the size/dimension caps hold, and nothing panics or allocates without
// bound.
func FuzzDecodePNG(f *testing.F) {
	if q, err := Encode("netstar-labs/qr", Medium); err == nil {
		var buf bytes.Buffer
		if q.PNG(&buf, 4, 4) == nil {
			f.Add(buf.Bytes())
		}
	}
	f.Add([]byte("not an image"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodePNG(bytes.NewReader(data))
	})
}

func flattenModules(m [][]bool) []byte {
	size := len(m)
	out := make([]byte, (size*size+7)/8)
	i := 0
	for _, row := range m {
		for _, dark := range row {
			if dark {
				out[i/8] |= 1 << uint(7-i%8)
			}
			i++
		}
	}
	return out
}

func unflattenModules(size int, packed []byte) [][]bool {
	m := make([][]bool, size)
	i := 0
	for r := 0; r < size; r++ {
		m[r] = make([]bool, size)
		for c := 0; c < size; c++ {
			if byteIdx := i / 8; byteIdx < len(packed) {
				m[r][c] = packed[byteIdx]&(1<<uint(7-i%8)) != 0
			}
			i++
		}
	}
	return m
}
