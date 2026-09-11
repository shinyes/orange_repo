// OKLCH -> sRGB conversion for the Tailwind v4 -> v3.4 migration.
//
// Why: Chrome 109 (last Chrome for Windows 7) does not support oklch()/lab()/lch()
// nor color-mix(). Every theme token in src/index.css is authored in oklch(), so the
// tokens must become sRGB channel triplets that tailwind.config.js can wrap as
// `rgb(var(--x-rgb) / <alpha-value>)`.
//
// Math: Bjorn Ottosson's OKLab definitions (oklab -> LMS' -> LMS^3 -> linear sRGB),
// then the sRGB transfer function. Verified by round-tripping sRGB -> OKLCH.
//
// Usage: node scripts/oklch-to-srgb.mjs
//        node scripts/oklch-to-srgb.mjs --emit   (prints the :root/.dark CSS blocks)

/* ---------- color math ---------- */

const srgbEncode = (c) => (c <= 0.0031308 ? 12.92 * c : 1.055 * Math.pow(c, 1 / 2.4) - 0.055)
const srgbDecode = (c) => (c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4))

/** oklch(L, C, H_deg) -> linear sRGB [r,g,b] (unclamped, may fall outside gamut). */
function oklchToLinearSrgb(L, C, H) {
  const hRad = (H * Math.PI) / 180
  const a = C * Math.cos(hRad)
  const b = C * Math.sin(hRad)

  const l_ = L + 0.3963377774 * a + 0.2158037573 * b
  const m_ = L - 0.1055613458 * a - 0.0638541728 * b
  const s_ = L - 0.0894841775 * a - 1.291485548 * b

  const l = l_ * l_ * l_
  const m = m_ * m_ * m_
  const s = s_ * s_ * s_

  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ]
}

/** linear sRGB -> oklch, for round-trip verification. */
function linearSrgbToOklch([r, g, b]) {
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b)
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b)
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b)

  const L = 0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s
  const A = 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s
  const B = 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s

  const C = Math.hypot(A, B)
  let H = (Math.atan2(B, A) * 180) / Math.PI
  if (H < 0) H += 360
  return [L, C, H]
}

/**
 * Convert to an 8-bit sRGB triplet.
 * Reports gamut clipping so out-of-sRGB tokens are visible rather than silent.
 */
export function oklchTriplet(L, C, H, alpha = 1) {
  const linear = oklchToLinearSrgb(L, C, H)
  const encoded = linear.map(srgbEncode)
  const clamped = encoded.map((c) => Math.min(1, Math.max(0, c)))
  const channels = clamped.map((c) => Math.round(c * 255))
  const clip = Math.max(...encoded.map((c) => Math.max(0, -c, c - 1)))
  const hex = '#' + channels.map((c) => c.toString(16).padStart(2, '0')).join('')
  return { channels, hex, alpha, clip, linear }
}

const triplet = (ch) => ch.join(' ')

/* ---------- tokens (transcribed 1:1 from src/index.css) ---------- */

// [name, L, C, H, alphaPercent]
const LIGHT = [
  ['background', 1, 0, 0, 100],
  ['foreground', 0.145, 0, 0, 100],
  ['card', 1, 0, 0, 100],
  ['card-foreground', 0.145, 0, 0, 100],
  ['popover', 1, 0, 0, 100],
  ['popover-foreground', 0.145, 0, 0, 100],
  ['primary', 0.608, 0.19, 39, 100],
  ['primary-foreground', 0.985, 0, 0, 100],
  ['secondary', 0.97, 0, 0, 100],
  ['secondary-foreground', 0.205, 0, 0, 100],
  ['muted', 0.97, 0, 0, 100],
  ['muted-foreground', 0.556, 0, 0, 100],
  ['accent', 0.97, 0, 0, 100],
  ['accent-foreground', 0.205, 0, 0, 100],
  ['destructive', 0.577, 0.245, 27.325, 100],
  ['border', 0.922, 0, 0, 100],
  ['input', 0.922, 0, 0, 100],
  ['ring', 0.7, 0.16, 40, 100],
  ['chart-1', 0.87, 0, 0, 100],
  ['chart-2', 0.556, 0, 0, 100],
  ['chart-3', 0.439, 0, 0, 100],
  ['chart-4', 0.371, 0, 0, 100],
  ['chart-5', 0.269, 0, 0, 100],
  ['sidebar', 0.985, 0, 0, 100],
  ['sidebar-foreground', 0.145, 0, 0, 100],
  ['sidebar-primary', 0.205, 0, 0, 100],
  ['sidebar-primary-foreground', 0.985, 0, 0, 100],
  ['sidebar-accent', 0.97, 0, 0, 100],
  ['sidebar-accent-foreground', 0.205, 0, 0, 100],
  ['sidebar-border', 0.922, 0, 0, 100],
  ['sidebar-ring', 0.708, 0, 0, 100],
]

const DARK = [
  ['background', 0.145, 0, 0, 100],
  ['foreground', 0.985, 0, 0, 100],
  ['card', 0.205, 0, 0, 100],
  ['card-foreground', 0.985, 0, 0, 100],
  ['popover', 0.205, 0, 0, 100],
  ['popover-foreground', 0.985, 0, 0, 100],
  ['primary', 0.72, 0.16, 42, 100],
  ['primary-foreground', 0.205, 0, 0, 100],
  ['secondary', 0.269, 0, 0, 100],
  ['secondary-foreground', 0.985, 0, 0, 100],
  ['muted', 0.269, 0, 0, 100],
  ['muted-foreground', 0.708, 0, 0, 100],
  ['accent', 0.269, 0, 0, 100],
  ['accent-foreground', 0.985, 0, 0, 100],
  ['destructive', 0.704, 0.191, 22.216, 100],
  ['border', 1, 0, 0, 10], // alpha-carrying in v4
  ['input', 1, 0, 0, 15], // alpha-carrying in v4
  ['ring', 0.65, 0.15, 40, 100],
  ['chart-1', 0.87, 0, 0, 100],
  ['chart-2', 0.556, 0, 0, 100],
  ['chart-3', 0.439, 0, 0, 100],
  ['chart-4', 0.371, 0, 0, 100],
  ['chart-5', 0.269, 0, 0, 100],
  ['sidebar', 0.205, 0, 0, 100],
  ['sidebar-foreground', 0.985, 0, 0, 100],
  ['sidebar-primary', 0.488, 0.243, 264.376, 100],
  ['sidebar-primary-foreground', 0.985, 0, 0, 100],
  ['sidebar-accent', 0.269, 0, 0, 100],
  ['sidebar-accent-foreground', 0.985, 0, 0, 100],
  ['sidebar-border', 1, 0, 0, 10], // alpha-carrying in v4
  ['sidebar-ring', 0.556, 0, 0, 100],
]

/* ---------- reporting ---------- */

const oklchLiteral = ([, L, C, H, a]) =>
  a === 100 ? `oklch(${L} ${C} ${H})` : `oklch(${L} ${C} ${H} / ${a}%)`

function convert(set, label) {
  const rows = []
  let worstClip = 0
  let worstRoundTrip = 0
  for (const tok of set) {
    const [, L, C, H, a] = tok
    const res = oklchTriplet(L, C, H, a / 100)
    worstClip = Math.max(worstClip, res.clip)

    // round-trip: encode the 8-bit result back to oklch and compare channels
    const back = linearSrgbToOklch(res.channels.map((c) => srgbDecode(c / 255)))
    const dL = Math.abs(back[0] - L)
    const dC = Math.abs(back[1] - C)
    worstRoundTrip = Math.max(worstRoundTrip, dL, dC)

    rows.push({ tok, res, back, label })
  }
  return { rows, worstClip, worstRoundTrip }
}

const light = convert(LIGHT, 'light')
const dark = convert(DARK, 'dark')

console.log('='.repeat(96))
console.log('OKLCH -> sRGB triplet conversion  (Chrome 109-safe)')
console.log('='.repeat(96))
for (const [name, data] of [[':root (light)', light], ['.dark', dark]]) {
  console.log(`\n--- ${name} ---`)
  console.log('  token                       oklch source                       -> triplet          hex       alpha')
  for (const r of data.rows) {
    const [n, L, C, H, a] = r.tok
    console.log(
      `  ${n.padEnd(26)} ${oklchLiteral(r.tok).padEnd(34)} -> ${triplet(r.res.channels).padEnd(16)} ${r.res.hex}  ${(a / 100).toString().padEnd(5)}${r.res.clip > 0.0005 ? `  [clipped ${r.res.clip.toFixed(3)}]` : ''}`
    )
  }
  console.log(`  worst sRGB gamut clip: ${data.worstClip.toFixed(5)}   worst round-trip oklch delta: ${data.worstRoundTrip.toFixed(5)}`)
}

// Key tokens the task asks to be sanity-checked.
console.log('\n--- key-token sanity check ---')
for (const key of ['primary', 'background', 'foreground', 'destructive', 'border']) {
  const l = light.rows.find((r) => r.tok[0] === key)
  const d = dark.rows.find((r) => r.tok[0] === key)
  console.log(
    `  ${key.padEnd(14)} light ${l.res.hex} (${triplet(l.res.channels)})   dark ${d.res.hex} (${triplet(d.res.channels)})`
  )
}

/* ---------- color-mix(in oklch, A, B N%) equivalents ----------
 * src/components/ui/button.tsx uses the arbitrary value
 *   hover:bg-[color-mix(in_oklch,var(--secondary),var(--foreground)_5%)]
 * which Tailwind emits verbatim. Chrome 109 cannot parse color-mix(), so the
 * declaration would be dropped entirely and the hover state would lose its
 * background. The mix is precomputed per theme into --secondary-hover-rgb and
 * substituted by a PostCSS step (see postcss.config.js).
 */
function oklchMix(a, b, wB) {
  const [L1, C1, H1] = a
  const [L2, C2, H2] = b
  const wA = 1 - wB
  const L = L1 * wA + L2 * wB
  const C = C1 * wA + C2 * wB
  let H
  if (C1 === 0) H = H2
  else if (C2 === 0) H = H1
  else {
    let d = H2 - H1
    if (d > 180) d -= 360
    if (d < -180) d += 360
    H = H1 + d * wB
    if (H < 0) H += 360
  }
  return [L, C, H]
}

console.log('\n--- color-mix(in oklch, var(--secondary), var(--foreground) 5%) ---')
for (const [label, sec, fg] of [
  ['light', [0.97, 0, 0], [0.145, 0, 0]],
  ['dark', [0.269, 0, 0], [0.985, 0, 0]],
]) {
  const mixed = oklchMix(sec, fg, 0.05)
  const res = oklchTriplet(...mixed)
  console.log(
    `  ${label.padEnd(6)} oklch(${mixed[0].toFixed(5)} ${mixed[1].toFixed(5)} ${mixed[2]}) -> ${triplet(res.channels)}  ${res.hex}`
  )
}

/* ---------- CSS emission ---------- */

const ALPHA_VARS = new Set(['border', 'input', 'sidebar-border'])

function emitBlock(set, indent = '    ') {
  const lines = []
  for (const tok of set) {
    const [name, L, C, H, a] = tok
    const channels = triplet(oklchTriplet(L, C, H).channels)
    lines.push(`${indent}--${name}-rgb: ${channels}; /* ${oklchLiteral(tok)} */`)
    if (ALPHA_VARS.has(name)) lines.push(`${indent}--${name}-a: ${a / 100};`)
    // Full-colour alias so existing direct var(--x) consumers keep working
    // (accent-[var(--primary)], shadow-[...var(--primary)], sonner CSS vars, ...).
    lines.push(
      ALPHA_VARS.has(name)
        ? `${indent}--${name}: rgb(var(--${name}-rgb) / var(--${name}-a));`
        : `${indent}--${name}: rgb(var(--${name}-rgb));`
    )
  }
  return lines.join('\n')
}

if (process.argv.includes('--emit')) {
  console.log('\n' + '='.repeat(96))
  console.log(':root {')
  console.log(emitBlock(LIGHT))
  console.log('}')
  console.log('\n.dark {')
  console.log(emitBlock(DARK))
  console.log('}')
}

export { LIGHT, DARK, emitBlock, triplet }
