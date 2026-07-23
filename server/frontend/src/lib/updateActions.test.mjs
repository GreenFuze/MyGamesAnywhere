import assert from 'node:assert/strict'
import test from 'node:test'

import { resolveUpdateActionPresentation } from './updateActions.ts'

test('offers a combined primary action before the update is downloaded', () => {
  assert.deepEqual(resolveUpdateActionPresentation(false), {
    primaryLabel: 'Download and apply',
    secondaryLabel: 'Download only',
  })
})

test('offers apply and redownload after verification', () => {
  assert.deepEqual(resolveUpdateActionPresentation(true), {
    primaryLabel: 'Apply',
    secondaryLabel: 'Redownload',
  })
})
