import { ChevronDown, Info, Play } from 'lucide-react'
import { useEffect, useRef, useState, type MouseEvent, type ReactNode } from 'react'
import type { GameCardPrimaryAction } from '@/lib/gameCardActions'
import { cn } from '@/lib/utils'

interface GameSplitActionButtonProps {
  gameTitle: string
  primaryAction: GameCardPrimaryAction
  alternateActions: GameCardPrimaryAction[]
  onSelect: (action: GameCardPrimaryAction, event: MouseEvent<HTMLButtonElement>) => void
  appearance?: 'overlay' | 'surface'
}

function actionIcon(action: GameCardPrimaryAction): ReactNode {
  return action.kind === 'play' ? <Play size={16} fill="currentColor" /> : <Info size={16} />
}

export function GameSplitActionButton({
  gameTitle,
  primaryAction,
  alternateActions,
  onSelect,
  appearance = 'overlay',
}: GameSplitActionButtonProps) {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement | null>(null)
  const surface = appearance === 'surface'

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

  const mainClass = surface
    ? 'border border-mga-accent/45 bg-mga-accent text-white hover:bg-mga-accent/90'
    : 'bg-white text-black hover:bg-white/90'
  const arrowClass = surface
    ? 'border border-l-0 border-mga-accent/45 bg-mga-accent text-white hover:bg-mga-accent/90'
    : 'bg-white text-black hover:bg-white/90'

  return (
    <div
      ref={rootRef}
      className="pointer-events-auto relative flex min-w-[9.75rem] max-w-full flex-1 sm:max-w-[24rem] sm:flex-none"
    >
      <button
        type="button"
        disabled={primaryAction.disabled}
        onClick={(event) => onSelect(primaryAction, event)}
        title={primaryAction.title ?? primaryAction.label}
        aria-label={`${primaryAction.label} ${gameTitle}`}
        className={cn(
          'inline-flex h-10 min-w-0 flex-1 items-center justify-center gap-2 px-4 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-55',
          alternateActions.length > 0 ? 'rounded-l-full border-r border-black/14 pr-3' : 'rounded-full',
          mainClass,
        )}
      >
        {actionIcon(primaryAction)}
        <span className="truncate">{primaryAction.label}</span>
      </button>
      {alternateActions.length > 0 ? (
        <>
          <button
            type="button"
            onClick={(event) => {
              event.preventDefault()
              event.stopPropagation()
              setOpen((value) => !value)
            }}
            className={cn(
              'inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-r-full transition-colors',
              arrowClass,
            )}
            aria-label={`Choose how to play ${gameTitle}`}
            aria-haspopup="menu"
            aria-expanded={open}
            title="Other play options"
          >
            <ChevronDown size={17} aria-hidden="true" />
          </button>
          {open ? (
            <div
              role="menu"
              aria-label={`Ways to play ${gameTitle}`}
              className={cn(
                'absolute bottom-full right-0 z-20 mb-2 w-max min-w-full max-w-[min(32rem,calc(100vw-2rem))] overflow-hidden rounded-[14px] border p-1.5 shadow-2xl',
                surface
                  ? 'border-mga-border bg-mga-elevated text-mga-text'
                  : 'border-white/12 bg-[#17191f] text-white',
              )}
            >
              {alternateActions.map((action, index) => (
                <button
                  key={action.id ?? `${action.route ?? action.kind ?? 'action'}-${action.label}-${index}`}
                  type="button"
                  role="menuitem"
                  disabled={action.disabled}
                  title={action.title ?? action.label}
                  onClick={(event) => {
                    if (action.disabled) return
                    setOpen(false)
                    onSelect(action, event)
                  }}
                  className="flex w-full items-center gap-2 rounded-[10px] px-3 py-2 text-left text-sm transition-colors hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-45"
                >
                  {actionIcon(action)}
                  <span className="min-w-0 truncate">{action.label}</span>
                </button>
              ))}
            </div>
          ) : null}
        </>
      ) : null}
    </div>
  )
}
