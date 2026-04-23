import { useState } from 'react'
import { cn } from '@/lib/utils'

interface CoverImageProps {
  src: string | null
  alt: string
  className?: string
  fit?: 'cover' | 'contain'
  variant?: 'default' | 'card' | 'compact' | 'sidebar' | 'hero'
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
        : variant === 'hero'
          ? 'p-5 md:p-6'
          : variant === 'card'
            ? 'p-3'
            : 'p-3'
  const containImageClass =
    variant === 'compact'
      ? 'max-h-[96%] max-w-[92%]'
      : variant === 'sidebar'
        ? 'max-h-[94%] max-w-[90%]'
        : variant === 'hero'
          ? 'max-h-[94%] max-w-[86%]'
          : variant === 'card'
            ? 'max-h-[92%] max-w-[84%]'
            : 'max-h-[92%] max-w-[88%]'
  const containBackdropClass =
    variant === 'compact'
      ? 'scale-110 opacity-10 blur-xl'
      : variant === 'sidebar'
        ? 'scale-110 opacity-10 blur-xl'
        : variant === 'hero'
          ? 'scale-[1.2] opacity-30 blur-[42px]'
          : variant === 'card'
            ? 'scale-[1.18] opacity-25 blur-[30px]'
            : 'scale-105 opacity-20 blur-xl'
  const containOverlayClass =
    variant === 'hero'
      ? 'from-black/25 via-mga-surface/10 to-black/55'
      : variant === 'card'
        ? 'from-black/20 via-transparent to-black/40'
        : 'from-mga-bg/70 via-mga-surface/35 to-mga-bg/70'
  const containShellClass =
    variant === 'hero'
      ? 'rounded-[26px] border border-white/10 bg-black/40 shadow-[0_30px_80px_rgba(0,0,0,0.45)]'
      : variant === 'card'
        ? 'rounded-[20px] border border-white/10 bg-black/40 shadow-[0_14px_34px_rgba(0,0,0,0.34)]'
        : variant === 'compact'
          ? 'rounded-sm'
          : 'rounded-mga border border-mga-border/60 bg-mga-elevated/90'

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
      <div className={cn('relative aspect-[2/3] w-full overflow-hidden', containShellClass, className)}>
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
        {(variant === 'card' || variant === 'hero') && (
          <>
            <div className="absolute inset-x-0 top-0 h-16 bg-gradient-to-b from-black/30 to-transparent" />
            <div className="absolute inset-x-0 bottom-0 h-24 bg-gradient-to-t from-black/30 to-transparent" />
          </>
        )}
        <div className={cn('relative flex h-full w-full items-center justify-center', containPadding)}>
          <img
            src={src}
            alt={alt}
            loading="lazy"
            decoding="async"
            onError={() => setErrored(true)}
            className={cn(
              'h-full w-full object-contain drop-shadow-[0_10px_30px_rgba(0,0,0,0.42)]',
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
