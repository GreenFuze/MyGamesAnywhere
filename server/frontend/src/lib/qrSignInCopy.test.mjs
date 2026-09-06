import assert from 'node:assert/strict'
import test from 'node:test'
import { providerAppName, qrSignInPurpose, signedInAccount } from './qrSignInCopy.ts'

// A connection that is signed in kept offering to sign in, because nothing read
// the one field the server deliberately keeps for it. The refresh token is
// redacted from every browser response, so provider_identity is the only
// evidence a card has.

test('a signed-in connection is recognised by its account', () => {
  const account = signedInAccount(JSON.stringify({
    steam_id: '76561197995983754',
    provider_identity: { provider: 'steam', subject: '76561197995983754', display_name: 'greenfuze' },
  }))
  assert.equal(account?.display_name, 'greenfuze')
})

test('a connection that has never signed in has no account', () => {
  assert.equal(signedInAccount(JSON.stringify({ api_key: 'k', steam_id: '765' })), null)
  assert.equal(signedInAccount('{}'), null)
  assert.equal(signedInAccount(undefined), null)
})

test('a fragment without a subject is not an account', () => {
  // Half-written identities have appeared in stored configuration before. One
  // without a subject identifies nobody and must not read as signed in.
  assert.equal(signedInAccount(JSON.stringify({ provider_identity: { provider: 'steam' } })), null)
  assert.equal(signedInAccount(JSON.stringify({ provider_identity: { provider: 'steam', subject: '  ' } })), null)
  assert.equal(signedInAccount(JSON.stringify({ provider_identity: 'greenfuze' })), null)
})

test('unparseable configuration does not break the card', () => {
  assert.equal(signedInAccount('{not json'), null)
})

test('the words are the same wherever the sign-in is offered', () => {
  assert.equal(providerAppName('game-source-steam'), 'Steam mobile')
  assert.equal(qrSignInPurpose('game-source-steam'), 'Sign in to Steam')
  // A provider nobody has written words for still gets a usable sentence.
  assert.ok(qrSignInPurpose('game-source-unknown').length > 0)
  assert.ok(providerAppName('game-source-unknown', 'Thing').length > 0)
})
