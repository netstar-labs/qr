// Package qr generates QR Code symbols per ISO/IEC 18004 using only the Go
// standard library. It supports numeric, alphanumeric and byte modes, all 40
// versions and all four error-correction levels, with automatic mode and
// version selection and automatic mask selection.
//
// Kanji mode and ECI are intentionally not implemented; byte-mode content is
// emitted as its raw bytes (UTF-8 for Go strings), which every modern decoder
// interprets correctly.
package qr

import (
	"bufio"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"strings"
)

// Level is the error-correction level. The values are ordered by recovery
// capacity and are used directly as indices into the parameter tables.
type Level int

const (
	Low      Level = iota // ~7% recovery
	Medium                // ~15% recovery
	Quartile              // ~25% recovery
	High                  // ~30% recovery
)

// QRCode is a finished symbol.
type QRCode struct {
	Version int
	Level   Level
	Size    int // modules per side
	modules [][]bool
}

// Module reports whether the module at (x, y) is dark. x is the column and y
// the row, both zero-based from the top-left. Out-of-range coordinates return
// false.
func (q *QRCode) Module(x, y int) bool {
	if x < 0 || y < 0 || x >= q.Size || y >= q.Size {
		return false
	}
	return q.modules[y][x]
}

// Matrix returns the module grid as rows of booleans (true = dark). The result
// is a fresh copy the caller may modify.
func (q *QRCode) Matrix() [][]bool {
	out := make([][]bool, q.Size)
	for i := range q.modules {
		out[i] = append([]bool(nil), q.modules[i]...)
	}
	return out
}

// Decode decodes this symbol back to its content, applying Reed-Solomon error
// correction. It is primarily a self-check: Encode followed by Decode must
// round-trip.
func (q *QRCode) Decode() (*Decoded, error) {
	return DecodeMatrix(q.modules)
}

// Encode builds the smallest symbol that holds content at the given level,
// selecting mode, version and mask automatically.
func Encode(content string, level Level) (*QRCode, error) {
	if level < Low || level > High {
		return nil, errors.New("qr: invalid error-correction level")
	}
	m := detectMode(content)
	version, err := chooseVersion(content, m, level)
	if err != nil {
		return nil, err
	}
	return encodeAt(content, m, version, level), nil
}

// EncodeVersion builds a symbol at a fixed version (1..40), returning an error
// if content does not fit.
func EncodeVersion(content string, level Level, version int) (*QRCode, error) {
	if level < Low || level > High {
		return nil, errors.New("qr: invalid error-correction level")
	}
	if version < 1 || version > 40 {
		return nil, errors.New("qr: version out of range")
	}
	m := detectMode(content)
	if 4+charCountBits(version, m)+dataBitCount(content, m) > dataCodewords(version, level)*8 {
		return nil, errTooLong
	}
	return encodeAt(content, m, version, level), nil
}

func encodeAt(content string, m mode, version int, level Level) *QRCode {
	dataCW := buildCodewords(content, m, version, level)
	final := interleave(dataCW, version, level)
	g := build(version, level, final)
	return &QRCode{Version: version, Level: level, Size: g.size, modules: g.modules}
}

// interleave splits the data codewords into error-correction blocks, computes
// each block's EC codewords, and interleaves data then EC codewords in the
// order required for placement.
func interleave(data []byte, version int, level Level) []byte {
	spec := ecTable[version][level]
	words, maxData := spec.blockWords()

	dataBlocks := make([][]byte, len(words))
	ecBlocks := make([][]byte, len(words))
	pos := 0
	for b, w := range words {
		dataBlocks[b] = data[pos : pos+w]
		pos += w
		ecBlocks[b] = rsEncode(dataBlocks[b], spec.ecPerBlock)
	}

	out := make([]byte, 0, spec.totalCodewords())
	for i := 0; i < maxData; i++ {
		for b := range dataBlocks {
			if i < len(dataBlocks[b]) {
				out = append(out, dataBlocks[b][i])
			}
		}
	}
	for i := 0; i < spec.ecPerBlock; i++ {
		for b := range ecBlocks {
			out = append(out, ecBlocks[b][i])
		}
	}
	return out
}

// String renders the symbol as text using two block characters per module,
// with a one-module light quiet zone.
func (q *QRCode) String() string {
	const border = 1
	var sb strings.Builder
	for y := -border; y < q.Size+border; y++ {
		for x := -border; x < q.Size+border; x++ {
			if q.Module(x, y) {
				sb.WriteString("██")
			} else {
				sb.WriteString("  ")
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// maxPNGDim caps the rendered image side length. A v40 symbol at a generous
// 64 px/module with a wide border stays under it; anything larger is an
// allocation mistake, not a QR code.
const maxPNGDim = 1 << 15 // 32768 px

// PNG writes the symbol as a PNG. moduleSize is the pixel width of one module
// (>=1) and border is the quiet-zone width in modules (the standard minimum is
// 4). The rendered side length is capped at 32768 pixels.
func (q *QRCode) PNG(w io.Writer, moduleSize, border int) error {
	if moduleSize < 1 {
		moduleSize = 1
	}
	if border < 0 {
		border = 0
	}
	dim := (q.Size + 2*border) * moduleSize
	if dim <= 0 || dim > maxPNGDim {
		return errors.New("qr: rendered image dimensions out of range")
	}
	img := image.NewGray(image.Rect(0, 0, dim, dim))
	// Light background.
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	for y := 0; y < q.Size; y++ {
		for x := 0; x < q.Size; x++ {
			if !q.modules[y][x] {
				continue
			}
			px0 := (x + border) * moduleSize
			py0 := (y + border) * moduleSize
			for dy := 0; dy < moduleSize; dy++ {
				for dx := 0; dx < moduleSize; dx++ {
					img.SetGray(px0+dx, py0+dy, color.Gray{Y: 0x00})
				}
			}
		}
	}
	return png.Encode(w, img)
}

// WritePNGFile writes the symbol to a PNG file at path. On error the partial
// file is removed.
func (q *QRCode) WritePNGFile(path string, moduleSize, border int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(f)
	err = q.PNG(bw, moduleSize, border)
	if err == nil {
		err = bw.Flush()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}
