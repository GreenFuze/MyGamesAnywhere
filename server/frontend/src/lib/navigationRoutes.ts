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
  // Paths from the retired player shell that named a management surface stay
  // matchable so the shell can redirect them and existing bookmarks survive.
  // Play, game-launch and browser-runtime paths are gone: they fall through to
  // the catch-all rather than being redirected to somewhere that suggests MGA
  // still runs games.
  librarySection: 'library/section/:sectionId',
  libraryReview: 'library/review',
  stats: 'stats',
  settings: 'settings',
  about: 'about',
  fallback: '*',
} as const

export const APP_DESTINATIONS = {
  overview: '/overview',
  library: '/library',
  system: '/system',
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
      { path: APP_ROUTES.library },
      { path: APP_ROUTES.librarySection },
      { path: APP_ROUTES.libraryReview },
      { path: APP_ROUTES.achievements },
      { path: APP_ROUTES.stats },
      { path: APP_ROUTES.settings },
      { path: APP_ROUTES.about },
    ],
  },
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
