/**
 * Tailwind v3.4 configuration — migrated from Tailwind v4's CSS-first `@theme`.
 *
 * Colour tokens live in src/index.css as sRGB channel triplets
 * (`--primary-rgb: 219 76 12`) because Chrome 109 (the last Chrome available on
 * Windows 7) supports neither oklch()/lab()/lch() nor color-mix(). Tailwind
 * injects the alpha channel, so `bg-primary/50` compiles to
 * `rgb(var(--primary-rgb) / 0.5)` — plain, Chrome-109-parsable sRGB.
 *
 * src/index.css additionally keeps `--primary: rgb(var(--primary-rgb))` so that
 * the direct `var(--primary)` references used by components
 * (`accent-[var(--primary)]`, `shadow-[0_2px_0_0_var(--primary)]`, the sonner
 * CSS variables) and by index.css itself keep resolving to a real colour.
 */
import tailwindcssAnimate from 'tailwindcss-animate'
import v4Compat from './plugins/tailwind-v4-compat.cjs'

/** Standard token: triplet + Tailwind-controlled alpha. */
const token = (name) => `rgb(var(--${name}-rgb) / <alpha-value>)`

/**
 * Token that carries its own alpha in dark mode
 * (v4: `--border: oklch(1 0 0 / 10%)`). The token alpha is multiplied with the
 * utility alpha, so `border-border` keeps its 10% translucency in dark mode and
 * `border-border/60` still scales it.
 */
const alphaToken = (name) =>
  `rgb(var(--${name}-rgb) / calc(<alpha-value> * var(--${name}-a, 1)))`

/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],

  // Replaces v4's `@custom-variant dark (&:is(.dark *));` — same semantics:
  // matches an element that has a `.dark` ancestor.
  darkMode: 'class',

  corePlugins: {
    // Replaced by explicit v4-semantics definitions in plugins/tailwind-v4-compat.cjs.
    // v3's own `outline-none` emits `outline: 2px solid transparent; outline-offset: 2px`,
    // which would shift the focus rings drawn by `focus-visible:outline-*`; and v3's
    // `outline-<n>` sets only outline-width, leaving the outline invisible because
    // outline-style stays at its `none` initial value. v4 handles both.
    outlineStyle: false,
    outlineWidth: false,
  },

  theme: {
    extend: {
      colors: {
        background: token('background'),
        foreground: token('foreground'),

        card: { DEFAULT: token('card'), foreground: token('card-foreground') },
        popover: { DEFAULT: token('popover'), foreground: token('popover-foreground') },
        primary: { DEFAULT: token('primary'), foreground: token('primary-foreground') },
        secondary: { DEFAULT: token('secondary'), foreground: token('secondary-foreground') },
        muted: { DEFAULT: token('muted'), foreground: token('muted-foreground') },
        accent: { DEFAULT: token('accent'), foreground: token('accent-foreground') },

        destructive: token('destructive'),

        border: alphaToken('border'),
        input: alphaToken('input'),
        ring: token('ring'),

        chart: {
          1: token('chart-1'),
          2: token('chart-2'),
          3: token('chart-3'),
          4: token('chart-4'),
          5: token('chart-5'),
        },

        sidebar: {
          DEFAULT: token('sidebar'),
          foreground: token('sidebar-foreground'),
          primary: {
            DEFAULT: token('sidebar-primary'),
            foreground: token('sidebar-primary-foreground'),
          },
          accent: {
            DEFAULT: token('sidebar-accent'),
            foreground: token('sidebar-accent-foreground'),
          },
          border: alphaToken('sidebar-border'),
          ring: token('sidebar-ring'),
        },
      },

      // v4 `@theme` radius scale, keyed off --radius.
      borderRadius: {
        sm: 'calc(var(--radius) * 0.6)',
        md: 'calc(var(--radius) * 0.8)',
        lg: 'var(--radius)',
        xl: 'calc(var(--radius) * 1.4)',
        '2xl': 'calc(var(--radius) * 1.8)',
        '3xl': 'calc(var(--radius) * 2.2)',
        '4xl': 'calc(var(--radius) * 2.6)',
      },

      fontFamily: {
        sans: ['Geist Variable', 'sans-serif'],
        heading: ['Geist Variable', 'sans-serif'],
      },

      // v4 renamed this step: v4 `shadow-sm` == v3 `shadow`. Without this override
      // v3's smaller `shadow-sm` would silently render a weaker shadow.
      boxShadow: {
        sm: '0 1px 3px 0 rgb(0 0 0 / 0.1), 0 1px 2px -1px rgb(0 0 0 / 0.1)',
      },

      // v3 only ships ring widths 0,1,2,4,8; the codebase uses `ring-3`
      // (13 occurrences across 7 files).
      ringWidth: { 3: '3px' },

      // v3 only ships underline offsets 0,1,2,4,8; the codebase uses `underline-offset-3`.
      textUnderlineOffset: { 3: '3px' },
    },
  },

  plugins: [tailwindcssAnimate, v4Compat],
}
