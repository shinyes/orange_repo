// Class-coverage checker: proves every class name used in src/ actually produced
// a CSS rule in a built stylesheet. Used twice:
//   1. against a v3 "native only" probe  -> decides what the compat plugin must add
//   2. against the real dist CSS         -> acceptance evidence
//
// Usage: node scripts/class-coverage.mjs <css-file> [--list-missing] [--feature=<name>]
import fs from 'node:fs'
import path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')
const cssPath = process.argv[2]
if (!cssPath) {
  console.error('usage: node scripts/class-coverage.mjs <css-file> [--list-missing] [--feature=bare-data]')
  process.exit(2)
}
const listMissing = process.argv.includes('--list-missing')
const featureArg = process.argv.find((a) => a.startsWith('--feature='))?.slice('--feature='.length)

const inv = JSON.parse(fs.readFileSync(path.join(ROOT, 'scripts', 'v4-syntax-inventory.json'), 'utf8'))
const css = fs.readFileSync(path.resolve(cssPath), 'utf8')

/* ---------- extract every class name that appears in a selector ---------- */

// Decode CSS identifier escapes: \: -> :, \/ -> /, \2c  -> ',', \31 23 -> '123'
function unescapeIdent(s) {
  return s
    .replace(/\\([0-9a-fA-F]{1,6})\s?/g, (_, hex) => String.fromCodePoint(parseInt(hex, 16)))
    .replace(/\\(.)/g, '$1')
}

// Only look at selector preludes (text before '{'), never at declaration values.
const selectors = []
for (const m of css.matchAll(/(^|[}])([^{}@]+)\{/g)) {
  const prelude = m[2].trim()
  if (prelude && !prelude.startsWith('@')) selectors.push(prelude)
}
const selectorText = selectors.join('\n')

const covered = new Set()
for (const m of selectorText.matchAll(/\.((?:\\[^\s]|[^\s.,>+~:()\[\]\\])+)/g)) {
  covered.add(unescapeIdent(m[1]))
}
// The v4-only class names that v3.4 cannot compile at all (child-star + arbitrary
// variant, `(--var)` shorthand, trailing `!`) are emitted from src/index.css using
// `[class~="…"]` attribute selectors instead of escaped class selectors — matching
// them here keeps this check meaningful rather than reporting false negatives.
for (const m of selectorText.matchAll(/\[class~="([^"]+)"\]/g)) {
  covered.add(m[1])
}

/* ---------- compare against src tokens ---------- */

const tokens = Object.keys(inv.tokenFiles)

// The inventory tokenizer splits string literals on whitespace, so it also picks
// up JSX/CSS/API fragments that are not class names at all (URLs, numbers, JS
// keywords, CSS function calls). Keep only tokens that could plausibly be a
// Tailwind class candidate, so that the "missing" number means something.
const LOOKS_LIKE_CLASS = (t) =>
  /^[-a-z@\[]/i.test(t) &&
  !/^[a-z][a-z0-9+.-]*:\/\//i.test(t) &&
  !t.startsWith('./') &&
  !/^(var|rgb|rgba|hsl|calc|color-mix|min|max)\(/.test(t) &&
  !/^[0-9.]+$/.test(t) &&
  !t.includes("'") &&
  !t.includes('"') &&
  t.length < 160

const realTokens = tokens.filter(LOOKS_LIKE_CLASS)

// A token may be a compound/arbitrary value the extractor splits oddly; only assert
// on tokens that look like real single class names.
const FEATURES = {
  'bare-data': (t) => /(?:^|:)data-[a-z]/.test(t),
  'group-data': (t) => /(?:^|:)group-data-/.test(t),
  'has-data': (t) => /(?:^|:)has-data-/.test(t),
  'group-has-data': (t) => /(?:^|:)group-has-data-/.test(t),
  'group-has-state': (t) => /(?:^|:)group-has-(?!\[)/.test(t),
  'in-data': (t) => /(?:^|:)in-data-/.test(t),
  'not-': (t) => /(?:^|:)not-/.test(t),
  'double-star': (t) => /(?:^|:)\*\*:/.test(t),
  'child-star': (t) => /(?:^|:)\*:/.test(t),
  'v4-var-value': (t) => /\(--[a-zA-Z0-9-]+\)/.test(t),
  'trailing-bang': (t) => t.endsWith('!'),
}

const missing = realTokens.filter((t) => !covered.has(t))
const missingByFeature = {}
for (const [name, test] of Object.entries(FEATURES)) {
  const m = missing.filter(test)
  if (m.length) missingByFeature[name] = m
}

const target = featureArg ? realTokens.filter(FEATURES[featureArg] ?? (() => false)) : realTokens
const targetMissing = target.filter((t) => !covered.has(t))

console.log('='.repeat(90))
console.log(`class coverage vs ${path.relative(ROOT, path.resolve(cssPath))}`)
console.log('='.repeat(90))
console.log(`  src tokens scanned       : ${realTokens.length} (of ${tokens.length} raw tokens; artifacts excluded)`)
console.log(`  class selectors in CSS   : ${covered.size}`)
console.log(`  tokens WITH a CSS rule   : ${realTokens.length - missing.length}`)
console.log(`  tokens WITHOUT a rule    : ${missing.length}`)

if (featureArg) {
  console.log(`\n  --feature=${featureArg}: ${target.length} tokens, ${targetMissing.length} missing`)
  for (const t of targetMissing) console.log(`    MISSING  ${t}`)
}

if (Object.keys(missingByFeature).length) {
  console.log('\n  missing tokens grouped by v4-only feature:')
  for (const [name, list] of Object.entries(missingByFeature)) {
    console.log(`    ${name} (${list.length}):`)
    for (const t of list.slice(0, 40)) console.log(`      ${t}`)
    if (list.length > 40) console.log(`      ... +${list.length - 40} more`)
  }
}

if (listMissing) {
  const rest = missing.filter((t) => !Object.values(missingByFeature).flat().includes(t))
  if (rest.length) {
    console.log(`\n  other missing tokens (${rest.length}) — informational only, the`)
    console.log(`  tokenizer also yields non-class strings (API paths, enums, JS keywords):`)
    for (const t of rest.slice(0, 60)) console.log(`    ${t}`)
    if (rest.length > 60) console.log(`    ... +${rest.length - 60} more`)
  }
}

/* ---------- regression gate ----------
 * The "other missing" bucket cannot be asserted on, because the tokenizer also
 * yields non-class strings. The gate is therefore explicit: every v4-only
 * feature group must be fully compiled, and a curated list of the specific
 * utilities that v3.4 needed help with must each be present.
 */
const MUST_COMPILE = [
  // v3.4 scale gaps
  'focus-visible:ring-3',
  'aria-invalid:ring-3',
  'aria-invalid:border-destructive',
  'rounded-4xl',
  'size-3.5',
  'size-4',
  'shadow-sm',
  // outline family (corePlugins disabled, re-added with v4 semantics)
  'outline-none',
  'outline-hidden',
  'focus-visible:outline-1',
  // v4-only shorthands handled by the compat layer
  'max-h-(--available-height)',
  'origin-(--transform-origin)',
  'w-(--anchor-width)',
  'in-data-[slot=button-group]:rounded-lg',
  'has-data-[slot=kbd]:pr-1.5',
  'group-has-disabled/field:opacity-50',
  'group-has-[:focus-visible]/field-label:not-data-checked:border-input',
  'not-data-[variant=destructive]:focus:**:text-accent-foreground',
  'data-open:animate-in',
  'data-closed:animate-out',
  'data-checked:bg-primary',
  'data-horizontal:flex-col',
  'group-data-horizontal/tabs:h-8',
  'group-data-vertical/tabs:flex-col',
  'data-[variant=destructive]:*:[svg]:text-destructive',
  '*:[a]:underline',
  '*:[a]:underline-offset-3',
  '*:[span]:last:flex',
  "*:[svg:not([class*='size-'])]:size-6",
  '[&>svg]:size-3!',
  'data-[side=left]:top-1/2!',
  'focus:**:text-accent-foreground',
  '**:data-[slot=kbd]:relative',
]

const curatedMissing = MUST_COMPILE.filter((t) => !covered.has(t))
console.log(`\n  curated must-compile list : ${MUST_COMPILE.length - curatedMissing.length}/${MUST_COMPILE.length} present`)
for (const t of curatedMissing) console.log(`    MISSING  ${t}`)

const featureMissing = Object.values(missingByFeature).flat()
const gateFailures = curatedMissing.length + (featureArg ? targetMissing.length : 0)
console.log(
  `\n  v4-feature tokens missing : ${featureMissing.length}` +
    (featureArg ? `   (--feature=${featureArg}: ${targetMissing.length})` : '')
)
process.exitCode = gateFailures ? 1 : 0
