import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

// A provider sends the browser back to an address it was registered with, and
// for Microsoft and Google that is 127.0.0.1. When the console is open against
// a server on another machine, the sign-in therefore finishes on the wrong
// computer: measured on 2026-09-06, five Xbox callbacks meant for TV2 arrived
// at the local server instead, each showing "OAuth state is missing, expired,
// or was already used".
//
// The server has always had a way through — POST /api/auth/callback/import,
// which takes the address and validates it against the state the right server
// is holding. The API client has always exported importOAuthCallback. No screen
// called it, so the way through did not exist for anyone using MGA.
//
// That is the same shape of defect as the QR sign-in that was deleted and
// stayed compiling for months. These tests exist so this one cannot repeat.

const sourcesPage = readFileSync(
  new URL('../pages/management/SourcesPage.tsx', import.meta.url),
  'utf8',
)

test('the console offers a way to finish a sign-in that came back to the wrong machine', () => {
  assert.ok(
    sourcesPage.includes('importOAuthCallback'),
    'nothing calls importOAuthCallback, so a remote server can never finish an OAuth sign-in',
  )
})

test('the field appears when a sign-in starts, not only after someone asks for it', () => {
  // Whoever hits this has no way to know the paste-back exists unless the
  // console shows it, and they have just been handed an error page.
  assert.ok(
    sourcesPage.includes('setAwaitingCallback(true)'),
    'the paste-back field is never revealed when a sign-in is started',
  )
})

test('the copy says why the sign-in failed and what to paste', () => {
  assert.match(
    sourcesPage,
    /Copy the whole address from that\s+tab/,
    'the panel does not tell the user what to copy',
  )
  assert.match(
    sourcesPage,
    /another computer/,
    'the panel does not explain that the sign-in landed on the wrong machine',
  )
})
