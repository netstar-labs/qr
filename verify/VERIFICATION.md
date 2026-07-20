# Table verification record

The version-dependent parameter tables in this package are hand-transcribed
from ISO/IEC 18004 (`ecTable` and `alignmentPositions` in `tables.go`,
`totalCodewordsPerVersion` in `tables.go`, and `dataCapacityCodewords` in
`capacity.go`). To remove the risk of a correlated transcription error, they
were cross-checked against two independent, widely-validated reference
implementations of the same standard.

## Normative source

- **ISO/IEC 18004:2024** (4th edition) — the current normative QR Code
  symbology specification. The error-correction block structure and alignment
  pattern positions for Model-2 QR (versions 1–40) are unchanged across the
  2000 / 2006 / 2015 / 2024 editions.

## Independent references used for the cross-check

- **Project Nayuki — QR-Code-generator** (MIT license)
  `python/qrcodegen.py`, arrays `_ECC_CODEWORDS_PER_BLOCK` and
  `_NUM_ERROR_CORRECTION_BLOCKS`. Data-codeword totals are derived from the raw
  data-module geometry (`_get_num_raw_data_modules`), so this reference shares
  no hand-typed totals with our tables.
  Pinned at commit `777682a64202` (file last modified 2025-01-04).

- **Google ZXing** (Apache-2.0 license)
  `core/.../qrcode/decoder/Version.java`, the `ECBlocks` / `ECB` definitions and
  the `alignmentPatternCenters` arrays.
  Pinned at commit `f1683e1f4f67` (file last modified 2019-05-13).

These two were chosen because they are independently derived from ISO/IEC 18004,
are cross-validated against real-world scanners at scale, and encode the tables
in a *different representation* from ours (Nayuki stores block structure and
derives capacity; we store capacity totals directly), which makes a shared
transcription error very unlikely.

## What was checked

For every one of the **160** (40 versions × 4 error-correction levels) entries,
the tuple `(ecPerBlock, numBlocks, dataCodewords, totalCodewords)` was compared
against both references. In addition:

- `capacity.go` (`dataCapacityCodewords`) was checked against `ecTable`'s
  derived data-codeword count for all 160 entries.
- All **40** alignment-pattern-center rows were compared against ZXing.
- `totalCodewordsPerVersion` was implicitly validated: Nayuki's total is
  computed from module geometry, and it matched our stored totals for every
  version.

## Result

Performed **2026-07-19**.

```
EC parameters: 160 entries (40 versions x 4 levels),
comparing (ecPerBlock, numBlocks, dataCodewords, totalCodewords):
  ecTable      vs Project Nayuki : ALL MATCH
  ecTable      vs Google ZXing   : ALL MATCH
  capacity.go  vs ecTable        : ALL MATCH
  alignment    vs Google ZXing   : ALL MATCH (40 rows)
RESULT: PASS
```

Every table agreed with both independent references, with zero mismatches.

## Reproducing

The audit is reproducible with the standard library only (Python 3, network
access to `raw.githubusercontent.com`). It fetches the two references and parses
the Go tables directly from source:

```
python3 verify/crosscheck.py
```

`verify/crosscheck.py` is a developer audit tool. It is not part of the qr
library, is never imported by it, and does not affect the library's
zero-dependency, pure-Go guarantee.
