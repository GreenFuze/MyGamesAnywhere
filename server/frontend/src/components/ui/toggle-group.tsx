import { cn } from '@/lib/utils'
import type { ReactNode } from 'react'

interface ToggleOption<T extends string> {
  value: T
  label: ReactNode
}

interface ToggleGroupProps<T extends string> {
  value: T
  onChange: (value: T) => void
  options: ToggleOption<T>[]
  className?: string
}

export function ToggleGroup<T extends string>({
  value,
  onChange,
  options,
  className,
}: ToggleGroupProps<T>) {
  return (
    <div
      className={cn(
        'inline-flex items-center rounded-mga border border-mga-border bg-mga-bg',
        className,
      )}
      role="radiogroup"
    >
      {options.map((opt) => (
        <button
          key={opt.value}
          type="button"
          role="radio"
          aria-checked={value === opt.value}
          onClick={() => onChange(opt.value)}
          className={cn(
            'px-3 py-1.5 text-xs font-medium transition-colors first:rounded-l-mga last:rounded-r-mga',
            value === opt.value
              ? 'bg-mga-elevated text-mga-accent'
              : 'text-mga-muted hover:text-mga-text hover:bg-mga-elevated/50',
          )}
        >
          {opt.label}
        </button>
      ))}
    </div>
  )
}
