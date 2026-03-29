import { useState } from 'react'
import { cn } from '@/lib/utils'

interface CoverImageProps {
  src: string | null
  alt: string
  className?: string
  fit?: 'cover' | 'contain'
  variant?: 'default' | 'compact'
}

/**
 * Lazy-loaded cover art image with a lettered placeholder fallback.
 * Displays first letter of `alt` on a themed background when
 * `src` is null or the image fails to load.
 */
export function CoverImage({
  src,
  alt,
  className,
  fit = 'cover',
  variant = 'default',
}: CoverImageProps) {
  const [errored, setErrored] = useState(false)
  const containPadding = variant === 'compact' ? 'p-1.5' : 'p-3'

  // Show placeholder when no source or image failed to load
  if (!src || errored) {
    const letter = (alt || '?').charAt(0).toUpperCase()
    return (
      <div
        className={cn(
          'flex aspect-[2/3] w-full items-center justify-center bg-gradient-to-br from-mga-elevated via-mga-surface to-mga-elevated text-2xl font-bold text-mga-muted select-none',
          className,
        )}
        role="img"
        aria-label={alt}
      >
        {letter}
      </div>
    )
  }

  if (fit === 'contain') {
    return (
      <div className={cn('relative aspect-[2/3] w-full overflow-hidden bg-mga-elevated', className)}>
        <img
          src={src}
          alt=""
          aria-hidden="true"
          loading="lazy"
          decoding="async"
          onError={() => setErrored(true)}
          className="absolute inset-0 h-full w-full scale-105 object-cover opacity-20 blur-xl"
        />
        <div className="absolute inset-0 bg-gradient-to-br from-mga-bg/70 via-mga-surface/35 to-mga-bg/70" />
        <div className={cn('relative flex h-full w-full items-center justify-center', containPadding)}>
          <img
            src={src}
            alt={alt}
            loading="lazy"
            decoding="async"
            onError={() => setErrored(true)}
            className="h-full w-full object-contain drop-shadow-[0_8px_20px_rgba(0,0,0,0.35)]"
          />
        </div>
      </div>
    )
  }

  return (
    <img
      src={src}
      alt={alt}
      loading="lazy"
      decoding="async"
      onError={() => setErrored(true)}
      className={cn('aspect-[2/3] w-full object-cover', className)}
    />
  )
}
