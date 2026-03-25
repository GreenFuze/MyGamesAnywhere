import { useState } from 'react'
import { cn } from '@/lib/utils'

interface CoverImageProps {
  src: string | null
  alt: string
  className?: string
}

/**
 * Lazy-loaded cover art image with a lettered placeholder fallback.
 * Displays first letter of `alt` on a themed background when
 * `src` is null or the image fails to load.
 */
export function CoverImage({ src, alt, className }: CoverImageProps) {
  const [errored, setErrored] = useState(false)

  // Show placeholder when no source or image failed to load
  if (!src || errored) {
    const letter = (alt || '?').charAt(0).toUpperCase()
    return (
      <div
        className={cn(
          'flex aspect-[2/3] w-full items-center justify-center bg-mga-elevated text-2xl font-bold text-mga-muted select-none',
          className,
        )}
        role="img"
        aria-label={alt}
      >
        {letter}
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
