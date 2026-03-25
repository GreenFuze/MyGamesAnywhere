import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'
import { forwardRef, type HTMLAttributes } from 'react'

const variants = cva(
  'inline-flex items-center rounded-mga px-1.5 py-0.5 text-xs font-medium leading-none',
  {
    variants: {
      variant: {
        default:  'bg-mga-elevated text-mga-muted',
        accent:   'bg-mga-accent/20 text-mga-accent',
        muted:    'bg-mga-muted/20 text-mga-muted',
        playable: 'bg-green-500/20 text-green-400',
        xcloud:   'bg-mga-accent/20 text-mga-accent',
        gamepass: 'bg-purple-500/20 text-purple-400',
        platform: 'bg-mga-elevated/80 text-mga-text border border-mga-border',
        source:   'bg-mga-muted/15 text-mga-muted',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
)

export type BadgeProps = HTMLAttributes<HTMLSpanElement> &
  VariantProps<typeof variants>

export const Badge = forwardRef<HTMLSpanElement, BadgeProps>(
  ({ className, variant, ...props }, ref) => (
    <span ref={ref} className={cn(variants({ variant }), className)} {...props} />
  ),
)
Badge.displayName = 'Badge'
