// Audit which utilities in src/ are affected by v4 -> v3.4 *scale/semantic* renames.
// These are silent visual regressions: the class name still compiles, it just means
// something different. Each family below changed meaning between the two versions.
import fs from 'node:fs'
import path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')
const inv = JSON.parse(fs.readFileSync(path.join(ROOT, 'scripts', 'v4-syntax-inventory.json'), 'utf8'))
const tokens = Object.keys(inv.tokenFiles)

// strip variants: keep only the last colon-separated utility part
const utilOf = (t) => t.split(':').pop()

const FAMILIES = {
  // v4 `shadow-sm` == v3 `shadow`; v3 `shadow-sm` is smaller. v4 `shadow` == v3 `shadow-md`.
  shadow: /^(shadow|shadow-(sm|md|lg|xl|2xl|inner|none))$/,
  // v4 `rounded-sm` = 0.25rem, v3 `rounded-sm` = 0.125rem. v4 adds `rounded-xs`/`4xl`.
  rounded: /^rounded(-(sm|md|lg|xl|2xl|3xl|4xl|xs|none|full))?$/,
  // v4 `blur-sm` == v3 `blur`; v3 `blur-sm` = 4px vs v4 8px.
  blur: /^(blur|blur-(sm|md|lg|xl|2xl|3xl|none))$/,
  backdropBlur: /^backdrop-blur(-(sm|md|lg|xl|2xl|3xl|none))?$/,
  // v4 bare `ring` = 1px; v3 bare `ring` = 3px. v4 adds `ring-<n>` for any n.
  ring: /^ring(-([0-9]+|inset))?$/,
  ringOffset: /^ring-offset(-[0-9]+)?$/,
  // v4 `outline-hidden` replaces v3 `outline-none`; v4 `outline-none` = outline-style:none.
  outline: /^outline(-(none|hidden|dashed|dotted|double|[0-9]+|ring|[a-z-]+))?$/,
  // v4 graceful degradation names
  divideBorder: /^divide-(x|y)(-[0-9]+)?$/,
  underlineOffset: /^underline-offset(-(auto|[0-9]+))?$/,
  textDecorationThickness: /^decoration(-(auto|from-font|[0-9]+))?$/,
}

const report = {}
for (const [name, re] of Object.entries(FAMILIES)) {
  const hits = new Map()
  for (const t of tokens) {
    const u = utilOf(t)
    if (re.test(u)) {
      if (!hits.has(u)) hits.set(u, new Set())
      for (const f of inv.tokenFiles[t]) hits.get(u).add(f)
    }
  }
  report[name] = [...hits.entries()].sort()
}

console.log('='.repeat(90))
console.log('utility scale/semantic audit (v4 source -> v3.4 output)')
console.log('='.repeat(90))
for (const [name, list] of Object.entries(report)) {
  console.log(`\n### ${name}  (${list.length} distinct utilities)`)
  if (!list.length) console.log('    (none used)')
  for (const [u, files] of list) {
    const flare = [...files].filter((f) => !f.includes('components/ui'))
    console.log(`    ${u.padEnd(34)} files=${files.size}${flare.length ? `  non-ui=${flare.length}` : ''}`)
  }
}

// Also surface every distinct bare `ring` / `shadow` / `outline` usage site count.
console.log('\n### raw distinct utilities across whole src (for eyeballing)')
const allUtils = new Map()
for (const t of tokens) {
  const u = utilOf(t)
  allUtils.set(u, (allUtils.get(u) ?? 0) + 1)
}
console.log(`    ${allUtils.size} distinct utility names`)
