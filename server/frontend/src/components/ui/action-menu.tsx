import { useEffect, useRef, useState, type ReactNode } from 'react'
import { MoreHorizontal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Tooltip } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

export interface ActionMenuItem {
  label: string
  onSelect: () => void
  icon?: ReactNode
  disabled?: boolean
  danger?: boolean
}

interface ActionMenuProps {
  items: ActionMenuItem[]
  label?: string
  className?: string
}

/** Compact secondary-action menu with keyboard and outside-click dismissal. */
export function ActionMenu({ items, label = 'More actions', className }: ActionMenuProps) {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!open) return
    const closeOnPointer = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false)
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    document.addEventListener('pointerdown', closeOnPointer)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('pointerdown', closeOnPointer)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [open])

  if (items.length === 0) return null

  return (
    <div ref={rootRef} className={cn('relative', className)}>
      <Tooltip content={label}>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={label}
          aria-haspopup="menu"
          aria-expanded={open}
          onClick={() => setOpen((value) => !value)}
          className="h-8 w-8"
        >
          <MoreHorizontal className="h-4 w-4" />
        </Button>
      </Tooltip>
      {open ? (
        <div
          role="menu"
          className="absolute right-0 top-full z-40 mt-1 min-w-44 overflow-hidden rounded-mga border border-mga-border bg-mga-elevated p-1 shadow-2xl"
        >
          {items.map((item) => (
            <button
              key={item.label}
              type="button"
              role="menuitem"
              disabled={item.disabled}
              onClick={() => {
                if (item.disabled) return
                setOpen(false)
                item.onSelect()
              }}
              className={cn(
                'flex w-full items-center gap-2 rounded px-3 py-2 text-left text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-45',
                item.danger
                  ? 'text-red-300 hover:bg-red-500/10'
                  : 'text-mga-text hover:bg-mga-surface',
              )}
            >
              {item.icon}
              <span>{item.label}</span>
            </button>
          ))}
        </div>
      ) : null}
    </div>
  )
}
