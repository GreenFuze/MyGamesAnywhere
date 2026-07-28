import assert from 'node:assert/strict'
import test from 'node:test'
import { GameContentPresentation } from './gameContent.ts'

test('base games do not receive a technical content badge', () => {
  const presentation = new GameContentPresentation('base_game')
  assert.equal(presentation.badgeLabel, null)
  assert.equal(presentation.relationshipMessage('unlinked'), null)
})

test('player-facing labels replace raw content values', () => {
  assert.equal(new GameContentPresentation('dlc').badgeLabel, 'DLC')
  assert.equal(new GameContentPresentation('expansion').badgeLabel, 'Expansion')
  assert.equal(new GameContentPresentation('addon').badgeLabel, 'Add-on')
  assert.equal(new GameContentPresentation('patch').badgeLabel, 'Update')
  assert.equal(new GameContentPresentation('extras').badgeLabel, 'Bonus content')
})

test('unlinked and ambiguous add-ons remain actionable without guessing', () => {
  const presentation = new GameContentPresentation('expansion')
  assert.match(presentation.relationshipMessage('unlinked'), /not linked yet/i)
  assert.match(presentation.relationshipMessage('ambiguous'), /Library Review/i)
  assert.equal(presentation.relationshipMessage('known'), null)
})

test('unknown technical values use a review label', () => {
  assert.equal(new GameContentPresentation('future_kind').badgeLabel, 'Needs review')
})
