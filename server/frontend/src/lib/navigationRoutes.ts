import type { RouteObject } from 'react-router'

export const APP_ROUTES = {
  root: '/',
  credentialSetup: '/credential-setup',
  play: 'play',
  playSection: 'play/section/:sectionId',
  library: 'library',
  librarySection: 'library/section/:sectionId',
  libraryReview: 'library/review',
  achievements: 'achievements',
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
  play: '/play',
  statsLibrary: '/stats/library',
} as const

export const APP_ROUTE_MATCHERS: RouteObject[] = [
  { path: APP_ROUTES.credentialSetup },
  {
    path: APP_ROUTES.root,
    children: [
      { index: true },
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
