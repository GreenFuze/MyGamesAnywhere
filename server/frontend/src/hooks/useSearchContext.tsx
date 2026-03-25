import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'

// ---------------------------------------------------------------------------
// Context shape
// ---------------------------------------------------------------------------

type SearchCtx = {
  searchQuery: string
  setSearchQuery: (q: string) => void
  searchRef: React.RefObject<HTMLInputElement>
}

const SearchContext = createContext<SearchCtx | null>(null)

// ---------------------------------------------------------------------------
// Provider — owns search state, ref, and Ctrl+K shortcut
// ---------------------------------------------------------------------------

export function SearchProvider({ children }: { children: ReactNode }) {
  const [searchQuery, setSearchQueryState] = useState('')
  const searchRef = useRef<HTMLInputElement>(null!)

  const setSearchQuery = useCallback((q: string) => {
    setSearchQueryState(q)
  }, [])

  // Global Ctrl+K → focus the search input
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault()
        searchRef.current?.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <SearchContext.Provider value={{ searchQuery, setSearchQuery, searchRef }}>
      {children}
    </SearchContext.Provider>
  )
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useSearch(): SearchCtx {
  const ctx = useContext(SearchContext)
  if (!ctx) throw new Error('useSearch must be used inside <SearchProvider>')
  return ctx
}
