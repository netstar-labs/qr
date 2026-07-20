package qr

// maskCondition reports whether the data-mask pattern for the given mask
// number applies at (row, col), per ISO/IEC 18004 Table 10.
func maskCondition(mask, row, col int) bool {
	switch mask {
	case 0:
		return (row+col)%2 == 0
	case 1:
		return row%2 == 0
	case 2:
		return col%3 == 0
	case 3:
		return (row+col)%3 == 0
	case 4:
		return (row/2+col/3)%2 == 0
	case 5:
		return (row*col)%2+(row*col)%3 == 0
	case 6:
		return ((row*col)%2+(row*col)%3)%2 == 0
	case 7:
		return ((row+col)%2+(row*col)%3)%2 == 0
	}
	return false
}

// penalty scores the whole symbol with the four penalty rules; lower is
// better. Used to select the data mask.
func (g *grid) penalty() int {
	return g.penaltyRun() + g.penaltyBlocks() + g.penaltyFinderLike() + g.penaltyBalance()
}

// Rule 1: runs of five or more same-coloured modules in a row/column.
func (g *grid) penaltyRun() int {
	score := 0
	count := func(get func(int) bool) {
		run := 1
		prev := get(0)
		for i := 1; i < g.size; i++ {
			cur := get(i)
			if cur == prev {
				run++
			} else {
				if run >= 5 {
					score += 3 + (run - 5)
				}
				run = 1
				prev = cur
			}
		}
		if run >= 5 {
			score += 3 + (run - 5)
		}
	}
	for r := 0; r < g.size; r++ {
		r := r
		count(func(c int) bool { return g.modules[r][c] })
	}
	for c := 0; c < g.size; c++ {
		c := c
		count(func(r int) bool { return g.modules[r][c] })
	}
	return score
}

// Rule 2: each 2x2 block of a single colour scores 3.
func (g *grid) penaltyBlocks() int {
	score := 0
	for r := 0; r < g.size-1; r++ {
		for c := 0; c < g.size-1; c++ {
			m := g.modules[r][c]
			if m == g.modules[r][c+1] && m == g.modules[r+1][c] && m == g.modules[r+1][c+1] {
				score += 3
			}
		}
	}
	return score
}

// Rule 3: the 1:1:3:1:1 finder-like pattern with four light modules on either
// side scores 40, in rows and columns.
func (g *grid) penaltyFinderLike() int {
	patA := [11]bool{true, false, true, true, true, false, true, false, false, false, false}
	patB := [11]bool{false, false, false, false, true, false, true, true, true, false, true}
	score := 0
	match := func(get func(int) bool, i int, pat [11]bool) bool {
		for k := 0; k < 11; k++ {
			if get(i+k) != pat[k] {
				return false
			}
		}
		return true
	}
	for r := 0; r < g.size; r++ {
		r := r
		get := func(c int) bool { return g.modules[r][c] }
		for c := 0; c+11 <= g.size; c++ {
			if match(get, c, patA) || match(get, c, patB) {
				score += 40
			}
		}
	}
	for c := 0; c < g.size; c++ {
		c := c
		get := func(r int) bool { return g.modules[r][c] }
		for r := 0; r+11 <= g.size; r++ {
			if match(get, r, patA) || match(get, r, patB) {
				score += 40
			}
		}
	}
	return score
}

// Rule 4: deviation of the dark-module proportion from 50%.
func (g *grid) penaltyBalance() int {
	dark := 0
	total := g.size * g.size
	for r := 0; r < g.size; r++ {
		for c := 0; c < g.size; c++ {
			if g.modules[r][c] {
				dark++
			}
		}
	}
	percent := dark * 100 / total
	dev := percent - 50
	if dev < 0 {
		dev = -dev
	}
	return (dev / 5) * 10
}
