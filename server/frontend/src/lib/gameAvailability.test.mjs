import assert from 'node:assert/strict'
import test from 'node:test'
import { availabilityLabel, describePlayability, entitlementLabel, isStale, offersForGame } from './gameAvailability.ts'

// MGA-118 kept entitlement and availability apart, with a deliberate "unknown"
// in each, precisely so this question could be answered honestly: can I play
// it, must I buy it, or is it gone? These tests guard the two ways that can be
// got wrong, both of which mislead in a direction that costs the reader money
// or a game.

function offer(overrides) {
  return {
    id: 'offer-1',
    canonical_game_id: 'game-1',
    provider: 'game-source-xbox',
    sku: 'SKU',
    platform: 'windows_pc',
    region: 'US',
    entitlement: 'unknown',
    delivery: 'storefront',
    availability: 'unknown',
    observed_at: '2026-09-01T00:00:00Z',
    last_success_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
    ...overrides,
  }
}

test('unknown entitlement is never rounded up to ownership', () => {
  // The Xbox connector reads play history, not entitlements. Rendering that as
  // "you own this" would assert something MGA never observed, and would tell
  // someone they need not buy a game that they do.
  const answer = describePlayability(offer({ entitlement: 'unknown', availability: 'available' }))
  assert.match(answer.headline, /cannot tell/i)
  assert.doesNotMatch(answer.headline, /own/i)
  assert.equal(answer.tone, 'neutral')
  assert.match(answer.detail, /does not say/i)
})

test('unknown availability is never reported as removal', () => {
  // A provider going quiet must not make MGA announce that a game is gone.
  const answer = describePlayability(offer({ entitlement: 'owned', availability: 'unknown' }))
  assert.match(answer.headline, /own/i)
  assert.doesNotMatch(answer.headline, /gone|no longer/i)
  assert.equal(answer.tone, 'good')
})

test('owned but delisted is distinguished from simply gone', () => {
  // These need different actions from the reader: one still has their copy,
  // the other has to buy it somewhere else or do without.
  const owned = describePlayability(offer({ entitlement: 'owned', availability: 'unavailable' }))
  assert.match(owned.headline, /own this, but it is gone/i)
  assert.match(owned.detail, /already downloaded still works/i)
  assert.equal(owned.tone, 'attention')

  const gone = describePlayability(offer({ entitlement: 'none', availability: 'unavailable' }))
  assert.match(gone.headline, /no longer available/i)
  assert.equal(gone.tone, 'danger')
})

test('a subscription title leaving soon says what happens after', () => {
  const answer = describePlayability(offer({ entitlement: 'subscription', availability: 'leaving_soon' }))
  assert.match(answer.detail, /would have to buy it/i)
  assert.equal(answer.tone, 'attention')
})

test('needing to buy it is stated plainly', () => {
  const answer = describePlayability(offer({ entitlement: 'none', availability: 'available' }))
  assert.match(answer.headline, /need to buy/i)
  assert.equal(answer.tone, 'attention')
})

test('every entitlement and availability value has words', () => {
  // A value with no mapping would render as an empty pill, which reads as
  // "nothing is wrong" rather than "we did not handle this".
  for (const value of ['owned', 'subscription', 'shared', 'trial', 'none', 'unknown']) {
    assert.ok(entitlementLabel(value).length > 0, `no words for entitlement ${value}`)
  }
  for (const value of ['available', 'leaving_soon', 'unavailable', 'unknown']) {
    assert.ok(availabilityLabel(value).length > 0, `no words for availability ${value}`)
  }
  assert.equal(entitlementLabel('something-new-from-a-provider'), 'Unknown')
})

test('offers are filtered to the game and newest first', () => {
  const mine = offer({ id: 'a', observed_at: '2026-08-01T00:00:00Z' })
  const newer = offer({ id: 'b', observed_at: '2026-09-04T00:00:00Z' })
  const other = offer({ id: 'c', canonical_game_id: 'game-2' })

  const found = offersForGame([mine, other, newer], 'game-1')
  assert.deepEqual(found.map((entry) => entry.id), ['b', 'a'], 'a stale duplicate must not outrank the current answer')
})

test('staleness is read from the field, not inferred from a date', () => {
  // stale_at was a zero time serialized as "0001-01-01T00:00:00Z" until it was
  // made a pointer, and every fresh offer looked stale because that string is
  // truthy. Absent means fresh.
  assert.equal(isStale(offer({})), false)
  assert.equal(isStale(offer({ stale_at: '2026-09-01T00:00:00Z' })), true)
})
