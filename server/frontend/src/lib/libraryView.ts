/**
 * Which way the Library shows games, remembered between visits.
 *
 * Detailed is the default. A wall of covers is pleasant and says almost
 * nothing: not the platform, not which source it came from, not whether it is
 * in a subscription, not whether it has gone missing. The list says all of
 * that, so it is what someone lands on; the covers are there when they want to
 * look rather than read.
 */

export type LibraryView = 'detailed' | 'grid'

const STORAGE_KEY = 'mga.library-view.v1'

export function readLibraryView(): LibraryView {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === 'grid' ? 'grid' : 'detailed'
  } catch {
    // Private windows and blocked site data throw on access. A preference is
    // not worth an error, and the default is the right answer anyway.
    return 'detailed'
  }
}

export function storeLibraryView(view: LibraryView): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, view)
  } catch {
    // Nothing to do: the choice still applies for this visit.
  }
}
