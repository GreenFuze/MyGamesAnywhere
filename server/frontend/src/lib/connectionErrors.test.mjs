import assert from 'node:assert/strict'
import test from 'node:test'
import { explainConnectionFailure } from './connectionErrors.ts'

test('a provider that skipped interactive sign-in is explained and not retryable', () => {
  const failure = explainConnectionFailure(
    new Error('provider plugin did not start interactive account authorization'),
    'Xbox',
  )
  assert.ok(failure)
  assert.equal(failure.terminal, true)
  assert.match(failure.title, /Xbox/)
  // The operator must learn that nothing was created and nothing was stored.
  assert.match(failure.detail, /nothing was created/i)
  assert.match(failure.detail, /no credentials were stored/i)
  // A refusal with no remedy is a dead end: name the action that clears it.
  assert.match(failure.detail, /rebuild or reinstall/i)
  // And say why the refusal exists, not just that it happened.
  assert.match(failure.detail, /different profile/i)
})

test('recoverable sign-in problems are not marked terminal', () => {
  for (const message of [
    'OAuth sign-in state is missing or expired',
    'OAuth sign-in has not completed',
    'OAuth sign-in state does not belong to this profile connection',
  ]) {
    const failure = explainConnectionFailure(new Error(message), 'Google Drive')
    assert.ok(failure, message)
    assert.equal(failure.terminal, false, message)
  }
})

test('ordinary validation errors are left to speak for themselves', () => {
  assert.equal(explainConnectionFailure(new Error('config: field "username" is required'), 'SMB File Share'), null)
  assert.equal(explainConnectionFailure(new Error('failed to connect to host'), 'SMB File Share'), null)
  assert.equal(explainConnectionFailure(null, 'Steam'), null)
  assert.equal(explainConnectionFailure(undefined, 'Steam'), null)
  assert.equal(explainConnectionFailure(new Error(''), 'Steam'), null)
})

test('a plain string rejection is still explained', () => {
  const failure = explainConnectionFailure('provider plugin did not start interactive account authorization', 'Xbox')
  assert.ok(failure)
  assert.equal(failure.terminal, true)
})
