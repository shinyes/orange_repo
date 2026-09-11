// Dump CSS rules for exact class names from a built stylesheet, with at-rule context.
// Used to read the *real* Tailwind v4 output (= ground truth) for utilities whose
// meaning changed in v3.4, and to dump the :root/.dark variable blocks.
//
// Usage: node scripts/dump-css-rules.mjs <css> <class1,class2,...>
//        node scripts/dump-css-rules.mjs <css> --vars
import fs from 'node:fs'
import path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')
const cssPath = path.resolve(process.argv[2])
const css = fs.readFileSync(cssPath, 'utf8')

function unescapeIdent(s) {
  return s
    .replace(/\\([0-9a-fA-F]{1,6})\s?/g, (_, hex) => String.fromCodePoint(parseInt(hex, 16)))
    .replace(/\\(.)/g, '$1')
}

/* ---------- tokenise the stylesheet into leaf rules with context ---------- */

const rules = []
let i = 0
const ctx = []
while (i < css.length) {
  const ch = css[i]
  if (ch === '@') {
    // at-rule prelude up to '{' or ';'
    let j = i
    while (j < css.length && css[j] !== '{' && css[j] !== ';') j++
    const prelude = css.slice(i, j).trim()
    if (css[j] === ';') {
      i = j + 1
    } else {
      ctx.push(prelude)
      i = j + 1
    }
  } else if (ch === '}') {
    if (ctx.length) ctx.pop()
    i++
  } else if (/\s/.test(ch)) {
    i++
  } else {
    // selector up to '{'
    let j = i
    while (j < css.length && css[j] !== '{' && css[j] !== '}') j++
    if (css[j] !== '{') {
      i = j
      continue
    }
    const selector = css.slice(i, j).trim()
    // balanced body
    let depth = 1
    let k = j + 1
    while (k < css.length && depth > 0) {
      if (css[k] === '{') depth++
      else if (css[k] === '}') depth--
      if (depth === 0) break
      k++
    }
    const body = css.slice(j + 1, k)
    rules.push({ selector, body, context: [...ctx] })
    i = k + 1
  }
}

const vars = process.argv.includes('--vars')

if (vars) {
  console.log('='.repeat(92))
  console.log(`variable blocks in ${path.relative(ROOT, cssPath)}`)
  console.log('='.repeat(92))
  for (const r of rules) {
    if (/^(:root|\.dark|:root,?\s*\.dark)/.test(r.selector) && r.body.includes('--')) {
      console.log(`\n/* ${r.selector} ${r.context.length ? `[in ${r.context.join(' > ')}]` : ''} */`)
      for (const decl of r.body.split(';')) {
        const d = decl.trim()
        if (d) console.log(`  ${d};`)
      }
    }
  }
  process.exit(0)
}

const terms = (process.argv[3] ?? '').split(',').map((s) => s.trim()).filter(Boolean)

console.log('='.repeat(92))
console.log(`rules for ${terms.join(', ')}`)
console.log('='.repeat(92))
for (const term of terms) {
  const hits = rules.filter((r) => {
    const classes = [...r.selector.matchAll(/\.((?:\\[^\s]|[^\s.,>+~:()\[\]\\])+)/g)].map((m) =>
      unescapeIdent(m[1])
    )
    return classes.includes(term)
  })
  console.log(`\n--- .${term} ---  (${hits.length} rule(s))`)
  if (!hits.length) {
    console.log('    NOT PRESENT in this stylesheet')
    continue
  }
  for (const h of hits) {
    const c = h.context.length ? `   /* in ${h.context.join(' > ')} */` : ''
    console.log(`  ${h.selector} {${c}`)
    for (const decl of h.body.split(';')) {
      const d = decl.trim()
      if (d) console.log(`      ${d};`)
    }
    console.log('  }')
  }
}
