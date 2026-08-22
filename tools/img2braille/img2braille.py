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

# 4x4 Bayer ordered-dither threshold matrix (0..15). Proper 2D indexing:
# BAYER[y % 4][x % 4] for a pixel at (x, y) — NOT a 1D projection of both
# coordinates, which is what silently broke the first version of this
# script (produced sparse, noisy-looking output instead of even coverage).
BAYER = [
    [0, 8, 2, 10],
    [12, 4, 14, 6],
    [3, 11, 1, 9],
    [15, 7, 13, 5],
]

# The source PNG has a soft drop-shadow/glow around the icon: a thin band of
# low-to-mid alpha pixels between the fully-transparent background and the
# fully-opaque artwork (confirmed via histogram: ~1.13M pixels at alpha<32,
# ~435K at alpha>224, only ~7K in between). That thin band was leaking
# through as scattered stray dots. Hard-threshold it away instead of fading
# it in — this is flat vector art, not a photo, so there's no real
# soft-edge detail worth preserving here.
ALPHA_CUTOFF = 128


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
    src_w, src_h = img.size
    char_h = round((src_h / src_w) * chars_wide * 0.5)
    px_h = char_h * 4

    img = img.resize((px_w, px_h), Image.LANCZOS)
    rgba = img.load()

    dot_grid = [[0] * px_w for _ in range(px_h)]
    for y in range(px_h):
        for x in range(px_w):
            r, g, b, a = rgba[x, y]
            if a < ALPHA_CUTOFF:
                continue  # background or glow halo: never a dot
            lum = 0.2126 * r + 0.7152 * g + 0.0722 * b
            coverage = 1.0 - (lum / 255.0)  # darker source pixel -> denser dots
            # Floor so even light (but still opaque, real artwork) areas keep
            # SOME texture instead of vanishing to zero dots — this is what
            # gives the rose logo its consistently "filled" look rather than
            # empty gaps wherever a highlight/cream area happens to be.
            coverage = max(coverage, 0.22)
            threshold = (BAYER[y % 4][x % 4] + 0.5) / 16
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
                    if a >= ALPHA_CUTOFF:
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

    trimmed = [line.rstrip("⠀") for line in lines]
    print("\n".join(trimmed))
    print("\n--- dims: %d chars wide x %d tall ---\n" % (chars_wide, char_h), file=sys.stderr)

    with open("logo_gen.go.txt", "w", encoding="utf-8") as f:
        f.write("var sommLogoLines = []string{\n")
        for line in lines:
            f.write('\t"%s",\n' % line)
        f.write("}\n\n")
        f.write("var sommLogoColors = [][]string{\n")
        for colors in color_rows:
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
