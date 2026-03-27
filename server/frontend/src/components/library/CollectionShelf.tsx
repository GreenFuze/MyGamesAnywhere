import { ChevronDown, ChevronRight, Ellipsis } from 'lucide-react'
import type { CollectionSectionConfig, GameDetailResponse } from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { GameCard } from '@/components/library/GameCard'
import { GameGrid } from '@/components/library/GameGrid'
import { filterGamesBySection } from '@/lib/collectionSections'

const PREVIEW_CARD_LIMIT = 5

interface CollectionShelfProps {
  sections: CollectionSectionConfig[]
  expandedSectionId: string | null
  onExpandedSectionChange: (id: string | null) => void
  onRemoveSection: (id: string) => void
  games: GameDetailResponse[]
  isLoading: boolean
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
      {visibleSections.map(({ section, games: sectionGames }) => {
        const expanded = expandedSectionId === section.id
        const previewGames = sectionGames.slice(0, PREVIEW_CARD_LIMIT)
        const hasMore = sectionGames.length > PREVIEW_CARD_LIMIT

        return (
          <section key={section.id} className="space-y-3">
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
              <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
                {previewGames.map((game) => (
                  <div key={game.id} className="min-w-0">
                    <GameCard game={game} />
                  </div>
                ))}
                {hasMore && (
                  <button
                    type="button"
                    onClick={() => onExpandedSectionChange(section.id)}
                    className="flex min-h-[18rem] items-center justify-center rounded-mga border border-dashed border-mga-border bg-mga-surface/70 transition-colors hover:border-mga-accent hover:bg-mga-elevated/40"
                    aria-label={`Show more games in ${section.label}`}
                  >
                    <div className="flex flex-col items-center gap-2 text-center">
                      <Ellipsis size={28} className="text-mga-muted" />
                      <p className="text-xs font-medium uppercase tracking-[0.3em] text-mga-muted">
                        More
                      </p>
                    </div>
                  </button>
                )}
              </div>
            )}
          </section>
        )
      })}
    </div>
  )
}
