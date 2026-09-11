/**
 * PostCSS pipeline for the Tailwind v3.4 build.
 *
 * Order matters: Tailwind first, autoprefixer second, then the color-mix compat
 * step, which must see Tailwind's final output.
 */
import tailwindcss from 'tailwindcss'
import autoprefixer from 'autoprefixer'

/**
 * Chrome 109 (the last Chrome available on Windows 7) does not support
 * color-mix(), so such a declaration is dropped wholesale and the affected
 * state silently loses its styling.
 *
 * src/components/ui/button.tsx contains exactly one authored color-mix:
 *   hover:bg-[color-mix(in_oklch,var(--secondary),var(--foreground)_5%)]
 * Tailwind emits arbitrary values verbatim, so the mix is precomputed into
 * --secondary-hover-rgb (lerped in oklch space — see scripts/oklch-to-srgb.mjs)
 * and substituted here. The token is defined per theme, so light and dark both
 * resolve correctly.
 *
 * Deliberately narrow: only this single expression is rewritten. Any other
 * color-mix left in the bundle is third-party (e.g. monaco-editor's own
 * stylesheet) and is reported rather than silently altered.
 */
const MIX_RE =
  /color-mix\(\s*in\s+oklch\s*,\s*var\(\s*--secondary\s*\)\s*,\s*var\(\s*--foreground\s*\)\s+5%\s*\)/g

const replaceOklchColorMix = () => ({
  postcssPlugin: 'orangeoj-replace-oklch-color-mix',
  Declaration(decl) {
    if (!decl.value || !decl.value.includes('color-mix(')) return
    MIX_RE.lastIndex = 0
    if (!MIX_RE.test(decl.value)) return
    MIX_RE.lastIndex = 0
    decl.value = decl.value.replace(MIX_RE, 'rgb(var(--secondary-hover-rgb))')
  },
})

export default {
  plugins: [tailwindcss(), autoprefixer(), replaceOklchColorMix()],
}
