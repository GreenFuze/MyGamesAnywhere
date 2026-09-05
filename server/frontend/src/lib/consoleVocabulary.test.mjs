import assert from 'node:assert/strict'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'

// The console kept explaining its own architecture to the people using it:
// "canonical records", "profile-scoped catalog projection", "Execution
// authority: None", and a SectionCard that shipped a Jira key to the end user.
// Rewriting those was easy; keeping them out is the part that needs a test,
// because the vocabulary comes back every time someone writes UI copy while
// thinking about the schema.
//
// The first version of this guard only read the management pages, and missed
// the worst offender of all: the sidebar wordmark said "Control plane" on every
// screen in the product. So it reads the shell and the navigation table too.

const SRC = new URL('../', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1')

// Extended twice already, each time after copy shipped somewhere the guard was
// not looking: first the shell, whose wordmark read "Control plane", then the
// management components, which is where most new panels put their words.
const SCANNED = [
  { path: 'pages/management', kind: 'dir' },
  { path: 'components/management', kind: 'dir' },
  { path: 'layouts', kind: 'dir' },
  { path: 'lib/navigationRoutes.ts', kind: 'file' },
]

/** Words that describe our implementation rather than the user's situation. */
const BANNED = [
  { term: 'canonical', why: 'our identity vocabulary; the user has games' },
  { term: 'normalized', why: 'describes our pipeline, not their library' },
  { term: 'normalization', why: 'describes our pipeline, not their library' },
  { term: 'projection', why: 'a database view, not a thing a person has' },
  { term: 'control plane', why: 'architecture slogan' },
  { term: 'source record', why: 'a row count; say where the game came from' },
  { term: 'base_game', why: 'a raw enum must never reach a pixel' },
  { term: 'fails closed', why: 'a design property, not a user-facing promise' },
  { term: 'retirement window', why: 'internal migration vocabulary' },
  { term: 'execution authority', why: 'a card that only said what we do not do' },
  { term: 'freshness window', why: 'say when it was last checked instead' },
  { term: 'compliance blocker', why: 'say it cannot be sent yet' },
  // Added after a screen-by-screen tour found each of these still shipping.
  { term: 'stale evidence', why: 'say when it was last checked' },
  { term: 'compliance evidence', why: 'say the licence or checksum is unconfirmed' },
  { term: 'artifact', why: 'the user has emulators and runtimes, not artifacts' },
  { term: 'runtime supply', why: 'inventory vocabulary' },
  { term: 'progress data', why: 'they are achievements' },
  { term: 'refresh failure', why: 'say it could not update' },
  { term: 'frontend integration', why: 'it is an app they connected' },
  { term: 'api scope', why: 'they are permissions' },
  { term: 'current context', why: 'say which profile is in use' },
  { term: 'underlying evidence', why: 'name the problem instead of pointing at it' },
  { term: 'management area', why: 'name the page' },
]

const JIRA_KEY = /\bMGA-\d+\b/

function filesToScan() {
  const files = []
  for (const entry of SCANNED) {
    const full = join(SRC, entry.path)
    if (entry.kind === 'file') {
      files.push(full)
      continue
    }
    for (const name of readdirSync(full)) {
      if ((name.endsWith('.tsx') || name.endsWith('.ts')) && statSync(join(full, name)).isFile()) {
        files.push(join(full, name))
      }
    }
  }
  return files
}

/** Strings that reach the screen: JSX copy props, object-literal copy fields,
 *  and text sitting between tags. */
export function userFacingText(source) {
  const parts = []
  const jsxProp = /\b(title|description|detail|label|eyebrow|emptyTitle|emptyDescription|placeholder|aria-label)="([^"]+)"/g
  for (const match of source.matchAll(jsxProp)) parts.push(match[2])
  const objectField = /\b(title|description|detail|label|eyebrow)\s*:\s*'([^']+)'/g
  for (const match of source.matchAll(objectField)) parts.push(match[2])
  const between = />\s*([A-Z][^<>{}\n]{6,})</g
  for (const match of source.matchAll(between)) parts.push(match[1])
  return parts
}

const files = filesToScan()

test('there are console files to check', () => {
  // Guards the guard: a glob that silently matches nothing would make every
  // assertion below vacuously true.
  assert.ok(files.length >= 14, `expected the console sources, found ${files.length}`)
})

test('no user-facing copy explains our architecture', () => {
  const offences = []
  for (const file of files) {
    const source = readFileSync(file, 'utf8')
    const name = file.slice(SRC.length)
    for (const text of userFacingText(source)) {
      const lowered = text.toLowerCase()
      for (const { term, why } of BANNED) {
        if (lowered.includes(term)) offences.push(`${name}: "${text}" contains "${term}" — ${why}`)
      }
    }
  }
  assert.deepEqual(offences, [], `internal vocabulary reached the UI:\n${offences.join('\n')}`)
})

test('no user-facing copy ships a ticket number', () => {
  const offences = []
  for (const file of files) {
    const source = readFileSync(file, 'utf8')
    for (const text of userFacingText(source)) {
      if (JIRA_KEY.test(text)) offences.push(`${file.slice(SRC.length)}: "${text}"`)
    }
  }
  assert.deepEqual(offences, [], `a Jira key is being shown to the user:\n${offences.join('\n')}`)
})

test('the banned-term check can actually fail', () => {
  // Non-vacuity: prove the extractor sees each shape of copy and that the
  // matcher rejects it, otherwise a broken regex makes the tests above pass on
  // anything at all.
  const jsx = '<PageIntro title="Canonical records" description="A normalized projection." />'
  assert.deepEqual(userFacingText(jsx), ['Canonical records', 'A normalized projection.'])

  const literal = "{ id: 'x', description: 'Control plane health' }"
  assert.deepEqual(userFacingText(literal), ['Control plane health'])

  const markup = '<span>Execution authority is none</span>'
  assert.deepEqual(userFacingText(markup), ['Execution authority is none'])

  for (const sample of [jsx, literal, markup]) {
    const hit = userFacingText(sample).some((text) => BANNED.some(({ term }) => text.toLowerCase().includes(term)))
    assert.ok(hit, `the matcher failed to reject: ${sample}`)
  }
})
