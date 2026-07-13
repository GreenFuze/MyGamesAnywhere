import { cn } from '@/lib/utils'
import { forwardRef, type InputHTMLAttributes, type ReactNode } from 'react'

export type InputProps = InputHTMLAttributes<HTMLInputElement> & {
  label?: string
  error?: string
  trailing?: ReactNode
}

/** Styled text input using mga CSS vars. */
export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, label, error, id, trailing, ...props }, ref) => {
    const inputId = id || (label ? label.toLowerCase().replace(/\s+/g, '-') : undefined)

    return (
      <div className="flex flex-col gap-1">
        {label && (
          <label htmlFor={inputId} className="text-sm font-medium text-mga-text">
            {label}
          </label>
        )}
        <div className="relative">
          <input
            ref={ref}
            id={inputId}
            className={cn(
              'w-full rounded-mga border bg-mga-bg px-3 py-2 text-sm text-mga-text',
              'placeholder:text-mga-muted/60',
              'focus:outline-none focus:ring-2 focus:ring-mga-accent/50 focus:border-mga-accent',
              'transition-colors',
              error ? 'border-red-500' : 'border-mga-border',
              trailing ? 'pr-11' : null,
              className,
            )}
            {...props}
          />
          {trailing ? <div className="absolute inset-y-0 right-1 flex items-center">{trailing}</div> : null}
        </div>
        {error && <span className="text-xs text-red-400">{error}</span>}
      </div>
    )
  },
)
Input.displayName = 'Input'
