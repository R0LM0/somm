# Reference Data — OpenCode Go Plan (authoritative curation snapshot)

Source: OpenCode live documentation, captured 2026-08-21. This is a point-in-time snapshot
pasted by the user; it is **not re-fetchable or re-verifiable by later phases**. Treat it as
the source-of-truth curation input for the new shape/tier table. Do not attempt to re-scrape.

Model ids follow `opencode-go/<model-id>`, where `<model-id>` is the lowercase-hyphenated form
of the display name (e.g. `grok-4.5`, `gpt-5.6-luna`, `glm-5.3`, `kimi-k3`, `deepseek-v4-pro`,
`mimo-v2.5`, `minimax-m3`, `qwen3.8-max`, `hy3`, `muse-spark-1.2-contributor`, `ox-alpha-free`).

## Legend

- **Shape** — OpenCode's published observed per-request token pattern: `input / cached / output`.
  The docs call these "observed request patterns" and they are rounded; derived numbers are
  estimates, not byte-exact reproductions.
- **Price** — USD per 1M tokens: `input / output / cache-read / cache-write` (`—` = not published).
- **Tier** — per-model included-usage allowance in USD (`$15` / `$30` / `$60`). This is the
  validated month-window denominator input, **not** the flat account-wide $12/$30/$60 ceiling.
- **Reqs** — OpenCode's displayed request estimates: `5h / week / month`.

## Table

| Model | Shape (in/cached/out) | Price (in/out/cache-read/cache-write) | Tier | Reqs (5h/week/month) |
|---|---|---|---|---|
| Grok 4.5 | 1100 / 71500 / 220 | 2.00 / 6.00 / 0.30 / — | $15 | 120 / 300 / 600 |
| GPT 5.6 Luna | 1000 / 50000 / 220 | ≤272K: 0.20 / 1.20 / 0.02 / 0.25 · >272K: 0.40 / 1.80 / 0.04 / 0.50 | $15 | 2050 / 5100 / 10250 |
| GLM-5.3 | 700 / 52000 / 150 | 1.40 / 4.40 / 0.26 / — | $15 | 220 / 540 / 1080 |
| GLM-5.2 | 700 / 52000 / 150 | 1.40 / 4.40 / 0.26 / — | $60 | 880 / 2150 / 4300 |
| GLM-5.1 | 700 / 52000 / 150 | 1.40 / 4.40 / 0.26 / — | $60 | 880 / 2150 / 4300 |
| Kimi K3 | 1050 / 76500 / 300 | 3.00 / 15.00 / 0.30 / — | $15 | 110 / 250 / 490 |
| Kimi K2.7 Code | 870 / 55000 / 200 | 0.95 / 4.00 / 0.19 / — | $60 | 1350 / 3380 / 6750 |
| Kimi K2.6 | 870 / 55000 / 200 | 0.95 / 4.00 / 0.16 / — | $60 | 1150 / 2880 / 5750 |
| MiMo-V2.5 | 830 / 71500 / 295 | 0.14 / 0.28 / 0.0028 / — | $60 | 30100 / 75200 / 150400 |
| MiMo-V2.5-Pro | 790 / 86000 / 305 | 0.435 / 0.87 / 0.003625 / — | $15 | 3250 / 8150 / 16300 |
| MiniMax M3 | 510 / 56000 / 190 | 0.30 / 1.20 / 0.06 / — | $60 | 3200 / 8000 / 16000 |
| MiniMax M2.7 | 300 / 55000 / 125 | 0.30 / 1.20 / 0.06 / 0.375 | $60 | 3400 / 8500 / 17000 |
| MiniMax M2.5 | n/a (absent from estimate table) | 0.30 / 1.20 / 0.06 / 0.375 | $60 | n/a |
| Muse Spark 1.2 Contributor | 620 / 71400 / 300 | 0.10 / 0.20 / 0.002 / — | $60 | 45300 / 113300 / 226600 |
| Qwen3.8 Max | 420 / 66000 / 200 | 2.00 / 6.00 / 0.25 / 2.50 | $15 | 160 / 400 / 810 |
| Qwen3.7 Max | 420 / 66000 / 200 | 2.50 / 7.50 / 0.50 / 3.125 | $60 | 340 / 840 / 1690 |
| Qwen3.7 Plus | 500 / 57000 / 190 | ≤256K: 0.40 / 1.60 / 0.04 / 0.50 · >256K: 1.20 / 4.80 / 0.12 / 1.50 | $60 | 4300 / 10800 / 21600 |
| Qwen3.6 Plus | 500 / 57000 / 190 | ≤256K: 0.50 / 3.00 / 0.05 / 0.625 · >256K: 2.00 / 6.00 / 0.20 / 2.50 | $60 | 3300 / 8200 / 16300 |
| DeepSeek V4 Pro | 750 / 82000 / 290 | off-peak: 0.66 / 1.98 / 0.022 / — · peak: 1.32 / 3.96 / 0.044 / — | $15 | 1050 / 2600 / 5200 |
| DeepSeek V4 Flash | 410 / 71300 / 310 | off-peak: 0.22 / 0.66 / 0.007 / — · peak: 0.44 / 1.32 / 0.014 / — | $30 | 7600 / 18900 / 37800 |
| DeepSeek V4 Flash Vision Exp | 410 / 71300 / 310 | off-peak: 0.22 / 0.66 / 0.007 / — · peak: 0.44 / 1.32 / 0.014 / — | $15 | 3800 / 9450 / 18900 |
| Hy3 | 830 / 71500 / 295 | 0.14 / 0.58 / 0.035 / — | $60 | base 4300 / 10750 / 21500 |
| Ox Alpha Free | none | none (free) | — | none |

## Model-specific notes

- **Hy3 multiplier = 8.** The live bar chart shows 34,400 req/5h labelled "8x usage";
  34400 = 8 × 4300 (base). Every other model's `multiplier` defaults to 1.
- **DeepSeek peak hours**: 01:00–04:00 and 06:00–10:00 UTC. All other hours are off-peak.
  The docs' displayed request estimates validate against **off-peak** prices.
- **DeepSeek V4 Flash Vision Exp**: images convert to tokens by dimension and bill as input
  tokens alongside text. Out of scope for this change — noted only.
- **Ox Alpha Free**: no shape, no price, no quota. Excluded from the table. Already safely
  filtered upstream by the nil/zero-price gate in `collectCandidates`, so it cannot cause a
  `P_min = 0` division.
- **Muse Spark 1.2 Contributor**: has shape, price, and tier here, but no `PROVIDER_ALIASES`
  entry, so OpenRouter never supplies it a live price. See proposal decision D3.
- **MiniMax M2.5**: priced but absent from OpenCode's own estimate table — no shape available
  to curate. Design must decide whether it is table-absent (fallback path) or shape-inherited
  from M2.7.

## Formula validation (from exploration)

`requests_per_month ≈ tier_usd / cost_per_request(shape, price)`, using off-peak prices.

| Model | Published month | Computed month | Delta |
|---|---|---|---|
| Grok 4.5 | 600 | 600.7 | ~0% |
| Hy3 (base) | 21500 | 21507 | ~0% |
| DeepSeek V4 Pro (off-peak) | 5200 | 5203 | ~0% |
| MiniMax M3 | 16000 | 16043 | ~0% |
| Kimi K2.7 Code / Kimi K2.6 / GLM-5.3 | — | — | 10–36% drift (rounded published shapes) |

Window ratios: month/week is a consistent ~2.0x across every sampled model. month/5h ranges
4.45x–5.06x — **no exact universal divisor**. This is proposal decision D1, not a solved value.

The flat $12/$30/$60 account-wide limits reproduce Grok's figures ~4.0x too high uniformly
across all three windows, consistent with a separate account-wide ceiling layered on top of the
per-model tier math. Do **not** use them as `window_budget_usd`.
