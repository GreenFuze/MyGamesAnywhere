import { createHash } from 'node:crypto'
import http from 'node:http'
import { readFile } from 'node:fs/promises'
import path from 'node:path'

function toIsoNow() {
  return new Date().toISOString()
}

function sha256Hex(value) {
  return createHash('sha256').update(value).digest('hex')
}

function bytesToBase64(bytes) {
  return Buffer.from(bytes).toString('base64')
}

function json(response, statusCode, value) {
  response.writeHead(statusCode, {
    'Content-Type': 'application/json; charset=utf-8',
    'Cache-Control': 'no-store',
  })
  response.end(JSON.stringify(value))
}

function text(response, statusCode, value) {
  response.writeHead(statusCode, {
    'Content-Type': 'text/plain; charset=utf-8',
    'Cache-Control': 'no-store',
  })
  response.end(value)
}

function contentTypeFor(filePath) {
  const extension = path.extname(filePath).toLowerCase()
  switch (extension) {
    case '.html':
      return 'text/html; charset=utf-8'
    case '.js':
    case '.mjs':
      return 'text/javascript; charset=utf-8'
    case '.css':
      return 'text/css; charset=utf-8'
    case '.json':
      return 'application/json; charset=utf-8'
    case '.wasm':
      return 'application/wasm'
    case '.png':
      return 'image/png'
    case '.svg':
      return 'image/svg+xml'
    case '.ico':
      return 'image/x-icon'
    case '.zip':
    case '.jsdos':
      return 'application/zip'
    case '.nes':
      return 'application/octet-stream'
    case '.exe':
      return 'application/vnd.microsoft.portable-executable'
    default:
      return 'application/octet-stream'
  }
}

function ensureRelativePath(urlPathname) {
  const normalized = path.posix.normalize(urlPathname)
  if (normalized.includes('..')) {
    return null
  }
  return normalized
}

function writeUint16(view, offset, value) {
  view.setUint16(offset, value, true)
}

function writeUint32(view, offset, value) {
  view.setUint32(offset, value >>> 0, true)
}

function crc32(bytes) {
  let crc = 0xffffffff
  for (let index = 0; index < bytes.length; index += 1) {
    crc ^= bytes[index]
    for (let bit = 0; bit < 8; bit += 1) {
      const mask = -(crc & 1)
      crc = (crc >>> 1) ^ (0xedb88320 & mask)
    }
  }
  return (crc ^ 0xffffffff) >>> 0
}

function concatBytes(chunks) {
  const total = chunks.reduce((sum, chunk) => sum + chunk.length, 0)
  const out = new Uint8Array(total)
  let offset = 0
  for (const chunk of chunks) {
    out.set(chunk, offset)
    offset += chunk.length
  }
  return out
}

function writeStoredZip(files) {
  const encoder = new TextEncoder()
  const localChunks = []
  const centralChunks = []
  let offset = 0

  for (const file of files) {
    const nameBytes = encoder.encode(file.path)
    const fileBytes = file.bytes
    const checksum = crc32(fileBytes)

    const localHeader = new Uint8Array(30 + nameBytes.length)
    const localView = new DataView(localHeader.buffer)
    writeUint32(localView, 0, 0x04034b50)
    writeUint16(localView, 4, 20)
    writeUint16(localView, 6, 0)
    writeUint16(localView, 8, 0)
    writeUint16(localView, 10, 0)
    writeUint16(localView, 12, 0)
    writeUint32(localView, 14, checksum)
    writeUint32(localView, 18, fileBytes.length)
    writeUint32(localView, 22, fileBytes.length)
    writeUint16(localView, 26, nameBytes.length)
    writeUint16(localView, 28, 0)
    localHeader.set(nameBytes, 30)
    localChunks.push(localHeader, fileBytes)

    const centralHeader = new Uint8Array(46 + nameBytes.length)
    const centralView = new DataView(centralHeader.buffer)
    writeUint32(centralView, 0, 0x02014b50)
    writeUint16(centralView, 4, 20)
    writeUint16(centralView, 6, 20)
    writeUint16(centralView, 8, 0)
    writeUint16(centralView, 10, 0)
    writeUint16(centralView, 12, 0)
    writeUint16(centralView, 14, 0)
    writeUint32(centralView, 16, checksum)
    writeUint32(centralView, 20, fileBytes.length)
    writeUint32(centralView, 24, fileBytes.length)
    writeUint16(centralView, 28, nameBytes.length)
    writeUint16(centralView, 30, 0)
    writeUint16(centralView, 32, 0)
    writeUint16(centralView, 34, 0)
    writeUint16(centralView, 36, 0)
    writeUint32(centralView, 38, 0)
    writeUint32(centralView, 42, offset)
    centralHeader.set(nameBytes, 46)
    centralChunks.push(centralHeader)

    offset += localHeader.length + fileBytes.length
  }

  const centralDirectory = concatBytes(centralChunks)
  const end = new Uint8Array(22)
  const endView = new DataView(end.buffer)
  writeUint32(endView, 0, 0x06054b50)
  writeUint16(endView, 4, 0)
  writeUint16(endView, 6, 0)
  writeUint16(endView, 8, files.length)
  writeUint16(endView, 10, files.length)
  writeUint32(endView, 12, centralDirectory.length)
  writeUint32(endView, 16, offset)
  writeUint16(endView, 20, 0)

  return concatBytes([...localChunks, centralDirectory, end])
}

function createNesProofRom() {
  const headerSize = 16
  const prgSize = 16 * 1024
  const chrSize = 8 * 1024
  const rom = new Uint8Array(headerSize + prgSize + chrSize)
  rom.set([0x4e, 0x45, 0x53, 0x1a, 0x01, 0x01, 0x00, 0x00], 0)

  const prgOffset = headerSize
  rom.set([0x78, 0xd8, 0x4c, 0x00, 0x80], prgOffset)
  rom[prgOffset + 0x3ffa] = 0x00
  rom[prgOffset + 0x3ffb] = 0x80
  rom[prgOffset + 0x3ffc] = 0x00
  rom[prgOffset + 0x3ffd] = 0x80
  rom[prgOffset + 0x3ffe] = 0x00
  rom[prgOffset + 0x3fff] = 0x80
  return rom
}

function createJsdosBundle() {
  const encoder = new TextEncoder()
  return writeStoredZip([
    {
      path: 'AUTOEXEC.BAT',
      bytes: encoder.encode(
        '@echo off\r\necho MGA BROWSER PLAY PROOF\r\necho proof-run>%TEMP%\\MGA-PROOF.TXT\r\necho bundle-state>PROOF.TXT\r\ndir\r\n',
      ),
    },
    {
      path: 'README.TXT',
      bytes: encoder.encode('MyGamesAnywhere browser-play proof fixture.\r\n'),
    },
  ])
}

function createPlainExeFixture() {
  return new Uint8Array([0x4d, 0x5a, 0x90, 0x00, 0x03, 0x00, 0x00, 0x00])
}

function createScummvmFixture() {
  return new TextEncoder().encode('Proof fixture for ScummVM browser-play save round-trip.\n')
}

function createGameFixtures() {
  const emulatorRom = createNesProofRom()
  const jsdosBundle = createJsdosBundle()
  const plainExe = createPlainExeFixture()
  const scummvmFile = createScummvmFixture()
  const createdAt = toIsoNow()

  const deliveryProfile = (profile, rootFileId) => ({
    profiles: [
      {
        profile,
        mode: 'direct',
        prepare_required: false,
        ready: true,
        root_file_id: rootFileId,
      },
    ],
  })

  const sourcePlay = (rootFileId) => ({
    launchable: true,
    root_file_id: rootFileId,
  })

  return {
    files: new Map([
      ['emu-root', { bytes: emulatorRom, contentType: 'application/octet-stream' }],
      ['dos-bundle-root', { bytes: jsdosBundle, contentType: 'application/zip' }],
      ['dos-plain-root', { bytes: plainExe, contentType: 'application/vnd.microsoft.portable-executable' }],
      ['scummvm-root', { bytes: scummvmFile, contentType: 'application/octet-stream' }],
    ]),
    games: new Map([
      [
        'proof-emulatorjs',
        {
          id: 'proof-emulatorjs',
          title: 'Proof EmulatorJS',
          platform: 'nes',
          kind: 'base_game',
          source_games: [
            {
              id: 'source-emu',
              integration_id: 'proof-source',
              plugin_id: 'game-source-local',
              external_id: 'proof-emu',
              raw_title: 'Proof EmulatorJS',
              platform: 'nes',
              kind: 'base_game',
              group_kind: 'self_contained',
              root_path: 'C:/Proof/EmulatorJS',
              status: 'found',
              created_at: createdAt,
              files: [
                {
                  id: 'emu-root',
                  path: 'proof.nes',
                  file_name: 'proof.nes',
                  role: 'root',
                  file_kind: 'rom',
                  size: emulatorRom.length,
                },
              ],
              delivery: deliveryProfile('browser.emulatorjs', 'emu-root'),
              play: sourcePlay('emu-root'),
              resolver_matches: [],
            },
          ],
        },
      ],
      [
        'proof-jsdos-bundle',
        {
          id: 'proof-jsdos-bundle',
          title: 'Proof js-dos Bundle',
          platform: 'ms_dos',
          kind: 'base_game',
          source_games: [
            {
              id: 'source-dos-bundle',
              integration_id: 'proof-source',
              plugin_id: 'game-source-local',
              external_id: 'proof-dos-bundle',
              raw_title: 'Proof js-dos Bundle',
              platform: 'ms_dos',
              kind: 'base_game',
              group_kind: 'self_contained',
              root_path: 'C:/Proof/jsdos',
              status: 'found',
              created_at: createdAt,
              files: [
                {
                  id: 'dos-bundle-root',
                  path: 'proof.zip',
                  file_name: 'proof.zip',
                  role: 'root',
                  file_kind: 'archive',
                  size: jsdosBundle.length,
                },
              ],
              delivery: deliveryProfile('browser.jsdos', 'dos-bundle-root'),
              play: sourcePlay('dos-bundle-root'),
              resolver_matches: [],
            },
          ],
        },
      ],
      [
        'proof-jsdos-plain',
        {
          id: 'proof-jsdos-plain',
          title: 'Proof js-dos Plain File',
          platform: 'ms_dos',
          kind: 'base_game',
          source_games: [
            {
              id: 'source-dos-plain',
              integration_id: 'proof-source',
              plugin_id: 'game-source-local',
              external_id: 'proof-dos-plain',
              raw_title: 'Proof js-dos Plain File',
              platform: 'ms_dos',
              kind: 'base_game',
              group_kind: 'self_contained',
              root_path: 'C:/Proof/jsdos-plain',
              status: 'found',
              created_at: createdAt,
              files: [
                {
                  id: 'dos-plain-root',
                  path: 'proof.exe',
                  file_name: 'proof.exe',
                  role: 'root',
                  file_kind: 'dos_executable',
                  size: plainExe.length,
                },
              ],
              delivery: deliveryProfile('browser.jsdos', 'dos-plain-root'),
              play: sourcePlay('dos-plain-root'),
              resolver_matches: [],
            },
          ],
        },
      ],
      [
        'proof-scummvm',
        {
          id: 'proof-scummvm',
          title: 'Proof ScummVM',
          platform: 'scummvm',
          kind: 'base_game',
          source_games: [
            {
              id: 'source-scummvm',
              integration_id: 'proof-source',
              plugin_id: 'game-source-local',
              external_id: 'proof-scummvm',
              raw_title: 'Proof ScummVM',
              platform: 'scummvm',
              kind: 'base_game',
              group_kind: 'self_contained',
              root_path: 'C:/Proof/ScummVM',
              status: 'found',
              created_at: createdAt,
              files: [
                {
                  id: 'scummvm-root',
                  path: 'proof/game.dat',
                  file_name: 'game.dat',
                  role: 'required',
                  file_kind: 'document',
                  size: scummvmFile.length,
                },
              ],
              delivery: deliveryProfile('browser.scummvm', 'scummvm-root'),
              play: sourcePlay('scummvm-root'),
              resolver_matches: [],
            },
          ],
        },
      ],
      [
        'proof-browser-ambiguity',
        {
          id: 'proof-browser-ambiguity',
          title: 'Proof Browser Ambiguity',
          platform: 'ms_dos',
          kind: 'base_game',
          source_games: [
            {
              id: 'source-ambiguous-a',
              integration_id: 'proof-source',
              integration_label: 'Proof Source',
              plugin_id: 'game-source-local',
              external_id: 'proof-ambiguous-a',
              raw_title: 'Proof Browser Ambiguity',
              platform: 'ms_dos',
              kind: 'base_game',
              group_kind: 'self_contained',
              root_path: 'Proof/Version A',
              status: 'found',
              created_at: createdAt,
              files: [
                {
                  id: 'dos-bundle-root',
                  path: 'proof.zip',
                  file_name: 'proof.zip',
                  role: 'root',
                  file_kind: 'archive',
                  size: jsdosBundle.length,
                },
              ],
              delivery: deliveryProfile('browser.jsdos', 'dos-bundle-root'),
              play: sourcePlay('dos-bundle-root'),
              resolver_matches: [],
            },
            {
              id: 'source-ambiguous-b',
              integration_id: 'proof-source',
              integration_label: 'Proof Source',
              plugin_id: 'game-source-local',
              external_id: 'proof-ambiguous-b',
              raw_title: 'Proof Browser Ambiguity',
              platform: 'ms_dos',
              kind: 'base_game',
              group_kind: 'self_contained',
              root_path: 'Proof/Version B',
              status: 'found',
              created_at: createdAt,
              files: [
                {
                  id: 'dos-bundle-root',
                  path: 'proof.zip',
                  file_name: 'proof.zip',
                  role: 'root',
                  file_kind: 'archive',
                  size: jsdosBundle.length,
                },
              ],
              delivery: deliveryProfile('browser.jsdos', 'dos-bundle-root'),
              play: sourcePlay('dos-bundle-root'),
              resolver_matches: [],
            },
          ],
        },
      ],
      [
        'proof-invalid-remembered',
        {
          id: 'proof-invalid-remembered',
          title: 'Proof Invalid Remembered Source',
          platform: 'ms_dos',
          kind: 'base_game',
          source_games: [
            {
              id: 'source-remembered-current',
              integration_id: 'proof-source',
              integration_label: 'Proof Source',
              plugin_id: 'game-source-local',
              external_id: 'proof-remembered-current',
              raw_title: 'Proof Invalid Remembered Source',
              platform: 'ms_dos',
              kind: 'base_game',
              group_kind: 'self_contained',
              root_path: 'Proof/Remembered/Current',
              status: 'found',
              created_at: createdAt,
              files: [
                {
                  id: 'dos-bundle-root',
                  path: 'proof.zip',
                  file_name: 'proof.zip',
                  role: 'root',
                  file_kind: 'archive',
                  size: jsdosBundle.length,
                },
              ],
              delivery: deliveryProfile('browser.jsdos', 'dos-bundle-root'),
              play: sourcePlay('dos-bundle-root'),
              resolver_matches: [],
            },
          ],
        },
      ],
    ]),
  }
}

function createSlotStore() {
  const slots = new Map()
  return {
    getKey(gameId, sourceGameId, runtime, slotId) {
      return `${gameId}:${sourceGameId}:${runtime}:${slotId}`
    },
    list(gameId, sourceGameId, runtime) {
      return ['autosave', 'slot-1', 'slot-2', 'slot-3', 'slot-4', 'slot-5'].map((slotId) => {
        const entry = slots.get(this.getKey(gameId, sourceGameId, runtime, slotId))
        if (!entry) {
          return { slot_id: slotId, exists: false, file_count: 0, total_size: 0 }
        }
        return entry.summary
      })
    },
    get(gameId, sourceGameId, runtime, slotId) {
      return slots.get(this.getKey(gameId, sourceGameId, runtime, slotId)) ?? null
    },
    put(gameId, sourceGameId, runtime, slotId, snapshot) {
      const manifestHash = sha256Hex(JSON.stringify(snapshot.files ?? []))
      const totalSize = (snapshot.files ?? []).reduce((sum, file) => sum + (file.size ?? 0), 0)
      const summary = {
        slot_id: slotId,
        exists: true,
        file_count: (snapshot.files ?? []).length,
        total_size: totalSize,
        manifest_hash: manifestHash,
        updated_at: toIsoNow(),
      }
      const stored = {
        summary,
        snapshot: {
          ...snapshot,
          manifest_hash: manifestHash,
        },
      }
      slots.set(this.getKey(gameId, sourceGameId, runtime, slotId), stored)
      return stored
    },
  }
}

export async function startProofServer({ workspaceRoot, distDir }) {
  const fixtures = createGameFixtures()
  const slotStore = createSlotStore()
  const indexHtmlPath = path.join(distDir, 'index.html')
  const indexHtml = await readFile(indexHtmlPath)

  const server = http.createServer(async (request, response) => {
    try {
      if (!request.url) {
        text(response, 400, 'Missing request URL')
        return
      }
      const url = new URL(request.url, 'http://127.0.0.1')
      const pathname = url.pathname

      if (pathname === '/api/config/frontend') {
        json(response, 200, { saveSyncActiveIntegrationId: 'proof-save-sync' })
        return
      }

      const gameMatch = pathname.match(/^\/api\/games\/([^/]+)$/)
      if (request.method === 'GET' && gameMatch) {
        const gameId = decodeURIComponent(gameMatch[1])
        const game = fixtures.games.get(gameId)
        if (!game) {
          text(response, 404, 'Game not found')
          return
        }
        json(response, 200, game)
        return
      }

      if (request.method === 'GET' && pathname === '/api/games/proof-emulatorjs/achievements') {
        json(response, 200, { sets: [] })
        return
      }
      if (request.method === 'GET' && pathname === '/api/games/proof-jsdos-bundle/achievements') {
        json(response, 200, { sets: [] })
        return
      }
      if (request.method === 'GET' && pathname === '/api/games/proof-jsdos-plain/achievements') {
        json(response, 200, { sets: [] })
        return
      }
      if (request.method === 'GET' && pathname === '/api/games/proof-scummvm/achievements') {
        json(response, 200, { sets: [] })
        return
      }

      const slotsMatch = pathname.match(/^\/api\/games\/([^/]+)\/save-sync\/slots$/)
      if (request.method === 'GET' && slotsMatch) {
        const gameId = decodeURIComponent(slotsMatch[1])
        const sourceGameId = url.searchParams.get('source_game_id') ?? ''
        const runtime = url.searchParams.get('runtime') ?? ''
        json(response, 200, { slots: slotStore.list(gameId, sourceGameId, runtime) })
        return
      }

      const slotMatch = pathname.match(/^\/api\/games\/([^/]+)\/save-sync\/slots\/([^/]+)$/)
      if (slotMatch && request.method === 'GET') {
        const gameId = decodeURIComponent(slotMatch[1])
        const slotId = decodeURIComponent(slotMatch[2])
        const sourceGameId = url.searchParams.get('source_game_id') ?? ''
        const runtime = url.searchParams.get('runtime') ?? ''
        const entry = slotStore.get(gameId, sourceGameId, runtime, slotId)
        if (!entry) {
          text(response, 404, 'Save slot not found')
          return
        }
        json(response, 200, entry.snapshot)
        return
      }

      if (slotMatch && request.method === 'PUT') {
        const gameId = decodeURIComponent(slotMatch[1])
        const slotId = decodeURIComponent(slotMatch[2])
        const chunks = []
        for await (const chunk of request) {
          chunks.push(chunk)
        }
        const payload = JSON.parse(Buffer.concat(chunks).toString('utf8'))
        const sourceGameId = payload.source_game_id ?? ''
        const runtime = payload.runtime ?? ''
        const stored = slotStore.put(gameId, sourceGameId, runtime, slotId, payload.snapshot ?? {})
        json(response, 200, { ok: true, summary: stored.summary })
        return
      }

      const playMatch = pathname.match(/^\/api\/games\/([^/]+)\/play$/)
      if (request.method === 'GET' && playMatch) {
        const fileId = url.searchParams.get('file_id') ?? ''
        const file = fixtures.files.get(fileId)
        if (!file) {
          text(response, 404, 'Fixture file not found')
          return
        }
        response.writeHead(200, {
          'Content-Type': file.contentType,
          'Cache-Control': 'no-store',
        })
        response.end(Buffer.from(file.bytes))
        return
      }

      const relativePath = ensureRelativePath(pathname === '/' ? '/index.html' : pathname)
      if (!relativePath) {
        text(response, 400, 'Invalid path')
        return
      }

      const distPath = path.join(distDir, relativePath)
      try {
        const file = await readFile(distPath)
        response.writeHead(200, {
          'Content-Type': contentTypeFor(distPath),
          'Cache-Control': 'no-store',
        })
        response.end(file)
        return
      } catch {
        response.writeHead(200, {
          'Content-Type': 'text/html; charset=utf-8',
          'Cache-Control': 'no-store',
        })
        response.end(indexHtml)
      }
    } catch (error) {
      const message = error instanceof Error ? error.stack ?? error.message : String(error)
      text(response, 500, message)
    }
  })

  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })

  const address = server.address()
  const port = typeof address === 'object' && address ? address.port : 0
  const baseUrl = `http://127.0.0.1:${port}`

  return {
    baseUrl,
    slotStore,
    close: () =>
      new Promise((resolve, reject) => {
        server.close((error) => {
          if (error) {
            reject(error)
            return
          }
          resolve()
        })
      }),
    workspaceRoot,
  }
}
