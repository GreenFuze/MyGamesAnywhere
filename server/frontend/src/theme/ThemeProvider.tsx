import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import {
  defaultThemeForColorScheme,
  isThemeId,
  THEME_PRESETS,
  type ThemeId,
} from '@/theme/presets'
import { getFrontendConfig, setFrontendConfig } from '@/api/client'

const STORAGE_KEY = 'mga.themeId'

function readLocalThemeId(): ThemeId | null {
  try {
    const local = localStorage.getItem(STORAGE_KEY)
    if (local && isThemeId(local)) return local
  } catch {
    /* private mode */
  }
  return null
}

function initialThemeId(): ThemeId {
  return readLocalThemeId() ?? defaultThemeForColorScheme()
}

type Ctx = {
  themeId: ThemeId
  setThemeId: (id: ThemeId) => void
  reducedMotion: boolean
}

const ThemeContext = createContext<Ctx | null>(null)

function applyThemeVars(id: ThemeId) {
  const root = document.documentElement
  root.dataset.theme = id
  const vars = THEME_PRESETS[id]
  for (const [key, value] of Object.entries(vars)) {
    root.style.setProperty(key, value)
  }
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [themeId, setThemeIdState] = useState<ThemeId>(initialThemeId)
  const [reducedMotion, setReducedMotion] = useState(false)

  useEffect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    const sync = () => setReducedMotion(mq.matches)
    sync()
    mq.addEventListener('change', sync)
    return () => mq.removeEventListener('change', sync)
  }, [])

  useEffect(() => {
    document.documentElement.classList.toggle('mga-reduced-motion', reducedMotion)
  }, [reducedMotion])

  // Prefer server theme when available (overrides first paint)
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const remote = await getFrontendConfig()
        const id = remote.themeId
        if (typeof id === 'string' && isThemeId(id) && !cancelled) {
          setThemeIdState(id)
          applyThemeVars(id)
          try {
            localStorage.setItem(STORAGE_KEY, id)
          } catch {
            /* ignore */
          }
        }
      } catch {
        /* keep local / system */
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    applyThemeVars(themeId)
  }, [themeId])

  const setThemeId = useCallback((id: ThemeId) => {
    setThemeIdState(id)
    localStorage.setItem(STORAGE_KEY, id)
    void (async () => {
      try {
        const prev = await getFrontendConfig()
        await setFrontendConfig({ ...prev, themeId: id })
      } catch {
        /* persist local only */
      }
    })()
  }, [])

  const value = useMemo(
    () => ({ themeId, setThemeId, reducedMotion }),
    [themeId, setThemeId, reducedMotion],
  )

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme() {
  const c = useContext(ThemeContext)
  if (!c) throw new Error('useTheme outside ThemeProvider')
  return c
}
