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

/**
 * The account a connection is signed in as, if it is.
 *
 * The server redacts the refresh token from anything a browser sees but keeps
 * provider_identity for exactly this: so a person can confirm which account a
 * connection owns. Nothing was reading it, which is why a signed-in connection
 * kept offering to sign in.
 */
export type SignedInAccount = {
  provider: string
  subject: string
  display_name?: string
  avatar_url?: string
}

export function signedInAccount(configJSON: string | undefined): SignedInAccount | null {
  if (!configJSON) return null
  try {
    const parsed = JSON.parse(configJSON) as { provider_identity?: unknown }
    const identity = parsed.provider_identity
    if (!identity || typeof identity !== 'object') return null
    const account = identity as SignedInAccount
    // A subject is what makes it an account rather than a leftover fragment.
    return typeof account.subject === 'string' && account.subject.trim() !== '' ? account : null
  } catch {
    // A connection whose stored configuration will not parse is not a reason to
    // break the card it is drawn on.
    return null
  }
}
