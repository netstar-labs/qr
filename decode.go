package qr

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	stdbits "math/bits"

	// Image format decoders registered for DecodePNG's image.Decode.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// Decoded is the result of decoding a symbol.
type Decoded struct {
	Version int
	Level   Level
	Mask    int
	Mode    string // "numeric", "alphanumeric" or "byte"
	Text    string
	Errors  int // number of codeword errors corrected
}

// validModuleCount reports whether n is a valid QR symbol size on the
// version 1..40 grid: 21, 25, ..., 177.
func validModuleCount(n int) bool {
	return n >= 21 && (n-17)%4 == 0
}

// DecodeMatrix decodes a module matrix (true = dark), performing format
// recovery, unmasking, de-interleaving and Reed-Solomon error correction.
//
// It complements the encoder and is used for in-repo round-trip verification;
// it also decodes matrices produced by other conforming encoders.
func DecodeMatrix(modules [][]bool) (*Decoded, error) {
	size := len(modules)
	if !validModuleCount(size) {
		return nil, fmt.Errorf("qr: invalid matrix size %d", size)
	}
	for _, row := range modules {
		if len(row) != size {
			return nil, errors.New("qr: matrix is not square")
		}
	}
	version := (size - 17) / 4
	if version < 1 || version > 40 {
		return nil, fmt.Errorf("qr: matrix implies out-of-range version %d", version)
	}

	level, mask, err := readFormat(modules, size)
	if err != nil {
		return nil, err
	}

	// Reconstruct which cells are function/format/version reserved.
	ref := newGrid(version)
	ref.placeFunctionPatterns(version)

	// Read the codeword bit stream back with the SAME traversal the encoder
	// used (visitData), unmasking data cells on the fly.
	totalCW := totalCodewordsPerVersion[version]
	bitsNeeded := totalCW * 8
	stream := make([]byte, 0, bitsNeeded)
	visitData(size, ref.reserved, func(row, col int) {
		if len(stream) >= bitsNeeded {
			return // trailing remainder bits
		}
		dark := modules[row][col]
		if maskCondition(mask, row, col) {
			dark = !dark
		}
		var bit byte
		if dark {
			bit = 1
		}
		stream = append(stream, bit)
	})
	if len(stream) < bitsNeeded {
		// Cannot happen for a valid version geometry; guard so a future
		// geometry bug surfaces as an error, never an index panic below.
		return nil, errors.New("qr: data region shorter than codeword capacity")
	}

	// Pack bits into codewords.
	cw := make([]byte, totalCW)
	for i := 0; i < totalCW; i++ {
		var v byte
		for b := 0; b < 8; b++ {
			v = v<<1 | stream[i*8+b]
		}
		cw[i] = v
	}

	message, nerr, err := deinterleaveAndCorrect(cw, version, level)
	if err != nil {
		return nil, err
	}

	mode, text, err := parseMessage(message, version)
	if err != nil {
		return nil, err
	}
	return &Decoded{Version: version, Level: level, Mask: mask, Mode: mode, Text: text, Errors: nerr}, nil
}

// readFormat recovers the error-correction level and mask from the format
// information, choosing the (level, mask) whose 15-bit code is closest (by
// Hamming distance) to either stored copy.
func readFormat(modules [][]bool, size int) (Level, int, error) {
	bit := func(r, c int) int {
		if modules[r][c] {
			return 1
		}
		return 0
	}
	copy1, copy2 := formatBitCells(size)
	var c1, c2 int
	for i := 0; i <= 14; i++ {
		c1 = c1<<1 | bit(copy1[i][0], copy1[i][1])
		c2 = c2<<1 | bit(copy2[i][0], copy2[i][1])
	}

	bestLevel, bestMask, bestDist := Level(0), 0, 99
	for l := Low; l <= High; l++ {
		for m := 0; m < 8; m++ {
			code := formatInfo(l, m)
			if d := hamming15(code, c1); d < bestDist {
				bestDist, bestLevel, bestMask = d, l, m
			}
			if d := hamming15(code, c2); d < bestDist {
				bestDist, bestLevel, bestMask = d, l, m
			}
		}
	}
	if bestDist > 3 {
		return 0, 0, errors.New("qr: unrecoverable format information")
	}
	return bestLevel, bestMask, nil
}

func hamming15(a, b int) int {
	return stdbits.OnesCount(uint((a ^ b) & 0x7fff))
}

// deinterleaveAndCorrect reverses interleave, then Reed-Solomon corrects each
// block, returning the concatenated data codewords in block order.
func deinterleaveAndCorrect(cw []byte, version int, level Level) ([]byte, int, error) {
	spec := ecTable[version][level]
	words, maxData := spec.blockWords()
	blocks := len(words)
	totalData := spec.dataCodewords()

	dataFlat := cw[:totalData]
	ecFlat := cw[totalData:]

	dataBlocks := make([][]byte, blocks)
	for b, w := range words {
		dataBlocks[b] = make([]byte, w)
	}
	idx := 0
	for i := 0; i < maxData; i++ {
		for b := 0; b < blocks; b++ {
			if i < len(dataBlocks[b]) {
				dataBlocks[b][i] = dataFlat[idx]
				idx++
			}
		}
	}
	ecBlocks := make([][]byte, blocks)
	for b := 0; b < blocks; b++ {
		ecBlocks[b] = make([]byte, spec.ecPerBlock)
	}
	idx = 0
	for i := 0; i < spec.ecPerBlock; i++ {
		for b := 0; b < blocks; b++ {
			ecBlocks[b][i] = ecFlat[idx]
			idx++
		}
	}

	var message []byte
	totalErr := 0
	for b := 0; b < blocks; b++ {
		recv := append(append([]byte(nil), dataBlocks[b]...), ecBlocks[b]...)
		fixed, ne, ok := rsDecode(recv, spec.ecPerBlock)
		if !ok {
			return nil, 0, fmt.Errorf("qr: block %d uncorrectable", b)
		}
		totalErr += ne
		message = append(message, fixed...)
	}
	return message, totalErr, nil
}

// bitReader reads big-endian bits from a codeword slice.
type bitReader struct {
	data []byte
	pos  int
}

func (r *bitReader) read(n int) int {
	v := 0
	for i := 0; i < n; i++ {
		var bit int
		if r.pos < len(r.data)*8 {
			bit = int(r.data[r.pos/8]>>uint(7-r.pos%8)) & 1
		}
		v = v<<1 | bit
		r.pos++
	}
	return v
}

func (r *bitReader) remaining() int { return len(r.data)*8 - r.pos }

// errCorrupt is returned when a payload passes error correction yet violates
// the mode's value ranges — a symbol from a broken (or hostile) encoder, never
// from this package's own output.
var errCorrupt = errors.New("qr: corrupt payload — value outside its mode's range")

// parseMessage reads the (single-segment) message stream. This encoder emits
// one segment; the decoder stops at a terminator or when the stream is spent.
//
// Every value read from the stream is range-checked before use: numeric groups
// must be valid decimal, alphanumeric values must index the 45-symbol set, and
// no mode may claim more characters than the remaining bits can carry. The
// stream is attacker-controlled (any consistent Reed-Solomon codeword passes
// correction), so out-of-range values are errors, never index arithmetic.
func parseMessage(message []byte, version int) (string, string, error) {
	r := &bitReader{data: message}
	var out []byte
	modeName := ""
	for r.remaining() >= 4 {
		m := mode(r.read(4))
		if m == 0 { // terminator
			break
		}
		count := r.read(charCountBits(version, m))
		if dataBitCountN(count, m) > r.remaining() {
			return "", "", errCorrupt // claims more characters than bits remain
		}
		switch m {
		case modeNumeric:
			modeName = "numeric"
			for count >= 3 {
				v := r.read(10)
				if v > 999 {
					return "", "", errCorrupt
				}
				out = append(out, byte('0'+v/100), byte('0'+(v/10)%10), byte('0'+v%10))
				count -= 3
			}
			if count == 2 {
				v := r.read(7)
				if v > 99 {
					return "", "", errCorrupt
				}
				out = append(out, byte('0'+v/10), byte('0'+v%10))
			} else if count == 1 {
				v := r.read(4)
				if v > 9 {
					return "", "", errCorrupt
				}
				out = append(out, byte('0'+v))
			}
		case modeAlpha:
			modeName = "alphanumeric"
			for count >= 2 {
				v := r.read(11)
				if v >= 45*45 {
					return "", "", errCorrupt
				}
				out = append(out, alphaSet[v/45], alphaSet[v%45])
				count -= 2
			}
			if count == 1 {
				v := r.read(6)
				if v >= 45 {
					return "", "", errCorrupt
				}
				out = append(out, alphaSet[v])
			}
		case modeByte:
			modeName = "byte"
			for i := 0; i < count; i++ {
				out = append(out, byte(r.read(8)))
			}
		default:
			return "", "", fmt.Errorf("qr: unsupported mode %04b", int(m))
		}
	}
	return modeName, string(out), nil
}

// dataBitCountN returns the payload bit length of n characters in mode m —
// the counting half of dataBitCount, shared with the encoder's math.
func dataBitCountN(n int, m mode) int {
	switch m {
	case modeNumeric:
		bits := (n / 3) * 10
		switch n % 3 {
		case 1:
			bits += 4
		case 2:
			bits += 7
		}
		return bits
	case modeAlpha:
		bits := (n / 2) * 11
		if n%2 == 1 {
			bits += 6
		}
		return bits
	default:
		return n * 8
	}
}

// Input limits for DecodePNG. The reader is untrusted: a tiny compressed
// image can declare enormous pixel dimensions (a decompression bomb), so the
// encoded size is capped and the declared dimensions are checked BEFORE the
// pixel decode allocates anything.
const (
	maxImageBytes = 32 << 20 // 32 MiB of encoded input
	maxImageDim   = 1 << 15  // 32768 px per side (matches the PNG render cap)
)

// DecodePNG decodes a QR symbol from a clean, high-contrast, axis-aligned
// image (PNG, JPEG or GIF — a render, screenshot, or flat scan). It is not a
// perspective-correcting photo decoder: it assumes the symbol is upright,
// unrotated and rendered with square modules on a light quiet zone.
//
// The input is treated as untrusted: encoded size is capped at 32 MiB and
// declared dimensions at 32768x32768 before any pixel buffer is allocated.
func DecodePNG(r io.Reader) (*Decoded, error) {
	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(r, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if n > maxImageBytes {
		return nil, errors.New("qr: image input exceeds 32 MiB limit")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImageDim || cfg.Height > maxImageDim {
		return nil, fmt.Errorf("qr: image dimensions %dx%d out of range", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(&buf)
	if err != nil {
		return nil, err
	}
	modules, err := sampleModules(img)
	if err != nil {
		return nil, err
	}
	return DecodeMatrix(modules)
}

// dark reports whether a pixel is dark (luminance below mid-grey).
func darkAt(img image.Image, x, y int) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	// Rec. 601 luma on 16-bit channels.
	lum := (299*r + 587*g + 114*b) / 1000
	return lum < 0x8000
}

// sampleModules locates an upright QR symbol in a clean image and samples it to
// a module matrix. Module pitch is derived from the top-left finder pattern.
func sampleModules(img image.Image) ([][]bool, error) {
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if darkAt(img, x, y) {
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < minX || maxY < minY {
		return nil, errors.New("qr: no dark modules found")
	}
	symW := maxX - minX + 1
	symH := maxY - minY + 1

	// The top-left finder's top border is 7 modules of solid dark, capped on
	// the right by the light separator. Its leading dark run from (minX, *) is
	// the longest such run among the top rows; deeper rows cut through the
	// finder and yield short runs. Take the max over the top band to be robust
	// to a one-pixel antialiasing fringe on the very top row.
	band := symH / 8
	if band < 1 {
		band = 1
	}
	run := 0
	for y := minY; y <= minY+band && y <= maxY; y++ {
		r := 0
		for x := minX; x <= maxX && darkAt(img, x, y); x++ {
			r++
		}
		if r > run {
			run = r
		}
	}
	var pitch float64
	if run >= 7 {
		pitch = float64(run) / 7.0
	}
	// Determine module count: prefer the finder-derived pitch, else pick the
	// valid count (21..177) whose implied pitch best divides the symbol size.
	best := 0
	if pitch > 0 {
		best = int(float64(symW)/pitch + 0.5)
	}
	if !validModuleCount(best) || best > 177 {
		best = 0
		bestErr := 1e18
		for n := 21; n <= 177; n += 4 {
			p := float64(symW) / float64(n)
			e := math.Abs(p*float64(n) - float64(symW))
			// Prefer counts where both dimensions divide cleanly.
			e += math.Abs(float64(symH)/p - float64(n))
			if e < bestErr {
				bestErr, best = e, n
			}
		}
	}
	n := best
	pw := float64(symW) / float64(n)
	ph := float64(symH) / float64(n)

	modules := make([][]bool, n)
	for gy := 0; gy < n; gy++ {
		modules[gy] = make([]bool, n)
		for gx := 0; gx < n; gx++ {
			px := minX + int((float64(gx)+0.5)*pw)
			py := minY + int((float64(gy)+0.5)*ph)
			modules[gy][gx] = darkAt(img, px, py)
		}
	}
	return modules, nil
}
