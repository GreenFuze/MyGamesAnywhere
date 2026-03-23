/** Same-origin in prod (SPA behind Go); Vite proxy in dev. */
const base = ''

export async function getJson<T>(path: string): Promise<T> {
  const res = await fetch(`${base}${path}`, {
    headers: { Accept: 'application/json' },
  })
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

export async function postJson<T>(path: string, body: unknown): Promise<T | void> {
  const res = await fetch(`${base}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`)
  }
  if (res.status === 204 || res.headers.get('content-length') === '0') {
    return
  }
  const text = await res.text()
  if (!text) return
  return JSON.parse(text) as T
}

export async function getHealth(): Promise<string> {
  const res = await fetch(`${base}/health`)
  if (!res.ok) throw new Error(`health: ${res.status}`)
  return res.text()
}

export type GameSummary = {
  id: string
  title: string
  platform: string
  kind: string
  is_game_pass?: boolean
  xcloud_available?: boolean
  store_product_id?: string
  xcloud_url?: string
}

export type ListGamesResponse = {
  games: GameSummary[]
}

export async function listGames(): Promise<ListGamesResponse> {
  return getJson<ListGamesResponse>('/api/games')
}

export type FrontendConfig = Record<string, unknown>

export async function getFrontendConfig(): Promise<FrontendConfig> {
  return getJson<FrontendConfig>('/api/config/frontend')
}

export async function setFrontendConfig(cfg: FrontendConfig): Promise<void> {
  await postJson('/api/config/frontend', cfg)
}
