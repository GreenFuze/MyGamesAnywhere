import { useId, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface TooltipProps {
  content: ReactNode
  children: ReactNode
  className?: string
  side?: 'top' | 'bottom'
}

/**
 * Lightweight clarification tooltip. Essential instructions must also remain
 * available inline or in an expanded details surface.
 */
export function Tooltip({ content, children, className, side = 'top' }: TooltipProps) {
  const id = useId()

  return (
    <span className={cn('group/tooltip relative inline-flex', className)} aria-describedby={id}>
      {children}
      <span
        id={id}
        role="tooltip"
        className={cn(
          'pointer-events-none absolute left-1/2 z-50 hidden w-max max-w-64 -translate-x-1/2 rounded-mga border border-mga-border bg-mga-elevated px-2.5 py-1.5 text-center text-xs font-normal leading-4 text-mga-text shadow-xl',
          'group-hover/tooltip:block group-focus-within/tooltip:block',
          side === 'top' ? 'bottom-full mb-2' : 'top-full mt-2',
        )}
      >
        {content}
      </span>
    </span>
  )
}
