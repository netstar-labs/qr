package qr

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

const goldenPath = "golden.txt"

func goldenKey(c goldenCase, actualVersion int) string {
	// Key by content hash-ish label + level + resolved version so auto-version
	// cases remain stable.
	label := c.content
	if len(label) > 24 {
		label = fmt.Sprintf("%s..(%d)", label[:16], len(c.content))
	}
	return fmt.Sprintf("%s|L%d|v%d", label, c.level, actualVersion)
}

func loadGolden(t *testing.T) map[string]string {
	t.Helper()
	m := map[string]string{}
	f, err := os.Open(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return m
		}
		t.Fatalf("open golden: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.LastIndex(line, " ")
		if i < 0 {
			continue
		}
		m[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
	}
	return m
}

func saveGolden(t *testing.T, m map[string]string) {
	t.Helper()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString("# QR encoder golden digests (SHA-256 of module matrix).\n")
	sb.WriteString("# Regenerated automatically when a new case appears; a change to an\n")
	sb.WriteString("# existing line indicates encoder output drift.\n")
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s %s\n", k, m[k])
	}
	if err := os.WriteFile(goldenPath, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
}
