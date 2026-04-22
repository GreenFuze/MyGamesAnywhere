import { brandLabel, resolveBrandDefinition, type BrandDefinition } from '@/lib/brands'
import { cn } from '@/lib/utils'

type BrandRef = BrandDefinition | string | null | undefined

function resolveBrand(brand: BrandRef): BrandDefinition | null {
  if (!brand) return null
  return typeof brand === 'string' ? resolveBrandDefinition(brand) : brand
}

interface BrandIconProps {
  brand: BrandRef
  className?: string
}

export function BrandIcon({ brand, className }: BrandIconProps) {
  const resolved = resolveBrand(brand)
  if (!resolved?.iconPath) return null

  return (
    <img
      src={resolved.iconPath}
      alt=""
      aria-hidden="true"
      className={cn('h-4 w-4 object-contain', className)}
    />
  )
}

interface BrandMarkProps {
  brand: BrandRef
  className?: string
  tileClassName?: string
}

export function BrandMark({ brand, className, tileClassName }: BrandMarkProps) {
  const resolved = resolveBrand(brand)
  const text = brandLabel(typeof brand === 'string' ? brand : resolved?.id, resolved?.label)
  const lightTile = resolved?.presentation === 'light_tile'

  return (
    <span
      className={cn(
        'flex h-11 w-11 shrink-0 items-center justify-center rounded-mga',
        lightTile ? 'bg-white text-slate-900' : 'bg-transparent text-mga-text',
        tileClassName,
      )}
    >
      {resolved?.iconPath ? (
        <BrandIcon brand={resolved} className={cn('h-7 w-7', className)} />
      ) : (
        <span className={cn('px-2 text-center text-[10px] font-semibold uppercase tracking-wide', className)}>
          {text.slice(0, 4)}
        </span>
      )}
    </span>
  )
}

interface BrandBadgeProps {
  brand: BrandRef
  label?: string
  className?: string
}

export function BrandBadge({ brand, label, className }: BrandBadgeProps) {
  const resolved = resolveBrand(brand)
  const text = label ?? brandLabel(typeof brand === 'string' ? brand : resolved?.id, resolved?.label)

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border border-mga-border bg-mga-surface px-2 py-1 text-xs font-medium text-mga-text',
        className,
      )}
    >
      <BrandIcon brand={resolved} className="h-3.5 w-3.5" />
      <span>{text}</span>
    </span>
  )
}
