import type { CollectionSectionConfig, GameDetailResponse } from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { GameCard } from '@/components/library/GameCard'
import { GameGrid } from '@/components/library/GameGrid'
import { filterGamesBySection } from '@/lib/collectionSections'

const PREVIEW_CARD_LIMIT = 6

interface CollectionAccordionProps {
  sections: CollectionSectionConfig[]
  expandedSectionId: string | null
  onExpandedSectionChange: (id: string | null) => void
  onRemoveSection: (id: string) => void
  games: GameDetailResponse[]
  isLoading: boolean
}

export function CollectionAccordion({
  sections,
  expandedSectionId,
  onExpandedSectionChange,
  onRemoveSection,
  games,
  isLoading,
}: CollectionAccordionProps) {
  if (isLoading) {
    return (
      <div className="space-y-4">
        {Array.from({ length: 3 }, (_, index) => (
          <div
            key={index}
            className="rounded-mga border border-mga-border bg-mga-surface p-4 text-sm text-mga-muted"
          >
            Loading section...
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
    <div className="space-y-4">
      {visibleSections.map(({ section, games: sectionGames }) => {
        const expanded = expandedSectionId === section.id
        const previewGames = sectionGames.slice(0, PREVIEW_CARD_LIMIT)
        const hasMore = sectionGames.length > PREVIEW_CARD_LIMIT

        return (
          <section
            key={section.id}
            className="overflow-hidden rounded-mga border border-mga-border bg-mga-surface"
          >
            <button
              type="button"
              onClick={() => onExpandedSectionChange(expanded ? null : section.id)}
              className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-mga-elevated/40"
            >
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <h2 className="truncate text-lg font-semibold text-mga-text">{section.label}</h2>
                  <Badge variant="accent">{sectionGames.length}</Badge>
                </div>
              </div>
              <span className="text-xs uppercase tracking-wide text-mga-muted">
                {expanded ? 'Collapse' : 'Expand'}
              </span>
            </button>

            <div className="border-t border-mga-border p-4">
              <div className="mb-4 flex justify-end">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onRemoveSection(section.id)}
                >
                  Remove
                </Button>
              </div>

              {expanded ? (
                <GameGrid games={sectionGames} isLoading={false} />
              ) : (
                <div className="flex gap-4 overflow-x-auto pb-2">
                  {previewGames.map((game) => (
                    <div key={game.id} className="w-[170px] shrink-0">
                      <GameCard game={game} />
                    </div>
                  ))}
                  {hasMore && (
                    <button
                      type="button"
                      onClick={() => onExpandedSectionChange(section.id)}
                      className="flex w-[170px] shrink-0 items-center justify-center rounded-mga border border-dashed border-mga-border bg-mga-bg px-4 py-6 text-sm font-medium text-mga-muted transition-colors hover:border-mga-accent hover:text-mga-accent"
                    >
                      Show More
                    </button>
                  )}
                </div>
              )}
            </div>
          </section>
        )
      })}
    </div>
  )
}
