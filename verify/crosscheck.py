#!/usr/bin/env python3
"""Independent audit of this package's QR parameter tables.

Diffs tables.go (ecTable, alignmentPositions, totalCodewordsPerVersion) and
capacity.go (dataCapacityCodewords) against two independent, widely-validated
reference implementations derived from ISO/IEC 18004:

  * Project Nayuki  - github.com/nayuki/QR-Code-generator  (MIT)
  * Google ZXing    - github.com/zxing/zxing               (Apache-2.0)

For each of the 40 versions x 4 error-correction levels it compares
(ecPerBlock, numBlocks, dataCodewords, totalCodewords), and it compares all 40
alignment-pattern-center rows against ZXing.

This is a developer audit tool. It is NOT part of the qr library, is never
imported by it, and uses only the Python standard library. The library itself
remains pure-Go / zero-dependency. Run from anywhere:

    python3 verify/crosscheck.py

Exit status is 0 if every table agrees with both references, non-zero if not.
"""
import os
import re
import sys
import urllib.request

LEVELS = ["L", "M", "Q", "H"]  # ordinal order shared by ecTable, Nayuki, ZXing

NAYUKI_URL = ("https://raw.githubusercontent.com/nayuki/QR-Code-generator/"
              "master/python/qrcodegen.py")
ZXING_URL = ("https://raw.githubusercontent.com/zxing/zxing/master/core/src/"
             "main/java/com/google/zxing/qrcode/decoder/Version.java")

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(HERE)


def fetch(url):
    with urllib.request.urlopen(url, timeout=30) as r:
        return r.read().decode("utf-8")


# ----------------------------------------------------------- this package
def load_go_tables():
    tg = open(os.path.join(REPO, "tables.go")).read()
    cg = open(os.path.join(REPO, "capacity.go")).read()

    # ecTable: sequential {ec,g1b,g1w,g2b,g2w} tuples, 4 per version from v1.
    body = tg[tg.index("ecTable"):]
    tuples = re.findall(r"\{(\d+),\s*(\d+),\s*(\d+),\s*(\d+),\s*(\d+)\}", body)
    ectable = {}
    for v in range(1, 41):
        for li in range(4):
            ec, g1b, g1w, g2b, g2w = map(int, tuples[(v - 1) * 4 + li])
            nb = g1b + g2b
            data = g1b * g1w + g2b * g2w
            ectable[(v, LEVELS[li])] = (ec, nb, data, nb * ec + data)

    # dataCapacityCodewords: {a,b,c,d} per version (skip the empty v0 row).
    cbody = cg[cg.index("dataCapacityCodewords"):]
    rows = re.findall(r"\{(\d+),\s*(\d+),\s*(\d+),\s*(\d+)\}", cbody)
    cap = {}
    for v in range(1, 41):
        vals = list(map(int, rows[v - 1]))
        for li in range(4):
            cap[(v, LEVELS[li])] = vals[li]

    # alignmentPositions: 41 brace groups (v0..v40); parse in order.
    abody = tg[tg.index("alignmentPositions"):]
    abody = abody[abody.index("{"):]
    groups = re.findall(r"\{([^{}]*)\}", abody)
    align = {}
    for v in range(0, 41):
        nums = [int(x) for x in re.findall(r"\d+", groups[v])]
        align[v] = nums
    return ectable, cap, align


# ----------------------------------------------------------- Nayuki
def load_nayuki(src):
    def grab(name):
        d = re.search(name + r"\s*:\s*Sequence\[Sequence\[int\]\]\s*=\s*\(", src)
        start = d.end() - 1
        depth = 0
        for i in range(start, len(src)):
            if src[i] == "(":
                depth += 1
            elif src[i] == ")":
                depth -= 1
                if depth == 0:
                    end = i + 1
                    break
        inner = re.sub(r"#.*", "", src[start:end])[1:-1]
        rows = re.findall(r"\(([^()]*)\)", inner)
        return [[int(x) for x in re.findall(r"-?\d+", r)] for r in rows]

    ecpb = grab("_ECC_CODEWORDS_PER_BLOCK")
    nb = grab("_NUM_ERROR_CORRECTION_BLOCKS")

    def raw_modules(ver):
        result = (16 * ver + 128) * ver + 64
        if ver >= 2:
            na = ver // 7 + 2
            result -= (25 * na - 10) * na - 55
            if ver >= 7:
                result -= 36
        return result

    out = {}
    for v in range(1, 41):
        for o, l in enumerate(LEVELS):
            ec = ecpb[o][v]
            blocks = nb[o][v]
            raw = raw_modules(v) // 8            # from geometry, not our tables
            out[(v, l)] = (ec, blocks, raw - ec * blocks, raw)
    return out


# ----------------------------------------------------------- ZXing
def load_zxing(src):
    zx = re.sub(r"\s+", " ", src)
    ec_out, align_out = {}, {}
    for ch in zx.split("new Version(")[1:]:
        mnum = re.match(r"\s*(\d+)\s*,", ch)
        if not mnum:
            continue
        v = int(mnum.group(1))
        if not (1 <= v <= 40):
            continue
        ma = re.search(r"new int\[\]\{([^}]*)\}", ch)
        align_out[v] = [int(x) for x in re.findall(r"\d+", ma.group(1))] if ma else []
        blocks = re.findall(r"new ECBlocks\(\s*(\d+)\s*,(.*?)(?=new ECBlocks\(|$)", ch)
        for li, (ec, rest) in enumerate(blocks[:4]):
            ecbs = re.findall(r"new ECB\(\s*(\d+)\s*,\s*(\d+)\s*\)", rest)
            nb = sum(int(c) for c, _ in ecbs)
            data = sum(int(c) * int(d) for c, d in ecbs)
            ec = int(ec)
            ec_out[(v, LEVELS[li])] = (ec, nb, data, nb * ec + data)
    return ec_out, align_out


def main():
    print("Fetching references ...")
    nay = load_nayuki(fetch(NAYUKI_URL))
    zx_ec, zx_align = load_zxing(fetch(ZXING_URL))
    ectable, cap, align = load_go_tables()

    nay_bad = zx_bad = cap_bad = align_bad = 0
    for v in range(1, 41):
        for l in LEVELS:
            m = ectable[(v, l)]
            if m != nay[(v, l)]:
                nay_bad += 1
                print(f"  MISMATCH v{v}{l} ecTable={m} nayuki={nay[(v,l)]}")
            if m != zx_ec[(v, l)]:
                zx_bad += 1
                print(f"  MISMATCH v{v}{l} ecTable={m} zxing={zx_ec[(v,l)]}")
            if cap[(v, l)] != m[2]:
                cap_bad += 1
                print(f"  MISMATCH capacity.go v{v}{l} cap={cap[(v,l)]} ecTable={m[2]}")
    for v in range(1, 41):
        if align[v] != zx_align.get(v, []):
            align_bad += 1
            print(f"  ALIGN MISMATCH v{v} mine={align[v]} zxing={zx_align.get(v)}")

    print("-" * 64)
    print("EC parameters: 160 entries (40 versions x 4 levels),")
    print("comparing (ecPerBlock, numBlocks, dataCodewords, totalCodewords):")
    print(f"  ecTable      vs Project Nayuki : {'ALL MATCH' if nay_bad==0 else str(nay_bad)+' MISMATCH'}")
    print(f"  ecTable      vs Google ZXing   : {'ALL MATCH' if zx_bad==0 else str(zx_bad)+' MISMATCH'}")
    print(f"  capacity.go  vs ecTable        : {'ALL MATCH' if cap_bad==0 else str(cap_bad)+' MISMATCH'}")
    print(f"  alignment    vs Google ZXing   : {'ALL MATCH' if align_bad==0 else str(align_bad)+' MISMATCH'} (40 rows)")
    ok = not (nay_bad or zx_bad or cap_bad or align_bad)
    print("RESULT:", "PASS" if ok else "FAIL")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
