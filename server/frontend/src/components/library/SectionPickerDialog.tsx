import { useEffect, useMemo, useState } from 'react'
import type {
  CollectionSectionConfig,
  CollectionSectionField,
  GameDetailResponse,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import {
  createAllGamesSection,
  getSectionOptions,
  makeGroupSection,
} from '@/lib/collectionSections'
import { cn } from '@/lib/utils'

const FIELD_OPTIONS: Array<{ value: CollectionSectionField | 'all'; label: string }> = [
  { value: 'all', label: 'All Games' },
  { value: 'platform', label: 'Platform' },
  { value: 'genre', label: 'Genre' },
  { value: 'developer', label: 'Developer' },
  { value: 'publisher', label: 'Publisher' },
  { value: 'source', label: 'Source' },
  { value: 'year', label: 'Year' },
]

interface SectionPickerDialogProps {
  open: boolean
  onClose: () => void
  games: GameDetailResponse[]
  existingSections: CollectionSectionConfig[]
  onAddSections: (sections: CollectionSectionConfig[]) => void
}

export function SectionPickerDialog({
  open,
  onClose,
  games,
  existingSections,
  onAddSections,
}: SectionPickerDialogProps) {
  const [selectedField, setSelectedField] = useState<CollectionSectionField | 'all' | null>(null)
  const [selectedValues, setSelectedValues] = useState<string[]>([])

  useEffect(() => {
    if (!open) return
    setSelectedField(null)
    setSelectedValues([])
  }, [open])

  const existingIds = useMemo(() => new Set(existingSections.map((section) => section.id)), [existingSections])
  const fieldOptions = useMemo(
    () => (selectedField && selectedField !== 'all' ? getSectionOptions(games, selectedField) : []),
    [games, selectedField],
  )

  const toggleValue = (value: string) => {
    setSelectedValues((current) =>
      current.includes(value)
        ? current.filter((entry) => entry !== value)
        : [...current, value],
    )
  }

  const handleConfirm = () => {
    if (!selectedField) return

    if (selectedField === 'all') {
      onAddSections([createAllGamesSection()])
      onClose()
      return
    }

    const sections = selectedValues
      .map((value) => makeGroupSection(selectedField, value))
      .filter((section): section is CollectionSectionConfig => !!section)

    if (sections.length === 0) return
    onAddSections(sections)
    onClose()
  }

  const canConfirm = selectedField === 'all' || selectedValues.length > 0

  return (
    <Dialog open={open} onClose={onClose} title="Add Section" className="max-w-2xl">
      <div className="space-y-6">
        <div className="space-y-2">
          <p className="text-sm font-medium text-mga-text">1. Choose a grouping</p>
          <div className="flex flex-wrap gap-2">
            {FIELD_OPTIONS.map((option) => (
              <button
                key={option.value}
                type="button"
                onClick={() => {
                  setSelectedField(option.value)
                  setSelectedValues([])
                }}
                className={cn(
                  'rounded-mga border px-3 py-2 text-sm transition-colors',
                  selectedField === option.value
                    ? 'border-mga-accent bg-mga-accent/20 text-mga-accent'
                    : 'border-mga-border bg-mga-bg text-mga-muted hover:text-mga-text',
                )}
              >
                {option.label}
              </button>
            ))}
          </div>
        </div>

        {selectedField && selectedField !== 'all' && (
          <div className="space-y-2">
            <p className="text-sm font-medium text-mga-text">2. Choose values</p>
            <div className="max-h-80 space-y-2 overflow-y-auto rounded-mga border border-mga-border bg-mga-bg p-3">
              {fieldOptions.map((option) => {
                const candidate = makeGroupSection(selectedField, option.value)
                const alreadyAdded = candidate ? existingIds.has(candidate.id) : false
                const checked = selectedValues.includes(option.value)
                return (
                  <label
                    key={option.value}
                    className={cn(
                      'flex items-center justify-between rounded-mga border px-3 py-2 text-sm',
                      alreadyAdded
                        ? 'border-mga-border/60 bg-mga-surface text-mga-muted'
                        : 'border-mga-border bg-mga-surface text-mga-text',
                    )}
                  >
                    <span className="flex items-center gap-3">
                      <input
                        type="checkbox"
                        checked={checked}
                        disabled={alreadyAdded}
                        onChange={() => toggleValue(option.value)}
                      />
                      <span>{option.label}</span>
                    </span>
                    <span className="text-xs text-mga-muted">
                      {alreadyAdded ? 'Added' : option.count}
                    </span>
                  </label>
                )
              })}
            </div>
          </div>
        )}

        <div className="flex justify-end gap-3">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={handleConfirm} disabled={!canConfirm}>
            Add Section
          </Button>
        </div>
      </div>
    </Dialog>
  )
}
