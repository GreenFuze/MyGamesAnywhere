import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

// gameUtils.ts and PluginIcon.tsx cannot be imported here — the first uses "@/"
// value imports and the second is JSX — so these read the source. Registration
// in a hand-maintained lookup is exactly the kind of thing that gets forgotten,
// and a missing entry fails silently at runtime rather than loudly at build.
const here = dirname(fileURLToPath(import.meta.url))
const read = (relativePath) => readFileSync(join(here, relativePath), 'utf8')

const gameUtils = read('gameUtils.ts')
const displayText = read('displayText.ts')
const pluginIcon = read('../components/settings/PluginIcon.tsx')

/** Pulls the "Name" values out of PLUGIN_LUCIDE_ICONS. */
function mappedIconNames() {
  const block = gameUtils.slice(
    gameUtils.indexOf('PLUGIN_LUCIDE_ICONS'),
    gameUtils.indexOf('}', gameUtils.indexOf('PLUGIN_LUCIDE_ICONS')),
  )
  return [...block.matchAll(/:\s*'([A-Za-z0-9]+)'/g)].map((match) => match[1])
}

/** Pulls the identifiers registered in ICON_REGISTRY. */
function registeredIconNames() {
  const start = pluginIcon.indexOf('const ICON_REGISTRY')
  const block = pluginIcon.slice(start, pluginIcon.indexOf('\n}', start))
  return [...block.matchAll(/^\s{2}([A-Z][A-Za-z0-9]*),/gm)].map((match) => match[1])
}

test('every mapped plugin icon is actually registered', () => {
  // HardDrive and HardDriveUpload were mapped but never registered, so both
  // save-sync plugins silently rendered the generic puzzle icon. Nothing
  // caught it because the fallback is valid code.
  const registered = new Set(registeredIconNames())
  const mapped = mappedIconNames()

  assert.ok(mapped.length > 0, 'no icon mappings were parsed; the guard would pass vacuously')
  for (const iconName of mapped) {
    assert.ok(registered.has(iconName), `${iconName} is mapped to a plugin but missing from ICON_REGISTRY`)
  }
})

test('the local folder source is registered everywhere it has to be', () => {
  assert.match(displayText, /'game-source-local': 'Local Folder'/)
  assert.match(gameUtils, /'game-source-local':\s*'FolderOpen'/)
  // Without this the connection gets no include-path normalization in the UI.
  assert.match(gameUtils, /isFilesystemSourcePlugin[\s\S]{0,220}game-source-local/)
  // And without this its cards show a blank configuration summary.
  assert.match(gameUtils, /'game-source-local':\s*\(c\)\s*=>\s*\{/)
})

test('the local folder summary reads from base_path, not root_path', () => {
  // sourcescope deletes root_path for every filesystem-backed plugin, so a
  // summary built from it would always be empty.
  const strategy = gameUtils.slice(
    gameUtils.indexOf("'game-source-local': (c) => {"),
    gameUtils.indexOf("'game-source-google-drive': (c) => summarizeDriveIncludePaths"),
  )
  assert.match(strategy, /c\.base_path/)
  assert.doesNotMatch(strategy, /c\.root_path/)
})
