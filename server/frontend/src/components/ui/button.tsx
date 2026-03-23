import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'
import { forwardRef, type ButtonHTMLAttributes } from 'react'

const variants = cva(
  'inline-flex items-center justify-center gap-2 rounded-mga text-sm font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-mga-accent disabled:pointer-events-none disabled:opacity-50',
  {
    variants: {
      variant: {
        default: 'bg-mga-accent text-white hover:opacity-90',
        ghost: 'bg-transparent text-mga-text hover:bg-mga-surface',
        outline: 'border border-mga-border bg-mga-surface text-mga-text hover:bg-mga-elevated',
      },
      size: {
        default: 'h-9 px-4 py-2',
        sm: 'h-8 px-3 text-xs',
        icon: 'h-9 w-9 p-0',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> &
  VariantProps<typeof variants>

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => (
    <button ref={ref} className={cn(variants({ variant, size }), className)} {...props} />
  ),
)
Button.displayName = 'Button'
