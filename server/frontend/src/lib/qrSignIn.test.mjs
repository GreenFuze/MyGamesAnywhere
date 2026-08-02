import assert from 'node:assert/strict'
import test from 'node:test'

import { rotateQRChallenge } from './qrSignIn.ts'

const session = {
  status: 'pending',
  client_id: 'client-1',
  request_id: 'request-1',
  challenge_url: 'https://s.team/q/old',
  interval_seconds: 5,
}

test('replaces only the QR payload when Steam rotates a live challenge', () => {
  const rotated = rotateQRChallenge(session, ' https://s.team/q/new ')

  assert.deepEqual(rotated, {
    ...session,
    challenge_url: 'https://s.team/q/new',
  })
})

test('keeps the current challenge when Steam does not rotate it', () => {
  assert.equal(rotateQRChallenge(session), session)
  assert.equal(rotateQRChallenge(session, '   '), session)
  assert.equal(rotateQRChallenge(session, session.challenge_url), session)
})
