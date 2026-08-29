import type { RouteObject } from 'react-router'

export const APP_ROUTES = {
  root: '/',
  credentialSetup: '/credential-setup',
  overview: 'overview',
  profiles: 'profiles',
  catalog: 'catalog',
  sources: 'sources',
  artifacts: 'artifacts',
  system: 'system',
  achievements: 'achievements',
  library: 'library',
  // Compatibility paths remain matchable only so the shell can redirect them
  // to management surfaces. They are not active product destinations.
  play: 'play',
  playSection: 'play/section/:sectionId',
  librarySection: 'library/section/:sectionId',
  libraryReview: 'library/review',
  stats: 'stats',
  statsLibrary: 'stats/library',
  statsGamer: 'stats/gamer',
  playableLegacy: 'playable',
  xcloudLegacy: 'xcloud',
  settings: 'settings',
  about: 'about',
  gamePlay: '/game/:id/play',
  gameMedia: '/game/:id/media',
  gameDetail: '/game/:id',
  fallback: '*',
} as const

export const APP_DESTINATIONS = {
  overview: '/overview',
  library: '/library',
  system: '/system',
  play: '/library',
  statsLibrary: '/stats/library',
} as const

export const APP_ROUTE_MATCHERS: RouteObject[] = [
  { path: APP_ROUTES.credentialSetup },
  {
    path: APP_ROUTES.root,
    children: [
      { index: true },
      { path: APP_ROUTES.overview },
      { path: APP_ROUTES.profiles },
      { path: APP_ROUTES.catalog },
      { path: APP_ROUTES.sources },
      { path: APP_ROUTES.artifacts },
      { path: APP_ROUTES.system },
      { path: APP_ROUTES.play },
      { path: APP_ROUTES.playSection },
      { path: APP_ROUTES.library },
      { path: APP_ROUTES.librarySection },
      { path: APP_ROUTES.libraryReview },
      { path: APP_ROUTES.achievements },
      { path: APP_ROUTES.stats },
      { path: APP_ROUTES.statsLibrary },
      { path: APP_ROUTES.statsGamer },
      { path: APP_ROUTES.playableLegacy },
      { path: APP_ROUTES.xcloudLegacy },
      { path: APP_ROUTES.settings },
      { path: APP_ROUTES.about },
    ],
  },
  { path: APP_ROUTES.gamePlay },
  { path: APP_ROUTES.gameMedia },
  { path: APP_ROUTES.gameDetail },
  { path: APP_ROUTES.fallback },
]

export const MANAGEMENT_DESTINATIONS = [
  { id: 'overview', label: 'Overview', path: APP_DESTINATIONS.overview, description: 'Operational health and attention' },
  { id: 'profiles', label: 'Profiles', path: '/profiles', description: 'Identity, roles, and policy' },
  { id: 'library', label: 'Library', path: APP_DESTINATIONS.library, description: 'Managed games and metadata' },
  { id: 'catalog', label: 'Catalog', path: '/catalog', description: 'Offers, versions, and availability' },
  { id: 'sources', label: 'Sources', path: '/sources', description: 'Storefront and provider sync' },
  { id: 'artifacts', label: 'Artifacts', path: '/artifacts', description: 'Emulators and runtime compliance' },
  { id: 'achievements', label: 'Achievements', path: '/achievements', description: 'Normalized progress and sync' },
  { id: 'system', label: 'System', path: APP_DESTINATIONS.system, description: 'Server, API clients, and recovery' },
] as const

export function isCredentialSetupPath(pathname: string): boolean {
  return pathname === APP_ROUTES.credentialSetup
}

export const SETTINGS_TAB_IDS = [
  'my-settings',
  'integrations',
  'devices',
  'emulators',
  'profiles',
  'cache',
  'appearance',
  'update',
  'plugins',
] as const

export type SettingsTabId = (typeof SETTINGS_TAB_IDS)[number]

const PLAYER_SETTINGS_TAB_IDS = new Set<SettingsTabId>([
  'my-settings',
  'profiles',
  'devices',
  'emulators',
  'appearance',
])

export interface SettingsRouteResolution {
  activeTab: SettingsTabId
  redirectTo?: string
}

export function resolveSettingsRoute(
  searchParams: URLSearchParams,
  isAdmin: boolean,
): SettingsRouteResolution {
  const tabParam = searchParams.get('tab')
  if (tabParam === 'duplicates') {
    return { activeTab: isAdmin ? 'integrations' : 'my-settings', redirectTo: '/library/review?tab=copies' }
  }
  if (tabParam === 'undetected') {
    return { activeTab: isAdmin ? 'integrations' : 'my-settings', redirectTo: '/library/review?tab=identify' }
  }

  const normalizedTab = tabParam === 'settings' ? 'update' : tabParam
  const availableTabs = isAdmin
    ? new Set<string>(SETTINGS_TAB_IDS)
    : PLAYER_SETTINGS_TAB_IDS
  const fallbackTab: SettingsTabId = isAdmin ? 'integrations' : 'my-settings'
  const activeTab = normalizedTab && availableTabs.has(normalizedTab)
    ? normalizedTab as SettingsTabId
    : fallbackTab

  return { activeTab }
}
