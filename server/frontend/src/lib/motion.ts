const activeScrollAnimations = new WeakMap<HTMLElement, number>()

function easeOutCubic(value: number): number {
  return 1 - Math.pow(1 - value, 3)
}

export function prefersReducedMotion(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

export function animateHorizontalScrollTo(
  element: HTMLElement,
  targetLeft: number,
  durationMs = 220,
) {
  const nextLeft = Math.max(0, targetLeft)
  const currentFrame = activeScrollAnimations.get(element)
  if (currentFrame !== undefined) {
    window.cancelAnimationFrame(currentFrame)
    activeScrollAnimations.delete(element)
  }

  if (prefersReducedMotion()) {
    element.scrollLeft = nextLeft
    return
  }

  const startLeft = element.scrollLeft
  const distance = nextLeft - startLeft
  if (Math.abs(distance) < 2) {
    element.scrollLeft = nextLeft
    return
  }

  const startTime = performance.now()

  const step = (now: number) => {
    const elapsed = now - startTime
    const progress = Math.min(1, elapsed / durationMs)
    element.scrollLeft = startLeft + distance * easeOutCubic(progress)
    if (progress < 1) {
      const frame = window.requestAnimationFrame(step)
      activeScrollAnimations.set(element, frame)
      return
    }
    activeScrollAnimations.delete(element)
  }

  const frame = window.requestAnimationFrame(step)
  activeScrollAnimations.set(element, frame)
}
