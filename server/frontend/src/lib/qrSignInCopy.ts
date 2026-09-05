import { brandLabel } from './brands.ts'

/**
 * The words around an app-approved sign-in.
 *
 * Two screens offer the same sign-in — the connection dialog and the
 * connection card — and they must describe it identically, so the wording
 * lives here rather than in either of them.
 */

const APP_NAMES: Record<string, string> = {
  'game-source-steam': 'Steam mobile',
}

/** The app the player approves the sign-in in, as they know it. */
export function providerAppName(pluginId: string, fallback?: string): string {
  return APP_NAMES[pluginId] ?? brandLabel(pluginId, fallback ?? 'provider')
}

const PURPOSES: Record<string, string> = {
  'game-source-steam': 'Sign in to Steam',
}

/** What this sign-in is for, said as a thing the player wants. */
export function qrSignInPurpose(pluginId: string): string {
  return PURPOSES[pluginId] ?? 'Sign in with the app'
}

const EXPLANATIONS: Record<string, string> = {
  'game-source-steam':
    'Signing in reads your library as your own account rather than through an API key, and it is the only way to see Steam Family shared games.',
}

/** Why it is worth doing, where there is a reason worth giving. */
export function qrSignInReason(pluginId: string): string | null {
  return EXPLANATIONS[pluginId] ?? null
}
