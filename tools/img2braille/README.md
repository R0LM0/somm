# img2braille

Converts a source PNG into Braille Unicode (U+2800) dot-matrix art plus a
per-character true-color map, sampled straight from the image — the same
idea as gentle-ai's rose logo (`internal/tui/styles/logo.go` in that repo),
except colored per Braille cell from the source instead of a fixed row
gradient.

Used once to generate `cmd/somm/logo_data.go`'s `sommLogoLines` /
`sommLogoColors` from `img/logo.png`. Not built or run by `somm` itself —
a one-off asset pipeline, kept here so the artwork is reproducible if it
ever needs to change.

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
PYTHONIOENCODING=utf-8 python img2braille.py ../../img/logo.png 42 0.32
```

## Two auto-detected source styles

- **Icon on a transparent background** (real alpha channel with
  variation) — background/foreground split by alpha, "on" = darker source
  pixel (inverted luminance), ordered-dither shaded so tonal range still
  reads. Used for `img/vine_somm.png` and `img/grapes.png` (both earlier
  attempts, kept in `img/` for reference).
- **Art on a solid background** (no meaningful alpha — e.g. a flat navy
  background with bright dots/lines already drawn on it) —
  background/foreground split by color distance from a sampled background
  reference, "on" = brighter/more-saturated-than-background, no dithering.
  This is the mode `img/logo.png` uses: it's already a sparse dot/particle
  illustration, so dithering on top would just thin out the linework
  further — a straight threshold preserves it. Per-cell color is a
  distance-squared-weighted average so a few bright dot pixels aren't
  diluted by the many barely-above-threshold dim ones a naive average
  would include.

## Notes on quality

- The 4x4 Bayer ordered-dither threshold (icon-on-transparent mode) must
  be indexed as `BAYER[y % 4][x % 4]` — a 1D projection of both
  coordinates onto one axis (an earlier version of this script did that by
  mistake) produces sparse, noisy-looking output instead of even coverage.
- A source image with a soft drop-shadow/glow around the artwork (common
  in AI-generated icons) leaves a thin band of low/mid-alpha pixels
  between the fully-transparent background and the fully-opaque icon.
  Left alone, that band renders as scattered stray dots. `ALPHA_CUTOFF`
  hard-thresholds it away instead of alpha-blending it in.
- A round, glossy, gradient-shaded subject (grapes) dithers far more like
  gentle-ai's organic rose than a subject with hard rectangular edges (a
  terminal-window frame) — the latter reads as visibly "boxy/geometric"
  regardless of dithering tuning. Worth choosing the source image with
  this in mind if picking something new for the icon-on-transparent mode.
- For the art-on-solid-background mode, an already-thin/sparse source
  (like a particle/constellation-style illustration) needs a straight
  threshold, not dithering — dithering assumes continuous tone to work
  with, and further thins out linework that's already sparse.
