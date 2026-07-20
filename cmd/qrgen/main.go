// Command qrgen writes a QR Code PNG (or ASCII) for the given text.
//
// Usage:
//
//	qrgen -text "HELLO WORLD" -level Q -out code.png -scale 8 -border 4
//	qrgen -text "HELLO WORLD" -ascii
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/netstar-labs/qr"
)

func main() {
	var (
		text    = flag.String("text", "", "content to encode (required)")
		level   = flag.String("level", "M", "error-correction level: L, M, Q, or H")
		out     = flag.String("out", "qr.png", "output PNG path")
		scale   = flag.Int("scale", 8, "module size in pixels")
		border  = flag.Int("border", 4, "quiet-zone width in modules")
		version = flag.Int("version", 0, "force version 1..40 (0 = auto)")
		ascii   = flag.Bool("ascii", false, "print to stdout as text instead of PNG")
	)
	flag.Parse()

	if *text == "" {
		fmt.Fprintln(os.Stderr, "qrgen: -text is required")
		flag.Usage()
		os.Exit(2)
	}

	lvl, ok := map[string]qr.Level{
		"L": qr.Low, "M": qr.Medium, "Q": qr.Quartile, "H": qr.High,
	}[strings.ToUpper(*level)]
	if !ok {
		fmt.Fprintf(os.Stderr, "qrgen: invalid level %q\n", *level)
		os.Exit(2)
	}

	var (
		code *qr.QRCode
		err  error
	)
	if *version > 0 {
		code, err = qr.EncodeVersion(*text, lvl, *version)
	} else {
		code, err = qr.Encode(*text, lvl)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "qrgen: %v\n", err)
		os.Exit(1)
	}

	if *ascii {
		fmt.Print(code.String())
		return
	}
	if err := code.WritePNGFile(*out, *scale, *border); err != nil {
		fmt.Fprintf(os.Stderr, "qrgen: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (version %d, %dx%d modules)\n", *out, code.Version, code.Size, code.Size)
}
