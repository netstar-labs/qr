package qr

// grid is a mutable symbol under construction. modules holds module colours
// (true = dark) and reserved marks function-pattern and information cells that
// data must not overwrite and that masking must not alter.
type grid struct {
	size     int
	modules  [][]bool
	reserved [][]bool
}

func newGrid(version int) *grid {
	size := version*4 + 17
	g := &grid{size: size}
	g.modules = make([][]bool, size)
	g.reserved = make([][]bool, size)
	for i := range g.modules {
		g.modules[i] = make([]bool, size)
		g.reserved[i] = make([]bool, size)
	}
	return g
}

func (g *grid) set(r, c int, dark bool) {
	g.modules[r][c] = dark
	g.reserved[r][c] = true
}

// placeFunctionPatterns draws finders (whose -1/7 border cells double as the
// light separators), timing patterns, alignment patterns and the fixed dark
// module, and reserves the format/version areas.
func (g *grid) placeFunctionPatterns(version int) {
	g.placeFinder(0, 0)
	g.placeFinder(0, g.size-7)
	g.placeFinder(g.size-7, 0)
	g.placeTiming()
	g.placeAlignment(version)
	// Fixed dark module.
	g.set(g.size-8, 8, true)
	g.reserveFormatAndVersion(version)
}

func (g *grid) placeFinder(row, col int) {
	for r := -1; r <= 7; r++ {
		for c := -1; c <= 7; c++ {
			rr, cc := row+r, col+c
			if rr < 0 || rr >= g.size || cc < 0 || cc >= g.size {
				continue
			}
			// Ring at distance boundaries: outer 5x5 ring dark, inner 3x3 dark.
			dark := (r >= 0 && r <= 6 && (c == 0 || c == 6)) ||
				(c >= 0 && c <= 6 && (r == 0 || r == 6)) ||
				(r >= 2 && r <= 4 && c >= 2 && c <= 4)
			g.set(rr, cc, dark)
		}
	}
}

func (g *grid) placeTiming() {
	for i := 8; i < g.size-8; i++ {
		dark := i%2 == 0
		if !g.reserved[6][i] {
			g.set(6, i, dark)
		}
		if !g.reserved[i][6] {
			g.set(i, 6, dark)
		}
	}
}

func (g *grid) placeAlignment(version int) {
	pos := alignmentPositions[version]
	last := g.size - 7
	for _, r := range pos {
		for _, c := range pos {
			// Skip the three centers overlapping finder patterns.
			if (r == 6 && c == 6) || (r == 6 && c == last) || (r == last && c == 6) {
				continue
			}
			g.placeAlignmentPattern(r, c)
		}
	}
}

func (g *grid) placeAlignmentPattern(row, col int) {
	for r := -2; r <= 2; r++ {
		for c := -2; c <= 2; c++ {
			dark := r == -2 || r == 2 || c == -2 || c == 2 || (r == 0 && c == 0)
			g.set(row+r, col+c, dark)
		}
	}
}

// reserveFormatAndVersion marks the format-information cells (and, for
// versions >= 7, version-information cells) as reserved without setting their
// final values, which depend on the chosen mask.
func (g *grid) reserveFormatAndVersion(version int) {
	for i := 0; i <= 8; i++ {
		if i != 6 {
			g.reserved[8][i] = true // top-left horizontal
			g.reserved[i][8] = true // top-left vertical
		}
	}
	for i := 0; i < 8; i++ {
		g.reserved[g.size-1-i][8] = true // bottom-left vertical
		g.reserved[8][g.size-1-i] = true // top-right horizontal
	}
	if version >= 7 {
		for i := 0; i < 6; i++ {
			for j := 0; j < 3; j++ {
				g.reserved[i][g.size-11+j] = true
				g.reserved[g.size-11+j][i] = true
			}
		}
	}
}

// visitData walks every non-reserved cell in the standard upward/downward
// zigzag over two-module-wide columns (skipping the vertical timing column),
// calling fn in bit-stream order. It is the ONE definition of the traversal:
// the encoder places bits with it and the decoder reads them back with it, so
// the two orders can never drift apart.
func visitData(size int, reserved [][]bool, fn func(row, col int)) {
	up := true
	for col := size - 1; col > 0; col -= 2 {
		if col == 6 {
			col = 5 // skip vertical timing column
		}
		for i := 0; i < size; i++ {
			row := i
			if up {
				row = size - 1 - i
			}
			for j := 0; j < 2; j++ {
				if c := col - j; !reserved[row][c] {
					fn(row, c)
				}
			}
		}
		up = !up
	}
}

// placeData writes the codeword bit stream into the data region; bits past the
// stream (the version's remainder bits) are 0.
func (g *grid) placeData(data []byte) {
	idx := 0
	visitData(g.size, g.reserved, func(row, col int) {
		if idx < len(data)*8 {
			g.modules[row][col] = data[idx/8]&(1<<uint(7-idx%8)) != 0
		} else {
			g.modules[row][col] = false // remainder bits
		}
		idx++
	})
}

// applyMask flips data modules (never reserved cells) where the mask condition
// holds. It is its own inverse.
func (g *grid) applyMask(mask int) {
	for r := 0; r < g.size; r++ {
		for c := 0; c < g.size; c++ {
			if g.reserved[r][c] {
				continue
			}
			if maskCondition(mask, r, c) {
				g.modules[r][c] = !g.modules[r][c]
			}
		}
	}
}

// writeFormatInfo places the 15-bit format information for level+mask in both
// copies.
func (g *grid) writeFormatInfo(level Level, mask int) {
	f := formatInfo(level, mask)
	bit := func(i int) bool { return (f>>uint(14-i))&1 != 0 } // i=0 is MSB

	// Copy 1 around the top-left finder.
	for i := 0; i <= 5; i++ {
		g.modules[8][i] = bit(i)
	}
	g.modules[8][7] = bit(6)
	g.modules[8][8] = bit(7)
	g.modules[7][8] = bit(8)
	for i := 9; i <= 14; i++ {
		g.modules[14-i][8] = bit(i)
	}
	// Copy 2 split across the other two finders.
	for i := 0; i <= 7; i++ {
		g.modules[g.size-1-i][8] = bit(i)
	}
	for i := 8; i <= 14; i++ {
		g.modules[8][g.size-15+i] = bit(i)
	}
}

// writeVersionInfo places the 18-bit version information for versions >= 7.
func (g *grid) writeVersionInfo(version int) {
	if version < 7 {
		return
	}
	v := versionInfo(version)
	for i := 0; i < 18; i++ {
		bit := (v>>uint(i))&1 != 0
		a, b := i/3, i%3
		g.modules[a][g.size-11+b] = bit
		g.modules[g.size-11+b][a] = bit
	}
}

// build assembles the full symbol: function patterns, data, then the
// lowest-penalty mask with its format and version information applied.
func build(version int, level Level, data []byte) *grid {
	base := newGrid(version)
	base.placeFunctionPatterns(version)
	base.placeData(data)

	bestMask := 0
	bestPenalty := 1 << 62
	for mask := 0; mask < 8; mask++ {
		cand := base.clone()
		cand.applyMask(mask)
		cand.writeFormatInfo(level, mask)
		cand.writeVersionInfo(version)
		if p := cand.penalty(); p < bestPenalty {
			bestPenalty = p
			bestMask = mask
		}
	}
	base.applyMask(bestMask)
	base.writeFormatInfo(level, bestMask)
	base.writeVersionInfo(version)
	return base
}

func (g *grid) clone() *grid {
	c := &grid{size: g.size}
	c.modules = make([][]bool, g.size)
	c.reserved = make([][]bool, g.size)
	for i := range g.modules {
		c.modules[i] = append([]bool(nil), g.modules[i]...)
		c.reserved[i] = append([]bool(nil), g.reserved[i]...)
	}
	return c
}
