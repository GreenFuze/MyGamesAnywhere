import type { GameMediaDetailDTO } from '@/api/client'

export const HERO_TARGET_ASPECT_RATIO = 16 / 9
export const HERO_MIN_ASPECT_RATIO = 1.55
export const HERO_MAX_ASPECT_RATIO = 2.6
export const MIN_BACKGROUND_WIDTH = 1600
export const MIN_BACKGROUND_HEIGHT = 900

export type BackgroundSuitability = {
  level: 'good' | 'warning'
  reasons: string[]
  actual: {
    width: number
    height: number
    aspectRatio: number
  }
  recommended: {
    minWidth: number
    minHeight: number
    minAspectRatio: number
    maxAspectRatio: number
    targetAspectRatio: number
  }
}

export function evaluateBackgroundSuitability(media: Pick<GameMediaDetailDTO, 'type' | 'width' | 'height'>): BackgroundSuitability {
  const width = media.width ?? 0
  const height = media.height ?? 0
  const aspectRatio = width > 0 && height > 0 ? width / height : 0

  const recommended = {
    minWidth: MIN_BACKGROUND_WIDTH,
    minHeight: MIN_BACKGROUND_HEIGHT,
    minAspectRatio: HERO_MIN_ASPECT_RATIO,
    maxAspectRatio: HERO_MAX_ASPECT_RATIO,
    targetAspectRatio: HERO_TARGET_ASPECT_RATIO,
  }

  const reasons: string[] = []
  if (!width || !height) {
    reasons.push('Image dimensions are unknown.')
  } else {
    if (width < recommended.minWidth) {
      reasons.push(`Width is ${width}px, recommended is at least ${recommended.minWidth}px.`)
    }
    if (height < recommended.minHeight) {
      reasons.push(`Height is ${height}px, recommended is at least ${recommended.minHeight}px.`)
    }
    if (aspectRatio < recommended.minAspectRatio || aspectRatio > recommended.maxAspectRatio) {
      reasons.push(
        `Aspect ratio is ${aspectRatio.toFixed(2)}:1, recommended is roughly ${recommended.targetAspectRatio.toFixed(2)}:1 or wider cinematic artwork.`,
      )
    }
  }

  if (media.type === 'cover') {
    reasons.push('Cover art is a fallback background source and often crops poorly in a wide hero.')
  }

  return {
    level: reasons.length === 0 ? 'good' : 'warning',
    reasons,
    actual: {
      width,
      height,
      aspectRatio,
    },
    recommended,
  }
}

export function formatBackgroundSuitabilityMessage(suitability: BackgroundSuitability): string {
  const actualAspectRatio = suitability.actual.aspectRatio > 0 ? suitability.actual.aspectRatio.toFixed(2) : 'unknown'
  return [
    'This image may not work well as a background.',
    '',
    'Actual:',
    `${suitability.actual.width || 0} × ${suitability.actual.height || 0}`,
    `Aspect ratio: ${actualAspectRatio}:1`,
    '',
    'Recommended:',
    `At least ${suitability.recommended.minWidth} × ${suitability.recommended.minHeight}`,
    `Aspect ratio around ${suitability.recommended.targetAspectRatio.toFixed(2)}:1 (between ${suitability.recommended.minAspectRatio.toFixed(2)}:1 and ${suitability.recommended.maxAspectRatio.toFixed(2)}:1)`,
    '',
    ...suitability.reasons,
    '',
    'Because this image is not wide enough, it may look cropped, zoomed, blurry, or padded in the hero background.',
    '',
    'Use this image anyway?',
  ].join('\n')
}
