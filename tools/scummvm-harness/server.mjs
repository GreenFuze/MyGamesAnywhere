import http from 'node:http'
import { createReadStream } from 'node:fs'
import { promises as fs } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const repoRoot = path.resolve(__dirname, '..', '..')
const runtimeRoot = path.join(repoRoot, 'server', 'frontend', 'public', 'runtimes', 'scummvm')
const harnessRoot = __dirname
const gameRoot =
  process.env.SCUMMVM_HARNESS_GAME_ROOT ?? '\\\\tv2\\Games\\ScummVM\\Island of Dr. Brain (Floppy DOS)'
const host = process.env.SCUMMVM_HARNESS_HOST ?? '127.0.0.1'
const port = Number.parseInt(process.env.SCUMMVM_HARNESS_PORT ?? '43123', 10)

const contentTypes = new Map([
  ['.html', 'text/html; charset=utf-8'],
  ['.js', 'text/javascript; charset=utf-8'],
  ['.json', 'application/json; charset=utf-8'],
  ['.css', 'text/css; charset=utf-8'],
  ['.wasm', 'application/wasm'],
  ['.dat', 'application/octet-stream'],
  ['.zip', 'application/zip'],
  ['.so', 'application/octet-stream'],
  ['.scr', 'application/octet-stream'],
  ['.hep', 'application/octet-stream'],
  ['.map', 'application/octet-stream'],
  ['.drv', 'application/octet-stream'],
  ['.cfg', 'application/octet-stream'],
  ['.aud', 'application/octet-stream'],
  ['.msg', 'application/octet-stream'],
  ['.v56', 'application/octet-stream'],
  ['.exe', 'application/octet-stream'],
  ['.bat', 'text/plain; charset=utf-8'],
  ['.txt', 'text/plain; charset=utf-8'],
  ['.hlp', 'application/octet-stream'],
])

function normalizePlayPath(value) {
  return value.replaceAll('\\', '/').replace(/^\.\/+/, '').replace(/^\/+/, '')
}

function commonDirectoryPath(paths) {
  const normalized = paths
    .map((currentPath) => normalizePlayPath(currentPath))
    .filter((currentPath) => currentPath.length > 0)
    .map((currentPath) => currentPath.split('/').slice(0, -1))

  if (normalized.length === 0) return ''

  const prefix = [...normalized[0]]
  for (let index = 1; index < normalized.length; index += 1) {
    let shared = 0
    while (
      shared < prefix.length &&
      shared < normalized[index].length &&
      prefix[shared] === normalized[index][shared]
    ) {
      shared += 1
    }
    prefix.length = shared
    if (prefix.length === 0) break
  }

  return prefix.join('/')
}

function inferScummvmEnginePlugin(files) {
  const names = new Set(
    files.map((file) => normalizePlayPath(file.path).split('/').pop()?.toLowerCase() ?? '').filter(Boolean),
  )
  const hasAny = (...candidates) => candidates.some((candidate) => names.has(candidate))
  const hasPrefix = (prefix, suffix = '') =>
    Array.from(names).some((name) => name.startsWith(prefix) && name.endsWith(suffix))
  const hasExtension = (extension) => Array.from(names).some((name) => name.endsWith(extension))

  if (names.has('resource.map') && hasAny('resource.000', 'resource.001')) {
    return 'libsci.so'
  }
  if (
    names.has('monster.sou') ||
    (names.has('atlantis.000') && names.has('atlantis.001')) ||
    names.has('comi.la0') ||
    hasPrefix('monkey.')
  ) {
    return 'libscumm.so'
  }
  if ((names.has('english.smp') && names.has('english.idx')) || (names.has('dw.scn') && hasExtension('.scn'))) {
    return 'libtinsel.so'
  }
  if (names.has('hd.blb') && hasAny('a.blb', 'c.blb', 't.blb')) {
    return 'libneverhood.so'
  }
  if (hasAny('intro.stk', 'gobnew.lic')) {
    return 'libgob.so'
  }
  if (hasAny('dragon.res', 'dragon.sph')) {
    return 'libdragons.so'
  }
  if ((hasPrefix('bb', '.000') || hasPrefix('game', '.vnm')) && hasAny('bbloogie.000', 'bbtennis.000')) {
    return 'libbbvs.so'
  }
  if (names.has('sky.dnr') && names.has('sky.dsk')) {
    return 'libsky.so'
  }
  if (names.has('queen.1') && names.has('queen.1c')) {
    return 'libqueen.so'
  }
  if (names.has('toon.dat')) {
    return 'libtoon.so'
  }
  if (names.has('touche.dat')) {
    return 'libtouche.so'
  }
  if (names.has('drascula.dat')) {
    return 'libdrascula.so'
  }
  if (names.has('lure.dat')) {
    return 'liblure.so'
  }

  return null
}

function sendJson(res, statusCode, payload) {
  const body = JSON.stringify(payload, null, 2)
  res.writeHead(statusCode, {
    'Content-Type': 'application/json; charset=utf-8',
    'Content-Length': Buffer.byteLength(body),
    'Cache-Control': 'no-store',
  })
  res.end(body)
}

function sendText(res, statusCode, text) {
  res.writeHead(statusCode, {
    'Content-Type': 'text/plain; charset=utf-8',
    'Content-Length': Buffer.byteLength(text),
    'Cache-Control': 'no-store',
  })
  res.end(text)
}

function decodeSafePathname(rawPathname) {
  const trimmed = rawPathname.replace(/^\/+/, '')
  if (trimmed.length === 0) {
    return []
  }

  return trimmed.split('/').map((segment) => {
    const decoded = decodeURIComponent(segment)
    if (!decoded || decoded === '.' || decoded === '..') {
      throw new Error(`Illegal path segment: ${segment}`)
    }
    return decoded
  })
}

function safeJoin(rootPath, rawPathname) {
  const segments = decodeSafePathname(rawPathname)
  const resolved = path.resolve(rootPath, ...segments)
  const relative = path.relative(rootPath, resolved)
  if (relative.startsWith('..') || path.isAbsolute(relative)) {
    throw new Error(`Path escapes root: ${rawPathname}`)
  }
  return resolved
}

async function buildManifest() {
  const files = []

  async function walk(currentAbsolute, currentRelative = '') {
    const entries = await fs.readdir(currentAbsolute, { withFileTypes: true })
    entries.sort((left, right) => left.name.localeCompare(right.name))

    for (const entry of entries) {
      const absolutePath = path.join(currentAbsolute, entry.name)
      const relativePath = currentRelative
        ? `${currentRelative}/${entry.name}`
        : entry.name

      if (entry.isDirectory()) {
        await walk(absolutePath, relativePath)
        continue
      }

      const stat = await fs.stat(absolutePath)
      files.push({
        path: relativePath.replaceAll('\\', '/'),
        size: stat.size,
        url: `/game/${relativePath.split(path.sep).map(encodeURIComponent).join('/')}`,
      })
    }
  }

  await walk(gameRoot)

  return {
    title: path.basename(gameRoot),
    launchDirectoryPath: commonDirectoryPath(files.map((file) => file.path)),
    enginePluginFileName: inferScummvmEnginePlugin(files) ?? undefined,
    files,
  }
}

async function serveFile(req, res, absolutePath) {
  const stat = await fs.stat(absolutePath)
  if (!stat.isFile()) {
    sendText(res, 404, 'Not found')
    return
  }

  const extension = path.extname(absolutePath).toLowerCase()
  const contentType = contentTypes.get(extension) ?? 'application/octet-stream'
  const headers = {
    'Content-Type': contentType,
    'Accept-Ranges': 'bytes',
    'Cache-Control': 'no-store',
  }

  let start = 0
  let end = stat.size - 1
  let statusCode = 200
  const rangeHeader = req.headers.range

  if (typeof rangeHeader === 'string') {
    const match = /^bytes=(\d*)-(\d*)$/.exec(rangeHeader.trim())
    if (!match) {
      res.writeHead(416, {
        'Content-Range': `bytes */${stat.size}`,
        'Cache-Control': 'no-store',
      })
      res.end()
      return
    }

    if (match[1]) {
      start = Number.parseInt(match[1], 10)
    }
    if (match[2]) {
      end = Number.parseInt(match[2], 10)
    }

    if (Number.isNaN(start) || Number.isNaN(end) || start > end || start >= stat.size) {
      res.writeHead(416, {
        'Content-Range': `bytes */${stat.size}`,
        'Cache-Control': 'no-store',
      })
      res.end()
      return
    }

    end = Math.min(end, stat.size - 1)
    statusCode = 206
    headers['Content-Range'] = `bytes ${start}-${end}/${stat.size}`
  }

  headers['Content-Length'] = String(end - start + 1)
  res.writeHead(statusCode, headers)
  createReadStream(absolutePath, { start, end }).pipe(res)
}

const server = http.createServer(async (req, res) => {
  try {
    const requestUrl = new URL(req.url ?? '/', `http://${host}:${port}`)
    const pathname = requestUrl.pathname

    if (pathname === '/healthz') {
      sendJson(res, 200, {
        ok: true,
        runtimeRoot,
        gameRoot,
      })
      return
    }

    if (pathname === '/manifest') {
      const manifest = await buildManifest()
      sendJson(res, 200, manifest)
      return
    }

    if (pathname === '/' || pathname === '/index.html') {
      await serveFile(req, res, path.join(harnessRoot, 'index.html'))
      return
    }

    if (pathname === '/mga-player-launch.html') {
      await serveFile(req, res, path.join(harnessRoot, 'mga-player-launch.html'))
      return
    }

    if (pathname.startsWith('/runtime/')) {
      const absolutePath = safeJoin(runtimeRoot, pathname.slice('/runtime/'.length))
      await serveFile(req, res, absolutePath)
      return
    }

    if (pathname.startsWith('/game/')) {
      const absolutePath = safeJoin(gameRoot, pathname.slice('/game/'.length))
      await serveFile(req, res, absolutePath)
      return
    }

    sendText(res, 404, `Unknown route: ${pathname}`)
  } catch (error) {
    console.error(error)
    sendText(res, 500, error instanceof Error ? error.stack ?? error.message : String(error))
  }
})

server.listen(port, host, async () => {
  const manifest = await buildManifest()
  console.log(`ScummVM harness listening on http://${host}:${port}`)
  console.log(`Runtime root: ${runtimeRoot}`)
  console.log(`Game root: ${gameRoot}`)
  console.log(`Manifest files: ${manifest.files.length}`)
})
