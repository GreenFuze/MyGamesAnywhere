import type { Config } from 'tailwindcss'

export default {
  darkMode: ['class'],
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        mga: {
          bg: 'var(--mga-bg)',
          surface: 'var(--mga-surface)',
          elevated: 'var(--mga-elevated)',
          border: 'var(--mga-border)',
          text: 'var(--mga-text)',
          muted: 'var(--mga-muted)',
          accent: 'var(--mga-accent)',
          'accent-muted': 'var(--mga-accent-muted)',
        },
      },
      borderRadius: {
        mga: 'var(--mga-radius)',
      },
      fontFamily: {
        mga: 'var(--mga-font-sans)',
        mono: 'var(--mga-font-mono)',
      },
      keyframes: {
        indeterminate: {
          '0%': { transform: 'translateX(-100%)' },
          '100%': { transform: 'translateX(400%)' },
        },
      },
      animation: {
        indeterminate: 'indeterminate 1.5s ease-in-out infinite',
      },
    },
  },
  plugins: [],
} satisfies Config
