// Enumerate every `*:` (v4 child-star) usage together with whatever variant
// follows, because v3.4 mishandles `*:` composed with arbitrary variants:
//   *:[a]:underline                  -> not generated at all
//   *:[svg:not([class*='size-'])]:x  -> generated with a corrupt body
//   *:data-[slot=x]:y                -> correct
import fs from 'node:fs'
import path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')
const walk = (d) =>
  fs.readdirSync(d, { withFileTypes: true }).flatMap((e) =>
    e.isDirectory() ? walk(path.join(d, e.name)) : /\.tsx?$/.test(e.name) ? [path.join(d, e.name)] : []
  )

const tokens = new Set()
for (const f of walk(path.join(ROOT, 'src'))) {
  const src = fs.readFileSync(f, 'utf8')
  for (const lit of src.match(/'[^'\n]*'|"[^"\n]*"/g) ?? []) {
    for (const t of lit.slice(1, -1).split(/\s+/)) {
      if (t.includes('*:')) tokens.add(t)
    }
  }
}

const star = [...tokens].filter((t) => t.includes('*:')).sort()
console.log(`=== all tokens containing '*:' (${star.length}) ===`)
for (const t of star) console.log('  ' + t)
