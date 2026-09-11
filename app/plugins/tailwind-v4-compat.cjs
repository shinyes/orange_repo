/**
 * Tailwind v3.4 compatibility layer for Tailwind v4 authoring syntax.
 *
 * Why this exists: the components under src/ are authored against Tailwind v4
 * (shadcn v4 / Base UI style) and must not be edited. v3.4 has no equivalent for
 * several of the shorthands they use, so this plugin teaches v3.4 to compile them.
 *
 * Every registration below was derived empirically:
 *   - the *semantics* come from shadcn's own v4 variant definitions
 *     (node_modules/shadcn/dist/tailwind.css), which is the upstream source of
 *     these class names;
 *   - the *attribute names* come from scanning node_modules/@base-ui/react for
 *     `"data-*"` literals. Base UI emits BARE boolean attributes
 *     (`data-open`, `data-checked`, `data-orientation="horizontal"`), NOT
 *     Radix's `data-state="open"`. We still match the `[data-state=...]` form as
 *     well, because shadcn defines it and some dependencies (sonner) use Radix.
 *   - src/components/ui/tooltip.tsx uses `data-[state=delayed-open]`, which only
 *     Radix emits; Base UI uses `data-open`. That class is inert either way, in
 *     v4 as well as v3, so it is left alone.
 *
 * Anything NOT registered here is reported by `node scripts/class-coverage.mjs`,
 * so a newly introduced v4-only shorthand fails the check instead of silently
 * producing no CSS.
 */
const plugin = require('tailwindcss/plugin')

/**
 * Boolean-ish attributes. Matching `[data-x]:not([data-x="false"])` reproduces
 * v4's bare-attribute semantics exactly: present with any value (including the
 * empty string Base UI writes) counts as "on", except an explicit "false".
 */
const bareSel = (attr) => `:where([${attr}]:not([${attr}="false"]))`
const bare = (attr) => `&${bareSel(attr)}`

const WITH_DATA_STATE = {
  open: 'open',
  closed: 'closed',
  checked: 'checked',
  unchecked: 'unchecked',
  active: 'active',
}

/** Enum-valued attributes: `data-horizontal` is really `[data-orientation=...]`. */
const ORIENTATION = {
  horizontal: 'horizontal',
  vertical: 'vertical',
}

/**
 * Selector for "an ancestor carrying a group marker".
 *
 * Two traps here, both hit during this migration:
 *  1. addVariant() never receives the `/name` modifier, so the marker cannot be
 *     inserted from the variant name.
 *  2. A *named* group's class is literally `group/tabs` — NOT `.group`. A naive
 *     `.group[data-orientation=…]` selector therefore matches nothing at all,
 *     silently disabling every `group-data-horizontal/tabs:*` rule.
 *
 * Matching both `[class~="group"]` and `[class*="group/"]` covers the unnamed
 * group and any named one.
 */
const GROUP_ANY = (state = '') =>
  `:where([class~="group"]${state}, [class*="group/"]${state})`

/** Bare boolean attributes used by this codebase (inventory: scripts/v4-syntax-inventory.json). */
const BARE_ATTRS = [
  'open',
  'closed',
  'checked',
  'unchecked',
  'active',
  'selected',
  'disabled',
  'popup-open',
  'placeholder',
  'inset',
]

module.exports = plugin(({ addVariant, addUtilities, matchVariant }) => {
  /* ---------------- bare data-* variants ---------------- */

  for (const name of BARE_ATTRS) {
    const selectors = []
    // shadcn's companion selector, e.g. data-open also matches [data-state=open]
    if (WITH_DATA_STATE[name]) selectors.push(`&:where([data-state="${WITH_DATA_STATE[name]}"])`)
    // data-disabled is defined by shadcn as [data-disabled="true"] first
    if (name === 'disabled') selectors.push('&:where([data-disabled="true"])')
    selectors.push(bare(`data-${name}`))
    addVariant(`data-${name}`, selectors)
  }

  for (const name of Object.keys(ORIENTATION)) {
    addVariant(`data-${name}`, `&:where([data-orientation="${ORIENTATION[name]}"])`)
  }

  /* ---------------- group-data-<bare> variants ----------------
   * e.g. `group-data-horizontal/tabs:h-8` on TabsList. v3.4 supports
   * `group-data-[orientation=horizontal]/tabs:` natively but not the v4
   * shorthand spelling.
   */
  for (const name of Object.keys(ORIENTATION)) {
    addVariant(
      `group-data-${name}`,
      `${GROUP_ANY(`[data-orientation="${ORIENTATION[name]}"]`)} &`
    )
  }

  /* ---------------- aria-invalid ----------------
   * v3.4 hard-codes its aria-* variant values (checked/disabled/expanded/hidden/
   * pressed/readonly/required/selected) and has no `aria-invalid`; v4's aria-*
   * is dynamic. Verified against the v4 build: `[aria-invalid="true"]`.
   */
  addVariant('aria-invalid', '&[aria-invalid="true"]')

  /* ---------------- has-data-* variant ----------------
   * v4: has-data-[icon=inline-start]  ->  &:has([data-icon=inline-start])
   */
  const unwrap = (v) => String(v).replace(/^\[/, '').replace(/\]$/, '')
  matchVariant('has-data', (value) => `&:has([data-${unwrap(value)}])`)

  /* ---------------- in-data-* variant ----------------
   * v4: in-data-[slot=button-group]  ->  :where([data-slot=button-group]) &  (ancestor)
   */
  matchVariant('in-data', (value) => `:where([data-${unwrap(value)}]) &`)

  /* ---------------- group-has-data-* variant ----------------
   * v4: group-has-data-[slot=x]/y  ->  .group/y:has([data-slot=x]) &
   */
  matchVariant('group-has-data', (value, { modifier } = {}) => {
    const group = modifier ? `.group\\/${modifier}` : '.group'
    return `${group}:has([data-${unwrap(value)}]) &`
  })

  /* ---------------- group-has-<state> ----------------
   * e.g. group-has-disabled/field:opacity-50  (checkbox.tsx)
   */
  addVariant('group-has-disabled', `${GROUP_ANY('[data-disabled]')} &`)
  addVariant('group-has-checked', `${GROUP_ANY('[data-checked]')} &`)

  /* ---------------- not-* variants ---------------- */

  // not-data-checked  ->  &:not(:where([data-checked]:not([data-checked="false"])))
  for (const name of BARE_ATTRS) {
    addVariant(`not-data-${name}`, `&:not(${bareSel(`data-${name}`)})`)
  }
  // not-data-[variant=destructive]
  matchVariant('not-data', (value) => `&:not([data-${unwrap(value)}])`)
  // not-aria-[haspopup]  ->  &:not([aria-haspopup="true"])  (matches v3's aria-* mapping)
  matchVariant('not-aria', (value) => `&:not([aria-${unwrap(value)}="true"])`)
  // not-first / not-last (unused today, but part of the v4 not-* family)
  addVariant('not-first', '&:not(:first-child)')
  addVariant('not-last', '&:not(:last-child)')

  /* ---------------- ** (all descendants) ----------------
   * v4 `**:` == `& *`. v3 supports `*:` (direct children) natively; the
   * descendant form is what v4 added.
   */
  addVariant('**', '& *')

  /* ---------------- utilities missing from v3.4 ---------------- */

  // v4 semantics for the outline family. Keys passed to addUtilities() MUST carry
  // a leading dot — without it Tailwind emits a bare element selector
  // (`outline-none { … }`), which matches nothing.
  //
  // v3's own `outline-none` sets `outline: 2px solid transparent; outline-offset: 2px`,
  // which would shift the focus rings drawn by `focus-visible:outline-*`, and v3's
  // `outline-<n>` only sets outline-width (leaving outline-style at its `none`
  // initial value, so the ring would be invisible). v4 sets both. corePlugins
  // disables outlineStyle + outlineWidth so these are the only definitions.
  //
  // --tw-outline-style mirrors v4: `outline-none`/`outline-hidden` switch it off,
  // width utilities read it (falling back to `solid`) — so
  // `focus-visible:outline-1` is self-contained, exactly as in v4.
  addUtilities({
    '.outline': { 'outline-style': 'solid' },
    '.outline-none': { '--tw-outline-style': 'none', 'outline-style': 'none' },
    '.outline-hidden': { '--tw-outline-style': 'none', 'outline-style': 'none' },
  })
  addUtilities({
    '.outline-0': { 'outline-style': 'var(--tw-outline-style, solid)', 'outline-width': '0px' },
    '.outline-1': { 'outline-style': 'var(--tw-outline-style, solid)', 'outline-width': '1px' },
    '.outline-2': { 'outline-style': 'var(--tw-outline-style, solid)', 'outline-width': '2px' },
    '.outline-4': { 'outline-style': 'var(--tw-outline-style, solid)', 'outline-width': '4px' },
    '.outline-8': { 'outline-style': 'var(--tw-outline-style, solid)', 'outline-width': '8px' },
  })
  addUtilities({
    '@media (forced-colors: active)': {
      '.outline-hidden': { outline: '2px solid transparent', 'outline-offset': '2px' },
    },
  })

  // v4 `(--foo)` shorthand for a bare custom property, e.g. `max-h-(--available-height)`.
  // v3 has no equivalent syntax. addUtilities() cannot express these either: for a
  // key containing `(` Tailwind emits it verbatim *without* the leading dot,
  // producing the invalid selector `max-h-(--available-height) { … }`.
  // The three forms used in src/ are therefore emitted as raw CSS from
  // src/index.css — see "v4 (--var) 简写兼容". All three are used WITHOUT
  // variants, which is what makes the raw-CSS approach sufficient here; new
  // usages are caught by scripts/class-coverage.mjs.

  // v4 trailing `!` (important) modifier, e.g. `[&>svg]:size-3!`.
  // v3 only understands the leading form (`!size-3`), and addUtilities() rejects
  // `!` outright (postcss-selector-parser: "Unexpected '!'"). Those few class
  // names are therefore emitted as raw CSS from a `@layer utilities` block in
  // src/index.css instead — see "v4 trailing-! 兼容" there.
})
