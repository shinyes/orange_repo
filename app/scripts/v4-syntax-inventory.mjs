// Inventory of Tailwind v4-only syntax used in app/src.
// Tokenizes JS/TS string literals (not raw grep) so class names containing
// quotes / '>' / '(' stay intact, then buckets them by v4-only feature.
import fs from 'node:fs'
import path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')
const SRC = path.join(ROOT, 'src')

function walk(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) walk(p, out)
    else if (/\.(tsx?|jsx?)$/.test(e.name)) out.push(p)
  }
  return out
}

// Pull the *contents* of every string literal so embedded quotes are preserved.
function stringLiterals(src) {
  const out = []
  const re = /'(?:[^'\\\n]|\\.)*'|"(?:[^"\\\n]|\\.)*"|`(?:[^`\\]|\\.)*`/g
  let m
  while ((m = re.exec(src))) out.push(m[0].slice(1, -1))
  return out
}

const files = walk(SRC)
const tokens = new Map() // token -> Set(files)

for (const f of files) {
  const src = fs.readFileSync(f, 'utf8')
  for (const lit of stringLiterals(src)) {
    for (const raw of lit.split(/\s+/)) {
      const t = raw.trim()
      if (!t || t.length > 200) continue
      if (!/^[-a-zA-Z0-9_:[\]()/.%*&>~=+$#'",!]+$/.test(t)) continue
      if (!tokens.has(t)) tokens.set(t, new Set())
      tokens.get(t).add(path.relative(ROOT, f))
    }
  }
}

const all = [...tokens.keys()]

const BUCKETS = {
  'bare data-* variant': (t) => /(?:^|:)data-[a-z]/.test(t),
  'bare group-data-* variant': (t) => /(?:^|:)group-data-[a-z]/.test(t),
  'bare peer-data-* variant': (t) => /(?:^|:)peer-data-[a-z]/.test(t),
  'has-data-* variant': (t) => /(?:^|:)has-data-/.test(t),
  'group-has-data-* variant': (t) => /(?:^|:)group-has-data-/.test(t),
  'group-has-<state> variant': (t) => /(?:^|:)group-has-(?!\[)/.test(t),
  'in-* variant': (t) => /(?:^|:)in-/.test(t),
  'not-* variant': (t) => /(?:^|:)not-/.test(t),
  '**: variant': (t) => /(?:^|:)\*\*:/.test(t),
  'v4 (--var) value': (t) => /\(--[a-zA-Z0-9-]+\)/.test(t),
  'trailing ! important': (t) => t.endsWith('!'),
  'outline-hidden': (t) => /(?:^|:)outline-hidden$/.test(t),
  'ring-3': (t) => /(?:^|:)ring-3$/.test(t),
  'rounded-4xl': (t) => /(?:^|:)rounded-4xl$/.test(t),
}

const report = {}
for (const [name, test] of Object.entries(BUCKETS)) {
  report[name] = all.filter(test).sort()
}

// shadow-* / ring bare / blur-* semantic-shift inventory (v3 vs v4 scale renames)
const scaleShift = all
  .filter((t) => /(?:^|:)(shadow|ring|blur|rounded|outline)$|(?:^|:)shadow-(sm|2xs|xs)$|(?:^|:)rounded-(xs|sm)$|(?:^|:)blur-(sm|xs)$/.test(t))
  .sort()

const out = []
for (const [name, list] of Object.entries(report)) {
  out.push(`\n### ${name} (${list.length} unique)`)
  for (const t of list) out.push(`  ${t}   [${[...tokens.get(t)].length} file(s)]`)
}
out.push(`\n### scale-shift candidates (${scaleShift.length} unique)`)
for (const t of scaleShift) out.push(`  ${t}   [${[...tokens.get(t)].length} file(s)]`)

console.log(`scanned ${files.length} files, ${all.length} unique class tokens`)
console.log(out.join('\n'))

// Machine-readable dump for the compat-plugin coverage assertion.
const DUMP = path.join(ROOT, 'scripts', 'v4-syntax-inventory.json')
fs.writeFileSync(
  DUMP,
  JSON.stringify(
    { files: files.length, tokens: all.length, buckets: report, scaleShift, tokenFiles: Object.fromEntries([...tokens].map(([k, v]) => [k, [...v]])) },
    null,
    2
  )
)
console.log(`\nwrote ${path.relative(ROOT, DUMP)}`)
