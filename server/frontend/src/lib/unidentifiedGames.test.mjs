import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

// A game no metadata source can name never becomes a library entry, and until
// now nothing said so anywhere. On one library that hid 75 games — 48 of them
// an entire arcade collection — and the only symptom was a library that felt
// smaller than the drive it came from.
//
// The server has had the whole review-candidates API the entire time, and the
// API client has exported it. No screen called it. That is the fourth capability
// this session found present on both sides of the wire and reachable from
// neither: the Steam QR sign-in, the OAuth callback import, the card icon, and
// this. Each one compiled perfectly.
//
// These tests exist because compiling is not the property that was missing.

const panel = readFileSync(
  new URL('../components/management/UnidentifiedGamesPanel.tsx', import.meta.url),
  'utf8',
)
const libraryPage = readFileSync(
  new URL('../pages/management/LibraryManagementPage.tsx', import.meta.url),
  'utf8',
)

test('a screen actually reads the list of unnamed games', () => {
  assert.ok(
    panel.includes('listManualReviewCandidates'),
    'nothing lists review candidates, so unnamed games are invisible again',
  )
})

test('the panel is rendered by the library page, not merely written', () => {
  // The defect being guarded against is a component that exists and is never
  // mounted, which type-checks and ships.
  assert.ok(
    libraryPage.includes('<UnidentifiedGamesPanel'),
    'the panel is not rendered anywhere, so it might as well not exist',
  )
})

test('it sits above the game list', () => {
  // Someone who cannot find a game looks at the list, does not see it, and
  // concludes it is gone. The explanation has to arrive before that.
  const panelAt = libraryPage.indexOf('<UnidentifiedGamesPanel')
  const listAt = libraryPage.indexOf("title={search || narrowed ? 'Search results' : 'All games'}")
  assert.ok(panelAt > 0 && listAt > 0, 'could not locate both the panel and the game list')
  assert.ok(panelAt < listAt, 'the panel is below the game list, where it answers a question already given up on')
})

test('retrying says what it achieved, not that it finished', () => {
  assert.match(
    panel,
    /Identified \$\{formatCount\(result\.matched\)\}/,
    'the retry result does not report how many games it named',
  )
})
