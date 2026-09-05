import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
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

// --- Reachability ----------------------------------------------------------
// This panel and its two lib files were deleted by the retirement of the
// first-party player, which took the whole settings shell with it. Nothing
// noticed: the server kept answering /api/auth/qr/{plugin}/begin and /poll,
// the API client kept exporting beginQRSignIn and pollQRSignIn, everything
// compiled, every test passed, and the only way to sign in to Steam was gone
// for months.
//
// A type checker cannot catch that — unreferenced code is not an error — so
// this asserts that a screen still offers the sign-in.

const SRC = new URL('../', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1')

function consoleSources() {
  const files = []
  for (const dir of ['pages/management', 'components/management']) {
    for (const name of readdirSync(join(SRC, dir))) {
      if (name.endsWith('.tsx') && !name.startsWith('QRSignIn')) {
        files.push(join(SRC, dir, name))
      }
    }
  }
  return files
}

test('the console sources can be read', () => {
  // Guards the guard: an empty listing would make the next assertions vacuous.
  const files = consoleSources()
  assert.ok(files.length >= 10, `expected the console screens, found ${files.length}`)
})

test('a screen still offers the app sign-in', () => {
  const offering = consoleSources().filter((file) => {
    const source = readFileSync(file, 'utf8')
    // Anchored: <QRSignInSomethingElse is not this panel.
    return /<QRSignIn[\s/>]/.test(source) && source.includes('pluginQRSignInField')
  })
  assert.ok(
    offering.length > 0,
    'nothing renders QRSignIn for a provider that signs in through its app; the server endpoints are unreachable again',
  )
})

test('the panel is what actually calls the sign-in endpoints', () => {
  const panel = readFileSync(join(SRC, 'components/management/QRSignIn.tsx'), 'utf8')
  for (const call of ['beginQRSignIn', 'pollQRSignIn', 'rotateQRChallenge']) {
    assert.ok(panel.includes(call), `the panel no longer uses ${call}`)
  }
})
