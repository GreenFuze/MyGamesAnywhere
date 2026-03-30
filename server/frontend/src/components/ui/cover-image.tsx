import { useState } from 'react'
import { cn } from '@/lib/utils'

interface CoverImageProps {
  src: string | null
  alt: string
  className?: string
  fit?: 'cover' | 'contain'
  variant?: 'default' | 'card' | 'compact' | 'sidebar'
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

  const containPadding =
    variant === 'compact'
      ? 'p-1.5'
      : variant === 'sidebar'
        ? 'p-2'
        : variant === 'card'
          ? 'p-4'
          : 'p-3'
  const containImageClass =
    variant === 'compact'
      ? 'max-h-[96%] max-w-[92%]'
      : variant === 'sidebar'
        ? 'max-h-[94%] max-w-[90%]'
        : variant === 'card'
          ? 'max-h-[88%] max-w-[82%]'
          : 'max-h-[92%] max-w-[88%]'
  const containBackdropClass =
    variant === 'compact' || variant === 'sidebar'
      ? 'scale-110 opacity-15 blur-2xl'
      : variant === 'card'
        ? 'scale-[1.15] opacity-25 blur-3xl'
        : 'scale-105 opacity-20 blur-xl'
  const containOverlayClass =
    variant === 'card'
      ? 'from-mga-bg/85 via-mga-surface/45 to-mga-bg/85'
      : 'from-mga-bg/70 via-mga-surface/35 to-mga-bg/70'

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
          className={cn('absolute inset-0 h-full w-full object-cover', containBackdropClass)}
        />
        <div className={cn('absolute inset-0 bg-gradient-to-br', containOverlayClass)} />
        <div className={cn('relative flex h-full w-full items-center justify-center', containPadding)}>
          <img
            src={src}
            alt={alt}
            loading="lazy"
            decoding="async"
            onError={() => setErrored(true)}
            className={cn(
              'h-full w-full object-contain drop-shadow-[0_8px_20px_rgba(0,0,0,0.35)]',
              containImageClass,
            )}
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
