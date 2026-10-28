#!/usr/bin/env python3
"""Render TuiSense capture frames (from tools/capture) into GIFs.

Each frame is the exact character and colour grid TuiSense drew. Cells are
painted on a fixed grid with DejaVu Sans Mono (which includes the Braille
block used by the 3D view), then ffmpeg builds an optimised GIF.

    python3 tools/capture/render.py docs/media/frames docs/media [scene ...]

Requires: Python 3 with Pillow, ffmpeg, and the DejaVu Sans Mono font.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

from PIL import Image, ImageDraw, ImageFont

FONT = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
BOLD = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf"
# Symbols DejaVu Sans Mono lacks (e.g. U+27F3) come from these, in order.
FALLBACKS = ["/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
             "/usr/share/fonts/truetype/noto/NotoSansMath-Regular.ttf",
             "/usr/share/fonts/truetype/noto/NotoSansSymbols2-Regular.ttf"]
CELL_W, CELL_H, SIZE = 9, 18, 15
FPS = 12


def rgb(h):
    return tuple(int(h[i:i + 2], 16) for i in (0, 2, 4))


# Unicode Braille dot bits: (bit, column, row) for the 2x4 dot grid.
DOTS = [(0x01, 0, 0), (0x02, 0, 1), (0x04, 0, 2), (0x40, 0, 3), (0x08, 1, 0), (0x10, 1, 1), (0x20, 1, 2), (0x80, 1, 3)]


def braille(d, x0, y0, bits, fill):
    """Paint a Braille cell as a terminal does: 2x4 dots on the cell grid."""
    for bit, cx, cy in DOTS:
        if bits & bit:
            x = x0 + 1 + cx * 4
            y = y0 + 1 + cy * 4
            d.rectangle([x, y, x + 2, y + 2], fill=fill)


_notdef = {}
_pick = {}


def has_glyph(font, ch):
    """Whether font draws ch (rather than its .notdef box)."""
    if font not in _notdef:
        _notdef[font] = bytes(font.getmask("\U0010FFFD"))
    return bytes(font.getmask(ch)) != _notdef[font]


def glyph_font(ch, f, fallbacks):
    key = (ch, f)
    if key not in _pick:
        _pick[key] = f
        if not has_glyph(f, ch):
            _pick[key] = next((fb for fb in fallbacks if has_glyph(fb, ch)), f)
    return _pick[key]


def render_frame(frame, font, bold, fallbacks=()):
    rows = frame["rows"]
    cols = sum(len(s["t"]) for s in rows[0])
    img = Image.new("RGB", (cols * CELL_W, len(rows) * CELL_H), (17, 19, 26))
    d = ImageDraw.Draw(img)
    for y, row in enumerate(rows):
        x = 0
        for seg in row:
            n = len(seg["t"])
            d.rectangle([x * CELL_W, y * CELL_H, (x + n) * CELL_W - 1, (y + 1) * CELL_H - 1], fill=rgb(seg["b"]))
            f = bold if seg.get("o") else font
            fg = rgb(seg["f"])
            for i, ch in enumerate(seg["t"]):
                if ch == " ":
                    continue
                code = ord(ch)
                if 0x2800 <= code <= 0x28FF:
                    braille(d, (x + i) * CELL_W, y * CELL_H, code - 0x2800, fg)
                else:
                    d.text(((x + i) * CELL_W, y * CELL_H + 1), ch, font=glyph_font(ch, f, fallbacks), fill=fg)
            x += n
    return img


def render_scene(path, out_gif, scale=1.0):
    font = ImageFont.truetype(FONT, SIZE)
    bold = ImageFont.truetype(BOLD, SIZE) if os.path.exists(BOLD) else font
    fallbacks = [ImageFont.truetype(p, SIZE - 1) for p in FALLBACKS if os.path.exists(p)]
    tmp = tempfile.mkdtemp(prefix="tuisense-render-")
    try:
        n = 0
        with open(path) as fh:
            for line in fh:
                img = render_frame(json.loads(line), font, bold, fallbacks)
                if scale != 1.0:
                    img = img.resize((int(img.width * scale), int(img.height * scale)), Image.LANCZOS)
                img.save(os.path.join(tmp, "%05d.png" % n))
                n += 1
        vf = "split[a][b];[a]palettegen=max_colors=96:stats_mode=diff[p];[b][p]paletteuse=dither=none:diff_mode=rectangle"
        subprocess.run(["ffmpeg", "-loglevel", "error", "-y", "-framerate", str(FPS), "-i",
                        os.path.join(tmp, "%05d.png"), "-vf", vf, "-loop", "0", out_gif], check=True)
        print("%s: %d frames, %.0f KB" % (out_gif, n, os.path.getsize(out_gif) / 1024))
    finally:
        shutil.rmtree(tmp)


def main():
    src, dst = sys.argv[1], sys.argv[2]
    only = set(sys.argv[3:])
    os.makedirs(dst, exist_ok=True)
    names = sorted(f[:-6] for f in os.listdir(src) if f.endswith(".jsonl"))
    for name in names:
        if only and name not in only:
            continue
        # Wide scenes are scaled so every GIF stays about 1000 px wide.
        with open(os.path.join(src, name + ".jsonl")) as fh:
            cols = sum(len(s["t"]) for s in json.loads(fh.readline())["rows"][0])
        scale = min(1.0, 1000.0 / (cols * CELL_W))
        render_scene(os.path.join(src, name + ".jsonl"), os.path.join(dst, name + ".gif"), scale)


if __name__ == "__main__":
    main()
