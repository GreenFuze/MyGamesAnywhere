import assert from 'node:assert/strict'
import test from 'node:test'

import { ScanConnectionFailurePresenter } from './scanNotifications.ts'

const presenter = new ScanConnectionFailurePresenter()

test('turns an authentication scan failure into an actionable connection notification', () => {
  assert.deepEqual(presenter.present({
    integrationId: 'xbox-orr',
    pluginId: 'game-source-xbox',
    label: 'Orr Xbox',
    reason: 'auth_required',
    error: 'plugin error [AUTH_REQUIRED]: token expired',
  }), {
    title: 'Orr Xbox needs you to sign in',
    description: 'MGA could not check this connection. Sign in again, then rescan. Your existing games were kept.',
    action: {
      label: 'Sign in again',
      href: '/settings?tab=connections&integration=xbox-orr',
    },
    detail: 'plugin error [AUTH_REQUIRED]: token expired',
  })
})

test('links a missing connector by plugin when no connection id is available', () => {
  const notification = presenter.present({
    pluginId: 'game-source-steam',
    label: 'Steam',
    reason: 'plugin_not_found',
  })

  assert.equal(notification?.action.href, '/settings?tab=connections&plugin=game-source-steam')
  assert.match(notification?.description ?? '', /existing games were kept/i)
})

test('does not notify for a non-source connection skipped by discovery', () => {
  assert.equal(presenter.present({
    integrationId: 'metadata-1',
    label: 'Metadata',
    reason: 'no_source_capability',
  }), null)
})
