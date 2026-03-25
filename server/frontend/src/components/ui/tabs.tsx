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

/** Horizontal tab navigation bar using mga CSS vars. */
export function Tabs({ tabs, active, onChange, className }: TabsProps) {
  return (
    <div
      className={cn('flex gap-1 border-b border-mga-border', className)}
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
            'flex items-center gap-1.5 px-4 py-2.5 text-sm font-medium transition-colors',
            'border-b-2 -mb-px',
            active === tab.id
              ? 'border-mga-accent text-mga-accent'
              : 'border-transparent text-mga-muted hover:text-mga-text hover:border-mga-border',
          )}
        >
          {tab.icon}
          {tab.label}
        </button>
      ))}
    </div>
  )
}
