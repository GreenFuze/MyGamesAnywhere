/** CSS custom properties applied on `document.documentElement`. */

export const THEME_IDS = [
  'midnight',
  'daylight',
  'deep_blue',
  'obsidian',
  'curator',
  'big_screen',
  'retro_terminal',
  'synthwave',
  'cinema',
  'frost',
  'neon_arcade',
] as const

export type ThemeId = (typeof THEME_IDS)[number]

export const THEME_LABELS: Record<ThemeId, string> = {
  midnight: 'Midnight',
  daylight: 'Daylight',
  deep_blue: 'Deep Blue',
  obsidian: 'Obsidian',
  curator: 'Curator',
  big_screen: 'Big Screen',
  retro_terminal: 'Retro Terminal',
  synthwave: 'Synthwave',
  cinema: 'Cinema',
  frost: 'Frost',
  neon_arcade: 'Neon Arcade',
}

type Vars = Record<string, string>

const sans =
  "ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"
const mono = "ui-monospace, 'Cascadia Code', 'Consolas', monospace"

function v(partial: Vars): Vars {
  return {
    '--mga-radius': '0.5rem',
    '--mga-font-sans': sans,
    '--mga-font-mono': mono,
    ...partial,
  }
}

export const THEME_PRESETS: Record<ThemeId, Vars> = {
  /* Storm + electric blue — matches brand art (lightning, D-pad emblem); calmer than full neon */
  midnight: v({
    '--mga-bg': '#0c0a10',
    '--mga-surface': '#13101a',
    '--mga-elevated': '#1a1624',
    '--mga-border': '#2c2640',
    '--mga-text': '#eef0f6',
    '--mga-muted': '#8f8aa3',
    '--mga-accent': '#3db8ff',
    '--mga-accent-muted': '#1e6faf',
  }),
  daylight: v({
    '--mga-bg': '#f6f7f9',
    '--mga-surface': '#ffffff',
    '--mga-elevated': '#ffffff',
    '--mga-border': '#e1e4ea',
    '--mga-text': '#1a1d24',
    '--mga-muted': '#5c6370',
    '--mga-accent': '#2563eb',
    '--mga-accent-muted': '#93b4f4',
  }),
  deep_blue: v({
    '--mga-bg': '#0e1520',
    '--mga-surface': '#152030',
    '--mga-elevated': '#1a2d45',
    '--mga-border': '#274060',
    '--mga-text': '#e4edf7',
    '--mga-muted': '#7a8fa8',
    '--mga-accent': '#2dd4bf',
    '--mga-accent-muted': '#0d9488',
  }),
  obsidian: v({
    '--mga-bg': '#000000',
    '--mga-surface': '#0a0c0a',
    '--mga-elevated': '#121612',
    '--mga-border': '#1f2a1f',
    '--mga-text': '#e6ffe8',
    '--mga-muted': '#6b8f6e',
    '--mga-accent': '#39ff14',
    '--mga-accent-muted': '#1a6b0f',
  }),
  curator: v({
    '--mga-bg': '#14121c',
    '--mga-surface': '#1c1828',
    '--mga-elevated': '#242030',
    '--mga-border': '#3d3552',
    '--mga-text': '#f2eef8',
    '--mga-muted': '#9a8fb8',
    '--mga-accent': '#d4a853',
    '--mga-accent-muted': '#8a6f2a',
  }),
  big_screen: v({
    '--mga-bg': '#070b14',
    '--mga-surface': '#0c1426',
    '--mga-elevated': '#111c38',
    '--mga-border': '#1e3a6e',
    '--mga-text': '#f0f6ff',
    '--mga-muted': '#6b8cce',
    '--mga-accent': '#38bdf8',
    '--mga-accent-muted': '#0369a1',
  }),
  retro_terminal: v({
    '--mga-bg': '#0a0f0a',
    '--mga-surface': '#0d120d',
    '--mga-elevated': '#111811',
    '--mga-border': '#1f331f',
    '--mga-text': '#9fe6a8',
    '--mga-muted': '#4d7a52',
    '--mga-accent': '#c8ff7a',
    '--mga-accent-muted': '#5f8f2e',
    '--mga-font-sans': mono,
  }),
  synthwave: v({
    '--mga-bg': '#1a0b2e',
    '--mga-surface': '#2d1b4e',
    '--mga-elevated': '#3d2468',
    '--mga-border': '#ff6ad5',
    '--mga-text': '#fdebff',
    '--mga-muted': '#c084fc',
    '--mga-accent': '#ff2a6d',
    '--mga-accent-muted': '#06b6d4',
  }),
  cinema: v({
    '--mga-bg': '#000000',
    '--mga-surface': '#0a0a0a',
    '--mga-elevated': '#141414',
    '--mga-border': '#2a2419',
    '--mga-text': '#faf7ef',
    '--mga-muted': '#a89a78',
    '--mga-accent': '#d4af37',
    '--mga-accent-muted': '#6b5a2a',
  }),
  frost: v({
    '--mga-bg': '#2e3440',
    '--mga-surface': '#3b4252',
    '--mga-elevated': '#434c5e',
    '--mga-border': '#4c566a',
    '--mga-text': '#eceff4',
    '--mga-muted': '#aeb3bb',
    '--mga-accent': '#88c0d0',
    '--mga-accent-muted': '#5e81ac',
  }),
  neon_arcade: v({
    '--mga-bg': '#120024',
    '--mga-surface': '#1c0138',
    '--mga-elevated': '#28024d',
    '--mga-border': '#ff00aa',
    '--mga-text': '#fff23a',
    '--mga-muted': '#ff6ec7',
    '--mga-accent': '#00fff0',
    '--mga-accent-muted': '#bf00ff',
  }),
}

export function isThemeId(s: string): s is ThemeId {
  return (THEME_IDS as readonly string[]).includes(s)
}

export function defaultThemeForColorScheme(): ThemeId {
  if (typeof window === 'undefined') return 'midnight'
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'daylight' : 'midnight'
}
