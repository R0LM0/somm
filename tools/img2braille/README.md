# img2braille

Converts a source PNG (with alpha) into Braille Unicode (U+2800) dot-matrix
art plus a per-character true-color map, sampled straight from the image —
the same idea as gentle-ai's rose logo (`internal/tui/styles/logo.go` in
that repo), except colored per Braille cell from the source instead of a
fixed row gradient.

Used once to generate `cmd/somm/logo_data.go`'s `sommLogoLines` /
`sommLogoColors` from `img/grapes.png`. Not built or run by `somm`
itself — a one-off asset pipeline, kept here so the artwork is
reproducible if it ever needs to change.

`img/vine_somm.png` (a wine glass inside a terminal-window icon) was the
first source tried — its straight rectangular frame edges dithered as
visibly "boxy/geometric" compared to gentle-ai's organic rose. Round,
glossy, gradient-shaded subjects (like this grape cluster) match that
technique far better; keep that in mind if choosing a different source
image later.

## Requirements

Python 3 with Pillow (`pip install pillow`).

## Usage

```bash
PYTHONIOENCODING=utf-8 python img2braille.py <input.png> <chars_wide> [crop_bottom_frac]
```

- `chars_wide` — target width in terminal characters (height is derived to
  preserve the source image's aspect ratio).
- `crop_bottom_frac` — optional, 0..1: crops off the bottom fraction of the
  image before converting (used to drop a wordmark baked into the source
  image when the caller already renders that text separately).

Prints the Braille art to stdout and writes a ready-to-paste Go fragment
(`sommLogoLines`/`sommLogoColors`) to `logo_gen.go.txt` next to the script.

Example (what generated the current `logo_data.go`):

```bash
PYTHONIOENCODING=utf-8 python img2braille.py ../../img/grapes.png 40
```

## Notes on quality

- The 4x4 Bayer ordered-dither threshold must be indexed as
  `BAYER[y % 4][x % 4]` — a 1D projection of both coordinates onto one axis
  (an earlier version of this script did that by mistake) produces sparse,
  noisy-looking output instead of even coverage.
- A source image with a soft drop-shadow/glow around the artwork (common in
  AI-generated icons) leaves a thin band of low/mid-alpha pixels between the
  fully-transparent background and the fully-opaque icon. Left alone, that
  band renders as scattered stray dots. `ALPHA_CUTOFF` hard-thresholds it
  away instead of alpha-blending it in — check the alpha histogram
  (`img.getchannel("A").histogram()`) if a new source image still looks
  noisy after conversion.
