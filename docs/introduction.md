# Those little squares are lying to you

You can't read a QR code. Neither can I. That's the whole trick — we point a
camera at a pixelated Rorschach blot and *trust* wherever it takes us. A parking
meter, a restaurant menu, a payment page, a "scan to verify." We've trained a
few billion people to follow an instruction they are constitutionally incapable
of proofreading.

So the machine that draws those squares had better be honest. And the machine
that reads them had better assume everyone else's isn't.

That's this library.

## What it is

`qr` turns text into a QR code and turns a QR code back into text, in Go, using
**nothing but the standard library**. No `go get` cascade, no transitive
supply-chain roulette, no C library wearing a Go trench coat. You can read every
line that stands between your string and the symbol on the screen — and there
aren't many.

It does the whole standard: numeric, alphanumeric, and byte modes; all 40 sizes;
all four error-correction levels; automatic everything (smallest version, tightest
mode, prettiest mask). Hand it a string, get back a symbol that scans on the
first try.

## The part nobody else brags about

Most QR libraries encode. This one **decodes its own output and puts the two in
the same test suite** — so "it probably scans" becomes "it provably round-trips,
across every version and level, on every build."

And the decoder is a paranoid. Here's the thing about Reed-Solomon, the error
correction that lets a coffee-stained code still work: it doesn't care whether
your symbol is *correct*, only whether it's *consistent*. Feed it a hostile blob
crafted to be internally tidy and it will happily hand you back numbers that were
never supposed to exist — an alphanumeric value that indexes off the end of the
alphabet, a length field that claims more data than physically fits. A naive
decoder takes that value and reaches for an array slot that isn't there.

Ours checks. Every value, every length, before it's used. We proved it the only
way that counts: four fuzzers, millions of malformed inputs, zero panics. The
decoder returns an answer or an error. It never falls over.

## Why the tables aren't a leap of faith

A QR encoder is mostly one giant act of transcription — 160 error-correction
parameters copied out of an ISO spec, where a single fat-fingered digit produces
codes that look fine and scan on *nothing*. So we didn't just type them and hope.
Every entry is checked three ways: against itself, against a second independent
transcription, and against two unrelated reference implementations (Nayuki,
ZXing) that derive the same numbers by a different route. Zero mismatches. The
receipts are in `VERIFICATION.md`.

## Point it at something

```go
code, _ := qr.Encode("https://github.com/netstar-labs/qr", qr.High)
code.WritePNGFile("repo-qr.png", 8, 4)
```

Two lines, and out comes a symbol that scans — this library renders it and reads
it back to the same URL to prove it (that exact code is on the cover of the
[executive summary](executive-summary.md)). And when you scan your own, you get
your string back: with error correction, with no dependencies, and with a
decoder that a malicious input can annoy but cannot crash.

The squares still can't tell you where they go. But at least the thing that draws
them won't lie, and the thing that reads them won't flinch.

---

*Next: [executive-summary.md](executive-summary.md) for the capability grid ·
[userguide.md](userguide.md) to start encoding · [architecture.md](architecture.md)
for how the pipeline fits together · [VERIFICATION.md](../verify/VERIFICATION.md) for the
table provenance.*
