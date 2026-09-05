import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { gameBadges, gameSourceNames } from './gameBadges.ts'

// Badges are the shortest thing on the screen and therefore the easiest place
// to say something false. The rule inherited from MGA-118 is that absence is
// never evidence: a game without the Game Pass flag is not thereby owned — it
// is equally a demo, a trial, an expired subscription, or a family share.

function game(overrides) {
  return {
    id: 'game-1',
    title: 'A Game',
    favorite: false,
    platform: 'windows_pc',
    kind: 'base_game',
    media: [{ asset_id: 1, type: 'cover', url: 'https://example.test/c.png' }],
    source_games: [{ id: 's1', integration_id: 'i1', plugin_id: 'game-source-xbox', external_id: 'x', raw_title: 'A Game', platform: 'windows_pc', kind: 'base_game', status: 'found', created_at: '', files: [] }],
    ...overrides,
  }
}

function offer(overrides) {
  return {
    id: 'offer-1', canonical_game_id: 'game-1', provider: 'game-source-xbox', sku: 'S',
    platform: 'windows_pc', region: 'US', entitlement: 'unknown', delivery: 'storefront',
    availability: 'unknown', observed_at: '2026-09-01T00:00:00Z', last_success_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z', ...overrides,
  }
}

const ids = (badges) => badges.map((badge) => badge.id)

test('no Game Pass flag never becomes an ownership claim', () => {
  // The whole MGA-118 argument in one assertion. A game the provider said
  // nothing about must carry no entitlement badge at all.
  const badges = gameBadges(game({ is_game_pass: false }), [])
  assert.ok(!ids(badges).includes('owned'), `claimed ownership: ${ids(badges)}`)
  assert.ok(!ids(badges).includes('game-pass'), `claimed a subscription: ${ids(badges)}`)
})

test('Game Pass is shown only when the provider said so', () => {
  assert.ok(ids(gameBadges(game({ is_game_pass: true }), [])).includes('game-pass'))
  assert.ok(!ids(gameBadges(game({}), [])).includes('game-pass'))
})

test('ownership comes from a recorded offer, not from a flag', () => {
  const badges = gameBadges(game({}), [offer({ entitlement: 'owned', availability: 'available' })])
  assert.ok(ids(badges).includes('owned'))
})

test('unknown entitlement produces no entitlement badge', () => {
  // Not "owned", not "must be bought" — nothing, because nothing is known.
  const badges = gameBadges(game({}), [offer({ entitlement: 'unknown', availability: 'available' })])
  for (const claim of ['owned', 'purchase', 'trial', 'shared']) {
    assert.ok(!ids(badges).includes(claim), `${claim} was claimed without evidence`)
  }
})

test('unknown availability produces no availability badge', () => {
  const badges = gameBadges(game({}), [offer({ availability: 'unknown' })])
  assert.ok(!ids(badges).includes('unavailable'))
  assert.ok(!ids(badges).includes('leaving'))
})

test('the states the owner asked to tell apart are distinct badges', () => {
  // "which games to display, can be installed, which needs to be purchased,
  // and which simply not available anymore".
  assert.ok(ids(gameBadges(game({}), [offer({ availability: 'unavailable' })])).includes('unavailable'))
  assert.ok(ids(gameBadges(game({}), [offer({ availability: 'leaving_soon' })])).includes('leaving'))
  assert.ok(ids(gameBadges(game({}), [offer({ entitlement: 'none', availability: 'available' })])).includes('purchase'))
  assert.ok(ids(gameBadges(game({ is_game_pass: true }), [])).includes('game-pass'))
})

test('what you have lost is listed before what you have', () => {
  // Scanning a row, the first badge should be the one that changes what you do.
  const badges = gameBadges(
    game({ is_game_pass: true, xcloud_available: true, source_games: [{ ...game({}).source_games[0], status: 'not_found' }] }),
    [offer({ availability: 'unavailable' })],
  )
  assert.equal(badges[0].id, 'missing')
  assert.equal(badges[1].id, 'unavailable')
})

test('achievements show progress and mark completion', () => {
  const partial = gameBadges(game({ achievement_summary: { source_count: 1, total_count: 10, unlocked_count: 3 } }), [])
  const badge = partial.find((entry) => entry.id === 'achievements')
  assert.equal(badge.label, '3/10')
  assert.equal(badge.tone, 'neutral')

  const done = gameBadges(game({ achievement_summary: { source_count: 1, total_count: 10, unlocked_count: 10 } }), [])
  assert.equal(done.find((entry) => entry.id === 'achievements').tone, 'good')
})

test('a game with no achievements gets no achievement badge', () => {
  assert.ok(!ids(gameBadges(game({ achievement_summary: { source_count: 0, total_count: 0, unlocked_count: 0 } }), [])).includes('achievements'))
  assert.ok(!ids(gameBadges(game({}), [])).includes('achievements'))
})

test('missing needs every source to have lost it', () => {
  // One source losing a game that another still has is not the reader's
  // problem, and flagging it would cry wolf.
  const one = game({}).source_games[0]
  const badges = gameBadges(game({ source_games: [{ ...one, status: 'not_found' }, { ...one, id: 's2', status: 'found' }] }), [])
  assert.ok(!ids(badges).includes('missing'))
})

test('sources are named by their connection and deduplicated', () => {
  const one = game({}).source_games[0]
  const names = gameSourceNames(
    game({ source_games: [
      { ...one, integration_label: 'Xbox' },
      { ...one, id: 's2', integration_label: 'Xbox' },
      { ...one, id: 's3', integration_label: 'GF Google Drive' },
    ] }),
    (pluginId) => pluginId,
  )
  assert.deepEqual(names, ['GF Google Drive', 'Xbox'])
})

test('every badge carries something to show', () => {
  // An empty label renders as a blank pill, which reads as "fine" rather than
  // as a bug.
  const badges = gameBadges(
    game({ is_game_pass: true, xcloud_available: true, favorite: true, kind: 'dlc', media: [], achievement_summary: { source_count: 1, total_count: 5, unlocked_count: 5 } }),
    [offer({ entitlement: 'owned', availability: 'leaving_soon' })],
  )
  assert.ok(badges.length >= 6, `expected a full row, got ${ids(badges)}`)
  for (const badge of badges) {
    assert.ok(badge.label.trim().length > 0, `empty label on ${badge.id}`)
    assert.ok(['good', 'attention', 'danger', 'neutral'].includes(badge.tone), `bad tone on ${badge.id}`)
  }
})

// --- Icons -----------------------------------------------------------------
// A badge names its icon and the component resolves the name. Nothing at
// runtime checks that the name resolves, so a badge added without an entry in
// the map would render an empty pill and no test would notice. This is that
// check, and it has to read the component as text because the Node runner
// cannot load a .tsx.

const BADGES_SOURCE = readFileSync(
  new URL('../components/management/GameBadges.tsx', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'),
  'utf8',
)

/** The keys of the ICONS map, read out of the component. */
function mappedIconNames(source) {
  // From the opening brace of the map to the line that closes it: the
  // declaration itself contains braces, so the first one is not the map.
  const start = source.indexOf('= {', source.indexOf('const ICONS'))
  const end = source.indexOf('\n}', start)
  const block = source.slice(start, end)
  const names = []
  for (const match of block.matchAll(/^\s*'?([a-z-]+)'?\s*:/gm)) names.push(match[1])
  return names
}

test('the icon map can actually be read out of the component', () => {
  // Guards the guard: a regex that matched nothing would make the next test
  // pass on an empty map.
  const names = mappedIconNames(BADGES_SOURCE)
  assert.ok(names.length >= 10, `found only ${names.length} icons in the component`)
  assert.ok(names.includes('favorite'), `the map does not look like the icon map: ${names.join(', ')}`)
})

test('every badge names an icon the component can draw', () => {
  const mapped = new Set(mappedIconNames(BADGES_SOURCE))
  // One game carrying as many badges at once as the code can produce.
  const loud = gameBadges({
    id: 'g1',
    title: 'Everything',
    kind: 'dlc',
    favorite: true,
    is_game_pass: true,
    xcloud_available: true,
    media: [],
    achievement_summary: { total_count: 10, unlocked_count: 3 },
    source_games: [{ id: 's1', status: 'missing' }],
  }, [
    { id: 'o1', canonical_game_id: 'g1', availability: 'leaving_soon', entitlement: 'trial' },
  ])
  assert.ok(loud.length >= 6, `expected many badges at once, got ${loud.length}`)
  const unmapped = loud.filter((badge) => !mapped.has(badge.icon))
  assert.deepEqual(unmapped.map((badge) => badge.id), [], 'these badges would render without a picture')
})

test('the other availability and entitlement badges name icons too', () => {
  const mapped = new Set(mappedIconNames(BADGES_SOURCE))
  for (const [availability, entitlement] of [
    ['unavailable', 'owned'],
    ['available', 'none'],
    ['available', 'shared'],
  ]) {
    const badges = gameBadges(
      { id: 'g2', title: 'One', media: [{ asset_id: 1, type: 'cover' }], source_games: [{ id: 's', status: 'found' }] },
      [{ id: 'o', canonical_game_id: 'g2', availability, entitlement }],
    )
    for (const badge of badges) {
      assert.ok(mapped.has(badge.icon), `${badge.id} names "${badge.icon}", which the component cannot draw`)
    }
  }
})
