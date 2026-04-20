import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

import { chromium } from 'playwright'

import { startProofServer } from './proof-server.mjs'

function parseArgs(argv) {
  const options = {}
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index]
    if (arg === '--output' && argv[index + 1]) {
      options.outputDir = argv[index + 1]
      index += 1
    }
  }
  return options
}

async function sendBridgeCommand(page, command) {
  return page.evaluate(
    ({ nextCommand }) =>
      new Promise((resolve, reject) => {
        const iframe = document.querySelector('iframe')
        const target = iframe?.contentWindow
        if (!target) {
          reject(new Error('Browser player iframe is not available.'))
          return
        }

        const timeoutId = window.setTimeout(() => {
          window.removeEventListener('message', onMessage)
          reject(new Error(`Timed out waiting for bridge response to ${nextCommand.type}.`))
        }, 20000)

        function onMessage(event) {
          if (event.source !== target) return
          const message = event.data
          if (!message || typeof message !== 'object') return
          if (message.requestId !== nextCommand.requestId) return
          window.clearTimeout(timeoutId)
          window.removeEventListener('message', onMessage)
          resolve(message)
        }

        window.addEventListener('message', onMessage)
        target.postMessage(nextCommand, window.location.origin)
      }),
    { nextCommand: command },
  )
}

async function waitForRuntimeReady(page) {
  await page.locator('button:has-text("Save")').waitFor({ state: 'visible', timeout: 20000 })
  await page.waitForFunction(() => {
    const saveButton = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Save',
    )
    return Boolean(saveButton && !saveButton.disabled)
  })
}

async function waitForIframeCondition(page, predicate) {
  await page.waitForFunction(
    (source) => {
      const iframe = document.querySelector('iframe')
      const targetWindow = iframe?.contentWindow
      if (!targetWindow) return false
      const check = new Function('frameWindow', source)
      return Boolean(check(targetWindow))
    },
    predicate,
    { timeout: 20000 },
  )
}

async function waitForUnsupportedMessage(page, expected) {
  await page.waitForFunction(
    (message) => document.body.textContent?.includes(message),
    expected,
    { timeout: 20000 },
  )
}

function makeRequestId(label) {
  return `${label}-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

async function clickAndWait(page, buttonLabel, expectedText) {
  await page.getByRole('button', { name: buttonLabel }).click()
  try {
    await page.waitForFunction(
      (message) => document.body.textContent?.includes(message),
      expectedText,
      { timeout: 20000 },
    )
  } catch (error) {
    const bodyText = await page.locator('body').innerText()
    throw new Error(`${error instanceof Error ? error.message : String(error)}\nVisible page text:\n${bodyText}`)
  }
}

async function wait(ms) {
  await new Promise((resolve) => setTimeout(resolve, ms))
}

function writeMarkdownReport(results) {
  const lines = ['# Browser-Play E2E Proof', '']
  for (const result of results) {
    lines.push(`## ${result.name}`)
    lines.push(`- Status: ${result.status}`)
    lines.push(`- Detail: ${result.detail}`)
    lines.push('')
  }
  return `${lines.join('\n')}\n`
}

async function runEmulatorJsProof(page, baseUrl, slotStore) {
  await page.goto(`${baseUrl}/game/proof-emulatorjs/play`, { waitUntil: 'networkidle' })
  await waitForRuntimeReady(page)
  await waitForIframeCondition(
    page,
    'return frameWindow.EJS_emulator && frameWindow.EJS_emulator.gameManager && frameWindow.EJS_emulator.gameManager.FS',
  )

  const initialSnapshot = {
    files: [{ path: '/proof-state.sav', base64: 'RU1VTEFUT1JKUy1TVEFURS1B' }],
  }
  const overwriteSnapshot = {
    files: [{ path: '/proof-state.sav', base64: 'RU1VTEFUT1JKUy1TVEFURS1C' }],
  }

  const importResult = await sendBridgeCommand(page, {
    type: 'import-save-snapshot',
    requestId: makeRequestId('emulatorjs-import-initial'),
    files: initialSnapshot.files,
  })
  if (!importResult.ok) {
    throw new Error(importResult.error || 'Initial EmulatorJS import failed.')
  }

  await clickAndWait(page, 'Save', 'Saved autosave to the active integration.')

  const stored = slotStore.get('proof-emulatorjs', 'source-emu', 'emulatorjs', 'autosave')
  if (!stored) {
    throw new Error('EmulatorJS save slot was not stored by the proof server.')
  }

  const overwriteResult = await sendBridgeCommand(page, {
    type: 'import-save-snapshot',
    requestId: makeRequestId('emulatorjs-import-overwrite'),
    files: overwriteSnapshot.files,
  })
  if (!overwriteResult.ok) {
    throw new Error(overwriteResult.error || 'Overwrite EmulatorJS import failed.')
  }

  await clickAndWait(page, 'Load', 'Loaded autosave from the active integration.')

  const exported = await sendBridgeCommand(page, {
    type: 'export-save-snapshot',
    requestId: makeRequestId('emulatorjs-export-verify'),
  })
  const restored = exported.snapshot?.files?.[0]?.base64 ?? ''
  if (restored !== initialSnapshot.files[0].base64) {
    throw new Error(`EmulatorJS restore mismatch: ${restored}`)
  }

  return 'Save/Load restored the imported EmulatorJS snapshot.'
}

async function runJsdosBundleProof(page, baseUrl, slotStore) {
  await page.goto(`${baseUrl}/game/proof-jsdos-bundle/play`, { waitUntil: 'networkidle' })
  await waitForRuntimeReady(page)

  let initialBundleFile = null
  let initialBundle = ''
  for (let attempt = 0; attempt < 12; attempt += 1) {
    const exportedBeforeSave = await sendBridgeCommand(page, {
      type: 'export-save-snapshot',
      requestId: makeRequestId(`jsdos-export-before-save-${attempt}`),
    })
    initialBundleFile = exportedBeforeSave.snapshot?.files?.[0] ?? null
    initialBundle = initialBundleFile?.base64 ?? ''
    if (initialBundleFile) {
      break
    }
    await wait(1000)
  }
  if (!initialBundleFile) {
    throw new Error('js-dos bundle export never produced a snapshot file.')
  }

  await clickAndWait(page, 'Save', 'Saved autosave to the active integration.')

  const stored = slotStore.get('proof-jsdos-bundle', 'source-dos-bundle', 'jsdos', 'autosave')
  if (!stored) {
    throw new Error('js-dos bundle save slot was not stored by the proof server.')
  }

  await clickAndWait(page, 'Load', 'Loaded autosave from the active integration.')

  const exportedAfterLoad = await sendBridgeCommand(page, {
    type: 'export-save-snapshot',
    requestId: makeRequestId('jsdos-export-after-load'),
  })
  const restoredBundle = exportedAfterLoad.snapshot?.files?.[0]?.base64 ?? ''
  if (restoredBundle !== initialBundle) {
    throw new Error('js-dos bundle snapshot changed after save/load proof.')
  }

  return 'Bundle-backed js-dos launch completed save/export/import round-trip through the player UI.'
}

async function runJsdosPlainProof(page, baseUrl) {
  await page.goto(`${baseUrl}/game/proof-jsdos-plain/play`, { waitUntil: 'networkidle' })
  await waitForUnsupportedMessage(
    page,
    'This launch does not support save import/export. js-dos save sync requires a bundle-backed session.',
  )

  const saveDisabled = await page.getByRole('button', { name: 'Save' }).isDisabled()
  const loadDisabled = await page.getByRole('button', { name: 'Load' }).isDisabled()
  if (!saveDisabled || !loadDisabled) {
    throw new Error('Plain-file js-dos launch left Save/Load enabled.')
  }

  return 'Plain-file js-dos launch failed early with the explicit unsupported-state message.'
}

async function runScummvmProof(page, baseUrl, slotStore) {
  await page.goto(`${baseUrl}/game/proof-scummvm/play`, { waitUntil: 'networkidle' })
  await waitForRuntimeReady(page)
  await waitForIframeCondition(
    page,
    'return typeof frameWindow.FS === "object" && frameWindow.FS && typeof frameWindow.FS.syncfs === "function"',
  )

  const initialSnapshot = {
    files: [{ path: 'proof-slot.s00', base64: 'U0NVTU1WTS1TVEFURS1B' }],
  }
  const overwriteSnapshot = {
    files: [{ path: 'proof-slot.s00', base64: 'U0NVTU1WTS1TVEFURS1C' }],
  }

  const importInitial = await sendBridgeCommand(page, {
    type: 'import-save-snapshot',
    requestId: makeRequestId('scummvm-import-initial'),
    files: initialSnapshot.files,
  })
  if (!importInitial.ok) {
    throw new Error(importInitial.error || 'Initial ScummVM import failed.')
  }

  await clickAndWait(page, 'Save', 'Saved autosave to the active integration.')

  const stored = slotStore.get('proof-scummvm', 'source-scummvm', 'scummvm', 'autosave')
  if (!stored) {
    throw new Error('ScummVM save slot was not stored by the proof server.')
  }

  const importOverwrite = await sendBridgeCommand(page, {
    type: 'import-save-snapshot',
    requestId: makeRequestId('scummvm-import-overwrite'),
    files: overwriteSnapshot.files,
  })
  if (!importOverwrite.ok) {
    throw new Error(importOverwrite.error || 'Overwrite ScummVM import failed.')
  }

  await clickAndWait(page, 'Load', 'Loaded autosave from the active integration.')

  const exported = await sendBridgeCommand(page, {
    type: 'export-save-snapshot',
    requestId: makeRequestId('scummvm-export-verify'),
  })
  const restored = exported.snapshot?.files?.[0]?.base64 ?? ''
  if (restored !== initialSnapshot.files[0].base64) {
    throw new Error(`ScummVM restore mismatch: ${restored}`)
  }

  return 'ScummVM restored the imported save snapshot after a UI-driven save/load cycle.'
}

async function runAmbiguousSelectionProof(page, baseUrl) {
  await page.goto(`${baseUrl}/game/proof-browser-ambiguity/play`, { waitUntil: 'networkidle' })
  await waitForUnsupportedMessage(
    page,
    'Multiple browser-play sources or versions are available. Choose one before launching.',
  )

  const optionTexts = (await page.locator('select option').allTextContents())
    .map((value) => value.trim())
    .filter(Boolean)
    .filter((value) => !value.startsWith('Choose a source'))

  if (optionTexts.length < 2) {
    throw new Error(`Expected at least two source options, found: ${optionTexts.join(', ')}`)
  }
  if (new Set(optionTexts).size !== optionTexts.length) {
    throw new Error(`Expected distinct source labels, found duplicates: ${optionTexts.join(', ')}`)
  }
  if (!optionTexts.some((value) => value.includes('Proof/Version A'))) {
    throw new Error(`Expected a Version A option label, found: ${optionTexts.join(', ')}`)
  }
  if (!optionTexts.some((value) => value.includes('Proof/Version B'))) {
    throw new Error(`Expected a Version B option label, found: ${optionTexts.join(', ')}`)
  }

  return 'Ambiguous browser-play sources required explicit choice and rendered distinct option labels.'
}

async function runInvalidRememberedSourceProof(page, baseUrl) {
  await page.goto(baseUrl, { waitUntil: 'networkidle' })
  await page.evaluate(() => {
    window.localStorage.setItem(
      'mga.browserPlaySource.proof-invalid-remembered.jsdos',
      'missing-source-record',
    )
  })

  await page.goto(`${baseUrl}/game/proof-invalid-remembered/play`, { waitUntil: 'networkidle' })
  await waitForUnsupportedMessage(
    page,
    'The remembered browser-play source is no longer available. Choose a current source before launching.',
  )

  const sourceSelect = page.locator('select').first()
  await sourceSelect.waitFor({ state: 'visible', timeout: 20000 })
  await sourceSelect.selectOption('source-remembered-current')
  await page.getByRole('button', { name: 'Apply Source' }).click()
  await waitForRuntimeReady(page)

  return 'Invalid remembered source blocked auto-launch until the user explicitly chose the current source.'
}

async function main() {
  const options = parseArgs(process.argv.slice(2))
  const scriptDir = path.dirname(fileURLToPath(import.meta.url))
  const workspaceRoot = path.resolve(scriptDir, '..', '..')
  const distDir = path.join(workspaceRoot, 'server', 'frontend', 'dist')
  const outputDir = options.outputDir
    ? path.resolve(options.outputDir)
    : path.join(scriptDir, 'proof-output', new Date().toISOString().replace(/[:.]/g, '-'))

  const server = await startProofServer({ workspaceRoot, distDir })
  const browser = await chromium.launch({ headless: true })
  const context = await browser.newContext()
  const page = await context.newPage()
  page.on('dialog', (dialog) => dialog.accept())
  const results = []

  try {
    for (const [name, runner] of [
      ['EmulatorJS', () => runEmulatorJsProof(page, server.baseUrl, server.slotStore)],
      ['js-dos bundle-backed', () => runJsdosBundleProof(page, server.baseUrl, server.slotStore)],
      ['js-dos plain-file unsupported', () => runJsdosPlainProof(page, server.baseUrl)],
      ['Browser source ambiguity', () => runAmbiguousSelectionProof(page, server.baseUrl)],
      ['Invalid remembered source', () => runInvalidRememberedSourceProof(page, server.baseUrl)],
      ['ScummVM', () => runScummvmProof(page, server.baseUrl, server.slotStore)],
    ]) {
      try {
        const detail = await runner()
        results.push({ name, status: 'passed', detail })
      } catch (error) {
        results.push({
          name,
          status: 'failed',
          detail: error instanceof Error ? error.message : String(error),
        })
        throw error
      }
    }
  } finally {
    await mkdir(outputDir, { recursive: true })
    await writeFile(path.join(outputDir, 'proof-report.md'), writeMarkdownReport(results), 'utf8')
    await page.close()
    await context.close()
    await browser.close()
    await server.close()
  }

  process.stdout.write(`Browser-play proof completed successfully.\nOutput: ${outputDir}\n`)
}

main().catch((error) => {
  const message = error instanceof Error ? error.stack ?? error.message : String(error)
  process.stderr.write(`${message}\n`)
  process.exitCode = 1
})
