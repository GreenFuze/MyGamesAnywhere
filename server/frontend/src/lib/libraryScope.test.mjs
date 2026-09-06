import assert from 'node:assert/strict'
import test from 'node:test'
import { describeHiddenGames } from './libraryScope.ts'

// A rule that removes a fifth of an Xbox library has to say so. Measured on the
// owner's library: 99 Xbox games, 53 still on Game Pass, 23 with an unlocked
// achievement, 21 removed — and among those 21 are titles that may simply have
// been bought and never played, which is exactly why the count is shown and
// reversible rather than applied in silence.

test('a rule that removed nothing says nothing', () => {
  assert.equal(describeHiddenGames(0), null)
  assert.equal(describeHiddenGames(-3), null)
  assert.equal(describeHiddenGames(Number.NaN), null)
})

test('one game is described as one game', () => {
  const text = describeHiddenGames(1)
  assert.match(text, /^1 game is hidden/)
  assert.ok(!text.includes('1 games'))
})

test('the notice says how many and why', () => {
  const text = describeHiddenGames(21)
  assert.match(text, /21 games are hidden/)
  // Both halves of the rule appear, so nobody has to guess what was applied.
  assert.match(text, /subscription/)
  assert.match(text, /unlocked/)
})
