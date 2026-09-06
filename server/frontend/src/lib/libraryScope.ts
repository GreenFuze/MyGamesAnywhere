/**
 * Whether the Library hides games a rented catalogue no longer carries.
 *
 * A subscription source reports what an account has played, not what it owns,
 * so it lists titles that were tried once on a subscription and cannot be
 * started today without buying them. Those crowd out the games someone can
 * actually play.
 *
 * On by default, because the common case is wanting a library of playable
 * games. Remembered, and never silent: the Library says how many it removed
 * and offers them back in one click, because the rule can be wrong — a title
 * bought outright that nobody unlocked an achievement in looks exactly like a
 * lapsed subscription title from the outside.
 */

export type LibraryScope = 'playable' | 'everything'

const STORAGE_KEY = 'mga.library-scope.v1'

export function readLibraryScope(): LibraryScope {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === 'everything' ? 'everything' : 'playable'
  } catch {
    // Private windows and blocked site data throw on access. The default is
    // the right answer anyway.
    return 'playable'
  }
}

export function storeLibraryScope(scope: LibraryScope): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, scope)
  } catch {
    // Nothing to do: the choice still applies for this visit.
  }
}

/** What to say about the games the rule removed, if it removed any. */
export function describeHiddenGames(count: number): string | null {
  if (!Number.isFinite(count) || count <= 0) return null
  return count === 1
    ? '1 game is hidden: a subscription no longer carries it and nothing was unlocked in it.'
    : `${count} games are hidden: a subscription no longer carries them and nothing was unlocked in them.`
}
