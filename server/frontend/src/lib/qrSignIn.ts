export type RotatableQRChallenge = {
  challenge_url: string
}

/**
 * Applies a provider-issued replacement QR payload without changing the
 * in-progress session identity. Blank replacements mean the current QR remains
 * valid and are deliberately ignored.
 */
export function rotateQRChallenge<T extends RotatableQRChallenge>(
  challenge: T,
  replacementURL?: string,
): T {
  const normalized = replacementURL?.trim()
  if (!normalized || normalized === challenge.challenge_url) return challenge
  return { ...challenge, challenge_url: normalized }
}
