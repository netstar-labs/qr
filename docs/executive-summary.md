# Executive summary

<img src="repo-qr.png" alt="QR code linking to github.com/netstar-labs/qr" width="150" align="right">

**`qr` is a dependency-free QR Code library for Go — it both generates and reads
back QR symbols, and it treats decoder input as hostile.** It implements the QR
Code standard (ISO/IEC 18004) using nothing but the Go standard library.

The code on the right links to the repository; this library rendered it (level H)
and decoded it back to that URL, which is how we know it scans.

## What it does

| Capability | Detail |
|---|---|
| **Encode** | numeric, alphanumeric, and byte (UTF-8) modes; all 40 versions; all 4 error-correction levels; automatic smallest-fit version, mode, and mask selection |
| **Decode** | full Reed-Solomon **error correction** (not just detection); recovers level and mask from format info; reads a module matrix or a clean PNG/JPEG/GIF |
| **Output** | `image/png`, ASCII, or the raw module matrix |
| **Self-check** | every encoded symbol round-trips through the decoder in the test suite |

## Why it's trustworthy

- **Zero third-party code.** Standard library only — no `unsafe`, no reflection,
  no cgo. The entire dependency footprint is `image` and a handful of core
  packages. Nothing to audit but the code in this repo.
- **The tables are cross-checked three ways.** The 160 error-correction
  parameter entries and 40 alignment rows are (a) internally consistent against
  fixed total-codeword counts, (b) checked against an independently transcribed
  capacity table, and (c) diffed entry-by-entry against Project Nayuki and Google
  ZXing — two independent ISO/IEC 18004 implementations — with zero mismatches.
  See [VERIFICATION.md](../verify/VERIFICATION.md).
- **The decoder assumes malice.** Reed-Solomon correction accepts any consistent
  codeword, so a corrupt or crafted symbol can smuggle out-of-range values into
  the parser. Every such value is bounds-checked; image ingestion caps size and
  dimensions before allocating. Four fuzz targets at millions of executions hold
  the line: **decode returns a value or an error, never a panic.**
- **Correctness is pinned.** A golden corpus locks exact encoder output for
  representative inputs (confirmed against the independent `zbar` scanner);
  round-trip and error-correction tests run across all versions and levels.

## Where it fits

A small, embeddable building block for anywhere a Go service needs to *produce* a
scannable code — tickets, enrollment links, device-pairing payloads, offline
credentials — or *verify* one it rendered. It is a clean-image encoder/decoder,
not a camera pipeline: it does not do perspective correction from photographs.

The repo ships an `example/` server demonstrating the library over three
transports — HTTP, a Unix socket, and **MCP (stdio)** so an AI agent can call QR
generation as a tool — but the library itself is server-free and framework-free.

## The one-line version

Point it at text, get a symbol that scans; point it at a symbol, get the text
back — with real error correction, no dependencies, and a decoder that won't
fall over on a malformed input.
