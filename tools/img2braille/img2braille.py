"""Real image -> Braille Unicode dot-matrix art, same technique as
gentle-ai's rose logo, but with per-character TRUE COLOR sampled straight
from the source image (richer than a fixed row-gradient) and alpha-aware
so the transparent background never contributes stray dots.

Usage: python img2braille.py <input.png> <chars_wide> [crop_bottom_frac]
"""
import sys
from PIL import Image

DOT_BITS = [
    [0x01, 0x08],
    [0x02, 0x10],
    [0x04, 0x20],
    [0x40, 0x80],
]

# 4x4 Bayer ordered-dither threshold matrix (0..15), used so mid-tones get
# textured dot density instead of a hard black/white cutoff.
BAYER = [
    [0, 8, 2, 10],
    [12, 4, 14, 6],
    [3, 11, 1, 9],
    [15, 7, 13, 5],
]


def main():
    path = sys.argv[1]
    chars_wide = int(sys.argv[2])
    crop_bottom_frac = float(sys.argv[3]) if len(sys.argv) > 3 else 0.0

    img = Image.open(path).convert("RGBA")
    if crop_bottom_frac > 0:
        w, h = img.size
        img = img.crop((0, 0, w, int(h * (1 - crop_bottom_frac))))

    # Trim fully-transparent border so the art isn't padded with dead space.
    bbox = img.getchannel("A").getbbox()
    if bbox:
        img = img.crop(bbox)

    px_w = chars_wide * 2
    # preserve aspect ratio; braille cells are roughly square-ish once
    # 2-wide-4-tall dot cells render in a monospace font (~0.5 char aspect),
    # so scale height by the *character* aspect, not the raw pixel aspect.
    src_w, src_h = img.size
    char_h = round((src_h / src_w) * chars_wide * 0.5)
    px_h = char_h * 4

    img = img.resize((px_w, px_h), Image.LANCZOS)
    rgba = img.load()

    # Build the dot grid + a same-size color sample (per braille CELL, not
    # per pixel — one color per 2x4 block, averaged from its "on" pixels).
    dot_grid = [[0] * px_w for _ in range(px_h)]
    for y in range(px_h):
        for x in range(px_w):
            r, g, b, a = rgba[x, y]
            if a < 40:
                continue  # transparent background pixel: never a dot
            # perceptual luminance
            lum = 0.2126 * r + 0.7152 * g + 0.0722 * b
            coverage = 1.0 - (lum / 255.0)  # darker source pixel -> denser dots
            # alpha-fade at soft edges (the source has anti-aliased edges)
            coverage *= a / 255.0
            threshold = (BAYER[y % 4][x % 2 * 2 % 4] + 0.5) / 16
            # NOTE: x%2 varies 0/1 only per dot-column within a cell; combine
            # with y for a real 4x4 dither cell instead of a 4x2 one.
            threshold = (BAYER[y % 4][(x + y * 2) % 4] + 0.5) / 16
            if coverage > threshold:
                dot_grid[y][x] = 1

    lines = []
    color_rows = []
    for cy in range(char_h):
        line = []
        colors = []
        for cx in range(chars_wide):
            bits = 0
            rs = gs = bs = cnt = 0
            for dy in range(4):
                for dx in range(2):
                    px = cx * 2 + dx
                    py = cy * 4 + dy
                    if px >= px_w or py >= px_h:
                        continue
                    if dot_grid[py][px]:
                        bits |= DOT_BITS[dy][dx]
                    r, g, b, a = rgba[px, py]
                    if a > 40:
                        rs += r
                        gs += g
                        bs += b
                        cnt += 1
            line.append(chr(0x2800 + bits))
            if cnt:
                colors.append((rs // cnt, gs // cnt, bs // cnt))
            else:
                colors.append(None)
        lines.append("".join(line))
        color_rows.append(colors)

    # Trim trailing all-blank cells per line (U+2800 == blank braille cell).
    trimmed = []
    for line in lines:
        trimmed.append(line.rstrip("⠀"))
    print("\n".join(trimmed))

    print("\n--- dims: %d chars wide x %d tall ---\n" % (chars_wide, char_h), file=sys.stderr)

    # Emit a Go source fragment: []string of braille lines + a parallel
    # []string of hex colors per cell (only non-blank cells matter, but keep
    # it dense/simple: one hex color per character position, "" for blank).
    with open("logo_gen.go.txt", "w", encoding="utf-8") as f:
        f.write("var sommLogoLines = []string{\n")
        for line in lines:
            f.write('\t"%s",\n' % line)
        f.write("}\n\n")
        f.write("var sommLogoColors = [][]string{\n")
        for cy, colors in enumerate(color_rows):
            f.write("\t{")
            for c in colors:
                if c is None:
                    f.write('"", ')
                else:
                    f.write('"#%02x%02x%02x", ' % c)
            f.write("},\n")
        f.write("}\n")


if __name__ == "__main__":
    main()
