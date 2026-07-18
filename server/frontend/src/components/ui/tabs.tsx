import { cn } from '@/lib/utils'
import type { ReactNode } from 'react'

export interface Tab {
  id: string
  label: string
  icon?: ReactNode
}

interface TabsProps {
  tabs: Tab[]
  active: string
  onChange: (id: string) => void
  className?: string
}

/** Responsive tab navigation that wraps instead of exposing browser scrollbars. */
export function Tabs({ tabs, active, onChange, className }: TabsProps) {
  return (
    <div
      className={cn('flex flex-wrap gap-1 rounded-mga border border-mga-border bg-mga-surface/60 p-1', className)}
      role="tablist"
    >
      {tabs.map((tab) => (
        <button
          key={tab.id}
          type="button"
          role="tab"
          aria-selected={active === tab.id}
          onClick={() => onChange(tab.id)}
          className={cn(
            'flex min-h-10 flex-1 items-center justify-center gap-1.5 rounded-md px-3 py-2 text-sm font-medium transition-colors sm:flex-none',
            active === tab.id
              ? 'bg-mga-accent/15 text-mga-accent shadow-sm'
              : 'text-mga-muted hover:bg-mga-elevated hover:text-mga-text',
          )}
        >
          {tab.icon}
          {tab.label}
        </button>
      ))}
    </div>
  )
}
