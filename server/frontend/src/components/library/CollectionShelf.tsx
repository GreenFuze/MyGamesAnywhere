import { ChevronDown, ChevronRight, Ellipsis } from 'lucide-react'
import type { CollectionSectionConfig, GameDetailResponse } from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { GameCard } from '@/components/library/GameCard'
import { GameGrid } from '@/components/library/GameGrid'
import { filterGamesBySection } from '@/lib/collectionSections'

const PREVIEW_SLOT_LIMIT = 5

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
        const hasMore = sectionGames.length > PREVIEW_SLOT_LIMIT
        const previewGames = sectionGames.slice(0, hasMore ? PREVIEW_SLOT_LIMIT - 1 : PREVIEW_SLOT_LIMIT)
        const hiddenCount = sectionGames.length - previewGames.length

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
              <div className="relative flex items-stretch gap-4 overflow-hidden">
                {previewGames.map((game) => (
                  <div key={game.id} className="w-[190px] min-w-0 shrink-0">
                    <GameCard game={game} />
                  </div>
                ))}
                {hasMore && (
                  <button
                    type="button"
                    onClick={() => onExpandedSectionChange(section.id)}
                    className="group relative flex w-16 shrink-0 items-center justify-center overflow-hidden rounded-[1.1rem] border border-dashed border-mga-border bg-gradient-to-b from-mga-surface via-mga-bg to-mga-surface text-mga-muted transition-colors hover:border-mga-accent/60 hover:text-mga-text"
                    aria-label={`Show more games in ${section.label}`}
                  >
                    <div className="absolute inset-0 bg-gradient-to-l from-mga-elevated/40 to-transparent opacity-70 transition-opacity group-hover:opacity-100" />
                    <div className="relative flex flex-col items-center justify-center gap-2 py-4">
                      <Ellipsis size={18} />
                      <span className="text-[10px] font-semibold uppercase tracking-[0.18em]">
                        +{hiddenCount}
                      </span>
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
