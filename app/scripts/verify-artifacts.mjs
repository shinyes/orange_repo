// Acceptance assertions for the Tailwind v4 -> v3.4 migration.
//
// Target: Chrome 109, the last Chrome available on Windows 7. It supports neither
// oklch()/lab()/lch() nor color-mix(), so the built CSS must contain none of them
// (except in third-party vendored stylesheets, which are reported separately).
//
// Usage: node scripts/verify-artifacts.mjs
import fs from 'node:fs'
import path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')
const assets = path.join(ROOT, 'dist', 'assets')
const cssFiles = fs.readdirSync(assets).filter((f) => f.endsWith('.css'))
if (cssFiles.length !== 1) {
  console.error(`expected exactly 1 CSS asset in dist/assets, found ${cssFiles.length}`)
  process.exit(2)
}

const cssPath = path.join(assets, cssFiles[0])
const css = fs.readFileSync(cssPath, 'utf8')

const count = (re) => (css.match(re) ?? []).length

let failures = 0
const check = (label, actual, ok, expectation) => {
  if (!ok) failures++
  console.log(`  [${ok ? 'PASS' : 'FAIL'}] ${label.padEnd(46)} ${String(actual).padStart(5)}   (${expectation})`)
}

console.log('='.repeat(92))
console.log(`artifact verification — dist/assets/${cssFiles[0]}  (${(css.length / 1024).toFixed(1)} KiB)`);
console.log('='.repeat(92))

/* ---------- 1. Chrome 109 incompatible colour functions ---------- */

console.log('\n-- 1. colour functions unsupported by Chrome 109 --')
check('oklch(', count(/oklch\(/g), count(/oklch\(/g) === 0, 'must be 0')
check('lab(', count(/lab\(/g), count(/lab\(/g) === 0, 'must be 0')
check('lch(', count(/lch\(/g), count(/lch\(/g) === 0, 'must be 0')

const colorMixCount = count(/color-mix\(/g)
const colorMixThirdParty = count(/color-mix\(in srgb/g)
const colorMixOurs = colorMixCount - colorMixThirdParty
console.log(`\n-- 1b. color-mix() (${colorMixCount} total) --`)
console.log(`  authored by this project (color-mix(in oklch…)) : ${colorMixOurs}`)
console.log(`  third-party vendored (color-mix(in srgb…))      : ${colorMixThirdParty}`)
for (const m of css.matchAll(/color-mix\([^)]*\)/g)) {
  const around = css.slice(Math.max(0, m.index - 120), m.index)
  const origin = /vscode|monaco|quick-input|keybinding/.test(around) ? 'monaco-editor (vendored)' : 'PROJECT'
  console.log(`    ${origin.padEnd(26)} ${m[0]}`)
}
check('project-authored color-mix()', colorMixOurs, colorMixOurs === 0, 'must be 0')

/* ---------- 2. the sRGB triplet pipeline ---------- */

console.log('\n-- 2. sRGB variable pipeline --')
check('rgb(var(', count(/rgb\(var\(/g), count(/rgb\(var\(/g) > 100, 'expect >100')
const tripletDecls = [...css.matchAll(/--([a-z0-9-]+)-rgb:\s*(\d+)\s+(\d+)\s+(\d+)/g)]
console.log(`  --*-rgb triplet declarations : ${tripletDecls.length}`)
const badTriplet = tripletDecls.filter((m) =>
  [+m[2], +m[3], +m[4]].some((v) => v < 0 || v > 255)
)
check('triplet channels within 0..255', badTriplet.length, badTriplet.length === 0, 'must be 0')

/* ---------- 3. opacity utilities ---------- */

console.log('\n-- 3. alpha utilities emitted as rgb(var(--x-rgb) / a) --')
const alphaSamples = []
for (const m of css.matchAll(/\.[^{},;]*\\\/[0-9]+[^{},;]*\{[^}]*\}/g)) {
  if (/rgb\(var\(--[a-z0-9-]+-rgb\)\s*\/\s*(0?\.\d+|calc\()/.test(m[0])) alphaSamples.push(m[0])
}
for (const s of alphaSamples.slice(0, 6)) {
  console.log('    ' + s.replace(/\s+/g, ' ').slice(0, 130))
}
check('opacity utilities found', alphaSamples.length, alphaSamples.length > 0, 'expect >0')
const badAlpha = alphaSamples.filter((s) => /oklch|color-mix|lab\(/i.test(s))
check('opacity utilities using v4 colour funcs', badAlpha.length, badAlpha.length === 0, 'must be 0')

/* ---------- 4. dark mode is class-based (v4 `&:is(.dark *)`) ---------- */

console.log('\n-- 4. dark mode strategy --')
const darkClassRules = count(/:is\(\.dark \*\)|\.dark /g)
const darkMediaRules = count(/@media \(prefers-color-scheme:\s*dark\)/g)
check('class-based dark selectors', darkClassRules, darkClassRules > 0, 'expect >0')
check('prefers-color-scheme dark blocks', darkMediaRules, darkMediaRules === 0, 'must be 0')

/* ---------- 5. no leftover v4-only at-rules / nesting ---------- */

console.log('\n-- 5. v4-only CSS features --')
for (const [label, re] of [
  ['@theme', /@theme/g],
  ['@custom-variant', /@custom-variant/g],
  ['@utility', /@utility/g],
  ['@apply (must be resolved)', /@apply/g],
  ['@tailwind (must be resolved)', /@tailwind/g],
  ['@container', /@container/g],
]) {
  check(label, count(re), count(re) === 0, 'must be 0')
}

/* ---------- 6. custom utilities / global CSS preserved ---------- */

console.log('\n-- 6. hand-written CSS preserved --')
for (const sel of [
  '.markdown-body',
  '.md-clean',
  '.md-line',
  'html.low-effects',
  'prefers-reduced-transparency',
  '-webkit-tap-highlight-color',
]) {
  const present = css.includes(sel)
  check(sel, present ? 'yes' : 'no', present, 'must be present')
}

/* ---------- 7. animation utilities (tw-animate-css -> tailwindcss-animate) ---------- */

console.log('\n-- 7. animate utilities --')
for (const key of ['@keyframes enter', '@keyframes exit']) {
  const present = css.includes(key)
  check(key, present ? 'yes' : 'no', present, 'must be present')
}

/* ---------- summary ---------- */

console.log('\n' + '='.repeat(92))
console.log(failures === 0 ? 'ALL CHECKS PASSED' : `${failures} CHECK(S) FAILED`)
console.log('='.repeat(92))
process.exitCode = failures ? 1 : 0
