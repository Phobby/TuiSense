#!/usr/bin/env python3
"""Generate TuiSenseBraille.woff2: the 256 Unicode braille patterns
(U+2800-U+28FF) drawn as round dots on JetBrains Mono's cell (600 x 1320
units, ascent 1020, descent 300).

TuiSense draws its 3D controllers with braille "pixels". Few monospace web
fonts include braille, and system fallbacks have other widths, so the web
demo ships this small, generated font (MIT, like TuiSense) to keep every
dot on the terminal's cell grid.

Usage: python3 scripts/make_braille_font.py src/fonts/TuiSenseBraille.woff2
Requires: pip install fonttools brotli
"""
import math
import sys

from fontTools.fontBuilder import FontBuilder
from fontTools.pens.ttGlyphPen import TTGlyphPen

UPM, ADVANCE, ASCENT, DESCENT = 1000, 600, 1020, 300
COLS = (150, 450)                       # dot centres, x
ROW_PITCH = (ASCENT + DESCENT) / 4      # 330
ROWS = [ASCENT - ROW_PITCH * (i + 0.5) for i in range(4)]  # 855, 525, 195, -135
RADIUS = 92
# Unicode dot numbering: bit -> (column, row)
DOTS = [(0, 0), (0, 1), (0, 2), (1, 0), (1, 1), (1, 2), (0, 3), (1, 3)]


def dot(pen, cx, cy, r, segments=16):
    """A closed polygonal circle, clockwise (TrueType outer contour)."""
    pts = []
    for i in range(segments):
        a = -2 * math.pi * i / segments  # clockwise
        pts.append((round(cx + r * math.cos(a)), round(cy + r * math.sin(a))))
    pen.moveTo(pts[0])
    for p in pts[1:]:
        pen.lineTo(p)
    pen.closePath()


def main(out):
    names = [".notdef"] + ["uni%04X" % (0x2800 + n) for n in range(256)]
    glyphs, metrics, cmap = {}, {}, {}
    empty = TTGlyphPen(None)
    glyphs[".notdef"] = empty.glyph()
    metrics[".notdef"] = (ADVANCE, 0)
    for n in range(256):
        name = names[n + 1]
        pen = TTGlyphPen(None)
        for bit, (col, row) in enumerate(DOTS):
            if n & (1 << bit):
                dot(pen, COLS[col], ROWS[row], RADIUS)
        g = pen.glyph()
        glyphs[name] = g
        metrics[name] = (ADVANCE, COLS[0] - RADIUS if n else 0)
        cmap[0x2800 + n] = name

    fb = FontBuilder(UPM, isTTF=True)
    fb.setupGlyphOrder(names)
    fb.setupCharacterMap(cmap)
    fb.setupGlyf(glyphs)
    fb.setupHorizontalMetrics(metrics)
    fb.setupHorizontalHeader(ascent=ASCENT, descent=-DESCENT)
    fb.setupNameTable({
        "familyName": "TuiSense Braille",
        "styleName": "Regular",
        "copyright": "Copyright (c) TuiSense contributors. MIT License.",
        "version": "Version 1.0",
    })
    fb.setupOS2(sTypoAscender=ASCENT, sTypoDescender=-DESCENT, sTypoLineGap=0,
                usWinAscent=ASCENT, usWinDescent=DESCENT, xAvgCharWidth=ADVANCE,
                fsType=0)
    fb.setupPost(isFixedPitch=1)
    fb.font.flavor = "woff2"
    fb.save(out)


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else "TuiSenseBraille.woff2")
