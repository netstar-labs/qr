# User guide

Everything you need to encode, render, decode, and run the example server.
`import "github.com/netstar-labs/qr"` — standard library only, nothing to
`go get` transitively.

## Encoding

```go
code, err := qr.Encode("HELLO WORLD", qr.Quartile)
if err != nil {
    // only returned when content exceeds v40 capacity at this level
}
fmt.Println(code.Version, code.Size) // 1 21
```

`Encode` chooses the smallest version, the most compact mode, and the
lowest-penalty mask automatically. The four error-correction levels trade
capacity for recoverable damage:

| Level | Constant | Recovery | Use when |
|---|---|---|---|
| L | `qr.Low` | ~7% | clean digital display, maximum data |
| M | `qr.Medium` | ~15% | the general default |
| Q | `qr.Quartile` | ~25% | print, or a logo overlaid |
| H | `qr.High` | ~30% | small/dirty/abraded surfaces |

To pin a version (it errors if the content will not fit):

```go
code, err := qr.EncodeVersion("DATA", qr.High, 5)
```

**Mode is automatic.** All-digits → numeric (densest); digits + uppercase +
`space $%*+-./:` → alphanumeric; anything else → byte mode, which emits raw
UTF-8 bytes. You never select a mode; you just get the tightest one your content
allows.

## Rendering

```go
// PNG: 8 pixels per module, 4-module quiet zone (the standard minimum).
f, _ := os.Create("code.png")
code.PNG(f, 8, 4)
f.Close()

// or in one call, with cleanup on failure:
code.WritePNGFile("code.png", 8, 4)

// ASCII, for a terminal:
fmt.Print(code.String())

// raw modules, to render yourself (true = dark):
for _, row := range code.Matrix() { /* ... */ }
```

`PNG` refuses a render whose pixel side would exceed 32768 — a guard against an
accidental gigapixel allocation, not a real symbol.

## Decoding

```go
// From a symbol you just built (a self-check):
dec, err := code.Decode()
fmt.Println(dec.Text, dec.Mode, dec.Errors) // "HELLO WORLD" "alphanumeric" 0

// From a clean image (PNG, JPEG, or GIF):
img, _ := os.Open("code.png")
dec, err := qr.DecodePNG(img)
```

`Decoded` carries the recovered `Version`, `Level`, `Mask`, `Mode`, `Text`, and
`Errors` (the number of codeword errors Reed-Solomon repaired). Decoding applies
**error correction**, so a symbol with damage up to the level's budget still
reads.

**What the decoder is and isn't:** it reads an upright, high-contrast image with
square modules on a light quiet zone — a render, screenshot, or flat scan. It is
not a photo decoder: no perspective correction, rotation, or lens handling. And
it is hardened against hostile input — a malformed or crafted symbol yields an
error, never a panic.

## Command line

```sh
go build -o qrgen ./cmd/qrgen

./qrgen -text "HELLO WORLD" -level Q -out code.png -scale 8 -border 4
./qrgen -text "HELLO WORLD" -ascii          # print to the terminal
./qrgen -text "DATA" -version 5 -level H    # force a version
```

## Example servers

`example/` holds three small, self-contained programs — one per transport — that
embed the library. Each is its own `main`; none is part of the library or adds a
dependency to it. Run any directly:

```sh
# HTTP:
go run ./example/http -addr :8080
curl 'http://localhost:8080/qr?text=HELLO&level=H&scale=8' -o code.png
curl 'http://localhost:8080/qr?text=HELLO&fmt=txt'          # ASCII

# Unix socket (local sidecar, filesystem-permissioned):
go run ./example/unix -sock /tmp/qr.sock
curl --unix-socket /tmp/qr.sock 'http://qr/qr?text=HELLO'

# MCP over stdio — register with an AI agent; it calls generate_qr as a tool:
go run ./example/mcp
```

The HTTP and Unix examples share the same request:
`GET /qr?text=…&level=L|M|Q|H&scale=N&fmt=png|txt`. The MCP example advertises
one tool, `generate_qr`, taking `{text, level?, png?}` and returning ASCII text
or a base64 PNG.

## Limits & errors

- `Encode` errors only when content exceeds the largest symbol (v40) at the
  chosen level — drop to a lower level or shorten the input.
- Kanji mode and ECI are not implemented; non-Latin text goes through byte mode
  as UTF-8, which every modern scanner reads correctly.
- Decoding rejects: non-square or wrong-size matrices, unrecoverable format
  info, uncorrectable blocks (damage beyond the EC budget), and payloads whose
  values fall outside their mode's range.
