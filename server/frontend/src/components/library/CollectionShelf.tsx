import { useEffect, useRef, useState } from 'react'
import { ChevronDown, ChevronLeft, ChevronRight } from 'lucide-react'
import type { CollectionSectionConfig, GameDetailResponse } from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { GameCard } from '@/components/library/GameCard'
import { GameGrid } from '@/components/library/GameGrid'
import { filterGamesBySection } from '@/lib/collectionSections'

interface CollectionShelfProps {
  sections: CollectionSectionConfig[]
  expandedSectionId: string | null
  onExpandedSectionChange: (id: string | null) => void
  onRemoveSection: (id: string) => void
  games: GameDetailResponse[]
  isLoading: boolean
}

function PagedShelfRow({ games, label }: { games: GameDetailResponse[]; label: string }) {
  const viewportRef = useRef<HTMLDivElement>(null)
  const [canScrollLeft, setCanScrollLeft] = useState(false)
  const [canScrollRight, setCanScrollRight] = useState(false)

  const updateScrollState = () => {
    const el = viewportRef.current
    if (!el) return
    setCanScrollLeft(el.scrollLeft > 4)
    setCanScrollRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 4)
  }

  useEffect(() => {
    updateScrollState()
    const el = viewportRef.current
    if (!el) return
    el.addEventListener('scroll', updateScrollState, { passive: true })
    window.addEventListener('resize', updateScrollState)
    return () => {
      el.removeEventListener('scroll', updateScrollState)
      window.removeEventListener('resize', updateScrollState)
    }
  }, [games.length])

  const page = (dir: 1 | -1) => {
    const el = viewportRef.current
    if (!el) return
    el.scrollBy({ left: dir * Math.max(240, el.clientWidth - 96), behavior: 'smooth' })
  }

  return (
    <div className="group/shelf relative">
      <div
        ref={viewportRef}
        className="mga-hidden-scrollbar flex snap-x snap-mandatory gap-4 overflow-x-auto scroll-smooth pr-12"
      >
        {games.map((game) => (
          <div key={game.id} className="w-[clamp(10rem,18vw,13.5rem)] shrink-0 snap-start">
            <GameCard game={game} />
          </div>
        ))}
      </div>
      {canScrollLeft && (
        <button
          type="button"
          onClick={() => page(-1)}
          className="absolute left-0 top-1/2 hidden h-12 w-10 -translate-y-1/2 items-center justify-center rounded-mga border border-mga-border bg-mga-bg/90 text-mga-text shadow-lg backdrop-blur transition-colors hover:border-mga-accent sm:flex"
          aria-label={`Previous page in ${label}`}
        >
          <ChevronLeft size={22} />
        </button>
      )}
      {canScrollRight && (
        <button
          type="button"
          onClick={() => page(1)}
          className="absolute right-0 top-1/2 flex h-12 w-10 -translate-y-1/2 items-center justify-center rounded-mga border border-mga-border bg-mga-bg/90 text-mga-text shadow-lg backdrop-blur transition-colors hover:border-mga-accent"
          aria-label={`Next page in ${label}`}
        >
          <ChevronRight size={22} />
        </button>
      )}
    </div>
  )
}

export function CollectionShelf({
  sections,
  expandedSectionId,
  onExpandedSectionChange,
  onRemoveSection,
  games,
  isLoading,
}: CollectionShelfProps) {
  if (isLoading) {
    return (
      <div className="space-y-8">
        {Array.from({ length: 3 }, (_, index) => (
          <div key={index} className="space-y-3">
            <div className="h-7 w-48 rounded-mga bg-mga-surface" />
            <div className="grid grid-cols-[repeat(auto-fill,minmax(190px,1fr))] gap-4">
              {Array.from({ length: 5 }, (_, cardIndex) => (
                <div
                  key={cardIndex}
                  className="aspect-[3/4] rounded-mga border border-mga-border bg-mga-surface"
                />
              ))}
            </div>
          </div>
        ))}
      </div>
    )
  }

  const visibleSections = sections
    .map((section) => ({
      section,
      games: filterGamesBySection(games, section),
    }))
    .filter((entry) => entry.games.length > 0)

  if (visibleSections.length === 0) {
    return (
      <div className="rounded-mga border border-mga-border bg-mga-surface p-6 text-sm text-mga-muted">
        No saved sections match the current filters.
      </div>
    )
  }

  return (
    <div className="space-y-8">
      {visibleSections.map(({ section, games: sectionGames }, sectionIndex) => {
        const expanded = expandedSectionId === section.id

        return (
          <section
            key={section.id}
            className="mga-stagger-item space-y-3"
            style={{ animationDelay: `${Math.min(sectionIndex, 8) * 55}ms` }}
          >
            <div className="flex flex-wrap items-center justify-between gap-3">
              <button
                type="button"
                onClick={() => onExpandedSectionChange(expanded ? null : section.id)}
                className="flex min-w-0 items-center gap-2 text-left"
              >
                {expanded ? (
                  <ChevronDown size={18} className="shrink-0 text-mga-muted" />
                ) : (
                  <ChevronRight size={18} className="shrink-0 text-mga-muted" />
                )}
                <h2 className="truncate text-2xl font-semibold tracking-tight text-mga-text">
                  {section.label}
                </h2>
                <Badge variant="accent">{sectionGames.length}</Badge>
              </button>

              <div className="flex items-center gap-2">
                {expanded && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => onExpandedSectionChange(null)}
                  >
                    Collapse
                  </Button>
                )}
                <Button variant="ghost" size="sm" onClick={() => onRemoveSection(section.id)}>
                  Remove
                </Button>
              </div>
            </div>

            {expanded ? (
              <GameGrid games={sectionGames} isLoading={false} />
            ) : (
              <PagedShelfRow games={sectionGames} label={section.label} />
            )}
          </section>
        )
      })}
    </div>
  )
}
