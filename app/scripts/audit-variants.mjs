// Inventory of variant families whose v3.4 behaviour differs from v4:
//   - `aria-*`: v3.4 hard-codes a fixed list (checked/disabled/expanded/hidden/
//     pressed/readonly/required/selected) and has NO `aria-invalid`; v4 is dynamic.
//   - bare `group-data-<value>` / `group-has-<state>` with a `/name` marker:
//     v3.4's addVariant() silently drops the modifier, and named groups live on
//     `group/name` (not `.group`), so a naive `.group[…]` selector never matches.
import fs from 'node:fs'
import path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')
const walk = (d) =>
  fs.readdirSync(d, { withFileTypes: true }).flatMap((e) =>
    e.isDirectory() ? walk(path.join(d, e.name)) : /\.tsx?$/.test(e.name) ? [path.join(d, e.name)] : []
  )

const aria = new Set()
const group = new Set()
const peer = new Set()

for (const f of walk(path.join(ROOT, 'src'))) {
  const src = fs.readFileSync(f, 'utf8')
  for (const lit of src.match(/'[^'\n]*'|"[^"\n]*"/g) ?? []) {
    for (const t of lit.slice(1, -1).split(/\s+/)) {
      if (/(?:^|:)aria-[a-z-]+/.test(t)) aria.add(t)
      if (/(?:^|:)group-(?:has|data)-[a-z]/.test(t) || /(?:^|:)peer-(?:has|data)-[a-z]/.test(t)) {
        ;/(?:^|:)peer-/.test(t) ? peer.add(t) : group.add(t)
      }
    }
  }
}

// v3.4's built-in aria variant values
const V3_ARIA = new Set([
  'checked', 'disabled', 'expanded', 'hidden', 'pressed', 'readonly', 'required', 'selected',
])

const ariaVariants = new Set()
for (const t of aria) for (const m of t.matchAll(/aria-(\[?[a-z-]+)/g)) ariaVariants.add(m[1])

console.log('=== aria-* variant names used ===')
for (const a of [...ariaVariants].sort()) {
  const bare = a.replace(/^\[/, '')
  const native = V3_ARIA.has(bare)
  console.log(`  aria-${a.padEnd(22)} ${native ? 'native in v3' : 'NOT native -> must be registered'}`)
}

console.log('\n=== bare group-data-<x> / group-has-<x> (modifier-dropping risk) ===')
for (const g of [...group].sort()) console.log('  ' + g)
if (!group.size) console.log('  (none)')

console.log('\n=== peer variants ===')
for (const p of [...peer].sort()) console.log('  ' + p)
if (!peer.size) console.log('  (none)')
