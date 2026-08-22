"""Real image -> Braille Unicode (U+2800) dot-matrix art, same technique as
gentle-ai's rose logo (internal/tui/styles/logo.go in that repo), except
colored per Braille cell from the source image's own pixels instead of a
fixed row gradient.

Auto-detects two source styles:

- Icon on a transparent background (real alpha channel with variation):
  background/foreground split by alpha, "on" = darker source pixel
  (inverted luminance), ordered-dither shaded so tonal range still reads.
- Art on a solid background (no meaningful alpha, e.g. RGB/JPEG-style flat
  navy background with bright dots/lines drawn on it): background/
  foreground split by color distance from a sampled background reference,
  "on" = brighter/more-saturated-than-background. No dithering — this mode
  is meant for sources that are ALREADY a sparse dot/line illustration
  (dithering on top of already-thin lines just makes them disappear
  further); a straight threshold preserves the source's own linework.
  Per-cell color is a distance-squared-weighted average so a handful of
  bright dot pixels aren't diluted by many barely-above-threshold dim ones.

Usage: python img2braille.py <input.png> <chars_wide> [crop_bottom_frac]
"""
import math
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
# coordinates, which is what silently broke an early version of this
# script (produced sparse, noisy-looking output instead of even coverage).
BAYER = [
    [0, 8, 2, 10],
    [12, 4, 14, 6],
    [3, 11, 1, 9],
    [15, 7, 13, 5],
]

# Icon-on-transparent mode: the source PNG can have a soft drop-shadow/glow
# around the icon: a thin band of low-to-mid alpha pixels between the
# fully-transparent background and the fully-opaque artwork. That thin band
# leaks through as scattered stray dots if alpha-blended in. Hard-threshold
# it away instead — this is flat vector art, not a photo, so there's no
# real soft-edge detail worth preserving.
ALPHA_CUTOFF = 128

# Art-on-solid-background mode thresholds (color distance from the sampled
# background reference).
BG_DIST_CUTOFF = 22
BG_DIST_SATURATE = 130


def has_real_alpha(img: Image.Image) -> bool:
    if "A" not in img.getbands():
        return False
    a = img.getchannel("A")
    lo, hi = a.getextrema()
    return hi - lo > 40  # meaningful transparency, not just a flat 255 plane


def convert_icon_on_transparent(img: Image.Image, chars_wide: int):
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
            coverage = max(coverage, 0.22)  # keep light-but-opaque areas textured
            threshold = (BAYER[y % 4][x % 4] + 0.5) / 16
            if coverage > threshold:
                dot_grid[y][x] = 1

    def pixel_color(x, y):
        r, g, b, a = rgba[x, y]
        return (r, g, b) if a >= ALPHA_CUTOFF else None

    return px_w, px_h, char_h, dot_grid, pixel_color


def convert_art_on_solid_bg(img: Image.Image, chars_wide: int):
    w, h = img.size
    px = img.load()
    corners = [px[2, 2], px[w - 3, 2], px[2, h - 3], px[w - 3, h - 3]]
    bg = tuple(sum(c[i] for c in corners) / 4 for i in range(3))

    def dist(r, g, b):
        return math.sqrt((r - bg[0]) ** 2 + (g - bg[1]) ** 2 + (b - bg[2]) ** 2)

    px_w = chars_wide * 2
    char_h = round((h / w) * chars_wide * 0.5)
    px_h = char_h * 4

    img = img.resize((px_w, px_h), Image.LANCZOS)
    rgb = img.load()

    dot_grid = [[0] * px_w for _ in range(px_h)]
    for y in range(px_h):
        for x in range(px_w):
            r, g, b = rgb[x, y]
            if dist(r, g, b) >= BG_DIST_CUTOFF:
                dot_grid[y][x] = 1  # straight threshold: preserve already-thin linework

    def pixel_color(x, y):
        r, g, b = rgb[x, y]
        d = dist(r, g, b)
        return (r, g, b, d * d) if d >= BG_DIST_CUTOFF else None  # weight = d^2

    return px_w, px_h, char_h, dot_grid, pixel_color


def main():
    path = sys.argv[1]
    chars_wide = int(sys.argv[2])
    crop_bottom_frac = float(sys.argv[3]) if len(sys.argv) > 3 else 0.0

    raw = Image.open(path)
    mode_rgba = raw.convert("RGBA")

    if crop_bottom_frac > 0:
        w, h = mode_rgba.size
        mode_rgba = mode_rgba.crop((0, 0, w, int(h * (1 - crop_bottom_frac))))

    bbox = mode_rgba.getchannel("A").getbbox() if has_real_alpha(mode_rgba) else None
    if bbox:
        mode_rgba = mode_rgba.crop(bbox)

    if has_real_alpha(mode_rgba):
        px_w, px_h, char_h, dot_grid, pixel_color = convert_icon_on_transparent(mode_rgba, chars_wide)
        weighted = False
    else:
        px_w, px_h, char_h, dot_grid, pixel_color = convert_art_on_solid_bg(mode_rgba.convert("RGB"), chars_wide)
        weighted = True

    lines = []
    color_rows = []
    for cy in range(char_h):
        line = []
        colors = []
        for cx in range(chars_wide):
            bits = 0
            rs = gs = bs = wsum = 0.0
            for dy in range(4):
                for dx in range(2):
                    x = cx * 2 + dx
                    y = cy * 4 + dy
                    if x >= px_w or y >= px_h:
                        continue
                    if dot_grid[y][x]:
                        bits |= DOT_BITS[dy][dx]
                    c = pixel_color(x, y)
                    if c is None:
                        continue
                    if weighted:
                        r, g, b, w_ = c
                    else:
                        r, g, b = c
                        w_ = 1.0
                    rs += r * w_
                    gs += g * w_
                    bs += b * w_
                    wsum += w_
            line.append(chr(0x2800 + bits))
            colors.append((int(rs / wsum), int(gs / wsum), int(bs / wsum)) if wsum else None)
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
                f.write('"%s", ' % ("" if c is None else "#%02x%02x%02x" % c))
            f.write("},\n")
        f.write("}\n")


if __name__ == "__main__":
    main()
