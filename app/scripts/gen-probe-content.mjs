// Writes .probe/probe-content.txt containing every unique class token found in src/,
// so a Tailwind build can be checked for which of them it can actually compile.
import fs from 'node:fs'
import path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')
const inv = JSON.parse(fs.readFileSync(path.join(ROOT, 'scripts', 'v4-syntax-inventory.json'), 'utf8'))

const toks = Object.keys(inv.tokenFiles).filter(
  (t) => !t.includes("'") && !t.includes('"') && t.length < 160
)

const outPath = path.join(ROOT, '.probe', 'probe-content.txt')
fs.writeFileSync(outPath, toks.join('\n') + '\n')
console.log(`wrote ${toks.length} tokens to ${path.relative(ROOT, outPath)}`)
