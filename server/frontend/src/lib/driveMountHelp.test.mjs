import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

// The module is TypeScript with no runtime dependencies, so the behaviour worth
// guarding is read from its source: that each platform gets an answer, that the
// Linux answer is a script rather than a dead end, and above all that nothing
// here ever hands someone a .ps1 to download.

const SOURCE = readFileSync(new URL('./driveMountHelp.ts', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'), 'utf8')

test('Linux gets a script, because Google ships no client for it', () => {
  // This is the case the whole fallback exists for: on Linux detection is
  // expected to find nothing, so instructions are the primary path.
  assert.ok(SOURCE.includes('rclone config'), 'the rclone walkthrough is missing')
  assert.ok(SOURCE.includes('rclone mount'), 'nothing actually mounts the drive')
  assert.ok(SOURCE.includes('Google does not make a Drive client for Linux'), 'the reason is not explained')
})

test('every platform is answered', () => {
  for (const platform of ['windows', 'macos', 'linux']) {
    assert.ok(SOURCE.includes(`case '${platform}'`) || SOURCE.includes(`'${platform}'`), `no branch for ${platform}`)
  }
  // macOS gets both: the official client, and rclone for people who would
  // rather not install it.
  assert.ok(SOURCE.includes('rclone works too'), 'macOS is not offered the rclone alternative')
})

test('nothing is offered as a downloadable PowerShell script', () => {
  // Windows blocks a downloaded .ps1 by default, so a script file is worse than
  // no script at all. Everything here must be text to paste.
  // The module names .ps1 in the comment explaining why it never hands one
  // over, so comments are stripped before looking. Matching the bare string
  // found the explanation and reported it as the offence.
  const code = SOURCE.split('\n').filter((line) => !/^\s*(\/\/|\*|\/\*)/.test(line)).join('\n')
  assert.ok(code.length > 800, `comment stripping ate the module: ${code.length} bytes left`)
  assert.ok(!code.includes('.ps1'), 'a PowerShell script file is being offered')
  assert.ok(!/Set-ExecutionPolicy/.test(SOURCE), 'a Windows execution-policy bypass is being suggested')
  assert.ok(SOURCE.includes('execution policy blocks'), 'the reason for avoiding .ps1 is not recorded')
})

test('the instructions target the server, not the browser', () => {
  // A common way to get this wrong is to tell someone to mount Drive on the
  // laptop they are browsing from, when MGA reads files on the server.
  assert.ok(SOURCE.includes('the machine running MGA'), 'the instructions do not say where to run them')
  assert.ok(SOURCE.includes('a server is often not the machine you browse'), 'the platform guess does not admit it can be wrong')
})

test('the guard is reading the real module', () => {
  // Non-vacuity: every assertion above is a substring check, so an empty or
  // missing file would pass them all as false negatives only if they were
  // negated. Prove the source actually loaded and has the expected shape.
  assert.ok(SOURCE.length > 1500, `source looks empty: ${SOURCE.length} bytes`)
  assert.ok(SOURCE.includes('export function mountInstructions'), 'the exported entry point is missing')
})
