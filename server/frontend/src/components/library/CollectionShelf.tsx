import { ChevronRight } from 'lucide-react'
import type { CollectionSectionConfig, GameDetailResponse } from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { SectionPreviewShelf } from '@/components/library/SectionPreviewShelf'
import { filterGamesBySection } from '@/lib/collectionSections'

interface CollectionShelfProps {
  sections: CollectionSectionConfig[]
  onOpenSection: (id: string) => void
  onRemoveSection: (id: string) => void
  games: GameDetailResponse[]
  isLoading: boolean
}

export function CollectionShelf({
  sections,
  onOpenSection,
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
        return (
          <section
            key={section.id}
            className="mga-stagger-item space-y-3"
            style={{ animationDelay: `${Math.min(sectionIndex, 8) * 55}ms` }}
          >
            <div className="flex flex-wrap items-center justify-between gap-3">
              <button
                type="button"
                onClick={() => onOpenSection(section.id)}
                className="flex min-w-0 items-center gap-2 text-left"
              >
                <ChevronRight size={18} className="shrink-0 text-mga-muted" />
                <h2 className="truncate text-2xl font-semibold tracking-tight text-mga-text">
                  {section.label}
                </h2>
                <Badge variant="accent">{sectionGames.length}</Badge>
              </button>

              <div className="flex items-center gap-2">
                <Button variant="ghost" size="sm" onClick={() => onRemoveSection(section.id)}>
                  Remove
                </Button>
              </div>
            </div>

            <SectionPreviewShelf
              games={sectionGames}
              label={section.label}
              onOpenShelf={() => onOpenSection(section.id)}
            />
          </section>
        )
      })}
    </div>
  )
}
