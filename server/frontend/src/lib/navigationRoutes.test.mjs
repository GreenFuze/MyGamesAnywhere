import assert from 'node:assert/strict'
import test from 'node:test'
import { matchRoutes } from 'react-router'
import {
  APP_ROUTE_MATCHERS,
  isCredentialSetupPath,
  resolveSettingsRoute,
} from './navigationRoutes.ts'

test('credential setup remains outside the profile-scoped application', () => {
  assert.equal(isCredentialSetupPath('/credential-setup'), true)
  assert.equal(isCredentialSetupPath('/credential-setup/'), false)
  assert.equal(isCredentialSetupPath('/settings'), false)

  const matches = matchRoutes(APP_ROUTE_MATCHERS, '/credential-setup?ticket=secret#fragment')
  assert.equal(matches?.at(-1)?.route.path, '/credential-setup')
})

test('all gamer deep links match the intended route', () => {
  const cases = [
    ['/play', 'play'],
    ['/play/section/recent', 'play/section/:sectionId'],
    ['/library', 'library'],
    ['/library/section/xbox', 'library/section/:sectionId'],
    ['/library/review?tab=copies', 'library/review'],
    ['/achievements', 'achievements'],
    ['/stats/library', 'stats/library'],
    ['/stats/gamer', 'stats/gamer'],
    ['/settings?tab=devices', 'settings'],
    ['/about', 'about'],
    ['/game/game-123', '/game/:id'],
    ['/game/game-123/media', '/game/:id/media'],
    ['/game/game-123/play', '/game/:id/play'],
  ]

  for (const [url, expectedPath] of cases) {
    const matches = matchRoutes(APP_ROUTE_MATCHERS, url)
    assert.equal(matches?.at(-1)?.route.path, expectedPath, url)
  }
})

test('connection and restore links preserve their profile-scoped query data', () => {
  const params = new URLSearchParams(
    'tab=integrations&integration=xbox&plugin=xbox&first_run=restore',
  )
  assert.deepEqual(resolveSettingsRoute(params, true), { activeTab: 'integrations' })
  assert.equal(params.get('integration'), 'xbox')
  assert.equal(params.get('plugin'), 'xbox')
  assert.equal(params.get('first_run'), 'restore')
})

test('settings navigation keeps role boundaries and legacy redirects', () => {
  assert.deepEqual(resolveSettingsRoute(new URLSearchParams(), true), {
    activeTab: 'integrations',
  })
  assert.deepEqual(resolveSettingsRoute(new URLSearchParams(), false), {
    activeTab: 'my-settings',
  })
  assert.deepEqual(resolveSettingsRoute(new URLSearchParams('tab=devices'), false), {
    activeTab: 'devices',
  })
  assert.deepEqual(resolveSettingsRoute(new URLSearchParams('tab=plugins'), false), {
    activeTab: 'my-settings',
  })
  assert.deepEqual(resolveSettingsRoute(new URLSearchParams('tab=settings'), true), {
    activeTab: 'update',
  })
  assert.deepEqual(resolveSettingsRoute(new URLSearchParams('tab=duplicates'), true), {
    activeTab: 'integrations',
    redirectTo: '/library/review?tab=copies',
  })
  assert.deepEqual(resolveSettingsRoute(new URLSearchParams('tab=undetected'), false), {
    activeTab: 'my-settings',
    redirectTo: '/library/review?tab=identify',
  })
})
