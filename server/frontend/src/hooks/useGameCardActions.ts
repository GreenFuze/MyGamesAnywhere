import { useLocation, useNavigate } from 'react-router'
import {
  launchEmulatorGameOnDevice,
  launchGameOnDevice,
  type GameDetailResponse,
} from '@/api/client'
import { useToast } from '@/components/ui/toast'
import { browserPlaySourceOptionLabel, listBrowserPlaySelections } from '@/lib/browserPlay'
import {
  GameCardActionResolver,
  type GameCardActionResolution,
  type GameCardPlayRoute,
  type GameCardPrimaryAction,
} from '@/lib/gameCardActions'
import { buildGameRouteState } from '@/lib/gameNavigation'
import { launchOptionVersionContext, sourceVersionContext } from '@/lib/sourceCapabilities'

interface UseGameCardActionsOptions {
  primaryAction?: GameCardPrimaryAction
  alternateActions?: GameCardPrimaryAction[]
  preferredPlayRoute?: GameCardPlayRoute
}

/**
 * Builds the same complete play-route menu for cards, shelves, and table rows.
 */
export function useGameCardActions(
  game: GameDetailResponse,
  options: UseGameCardActionsOptions = {},
): GameCardActionResolution {
  const { notify } = useToast()
  const navigate = useNavigate()
  const location = useLocation()
  const routeState = buildGameRouteState(location.pathname, location.search)
  const details = () => {
    navigate(`/game/${encodeURIComponent(game.id)}`, { state: routeState })
  }
  const derivedActions: GameCardPrimaryAction[] = []
  const browserSelections = listBrowserPlaySelections(game)

  for (const selection of browserSelections) {
    const context = browserPlaySourceOptionLabel(selection, browserSelections)
    derivedActions.push({
      id: `browser:${selection.sourceGame.id}`,
      label: browserSelections.length > 1 ? `Play in browser · ${context}` : 'Play in browser',
      kind: 'play',
      route: 'browser',
      title: `Play in browser · ${context}`,
      onSelect: () => navigate(
        `/game/${encodeURIComponent(game.id)}/play?source=${encodeURIComponent(selection.sourceGame.id)}`,
        { state: routeState },
      ),
    })
  }

  const xcloudOptions = (game.play?.options ?? []).filter(
    (option) => option.kind === 'xcloud' && option.launchable && Boolean(option.url),
  )
  for (const option of xcloudOptions) {
    const context = launchOptionVersionContext(option, game.source_games)
    derivedActions.push({
      id: `xcloud:${option.source_game_id}:${option.url}`,
      label: xcloudOptions.length > 1 ? `Play in xCloud · ${context}` : 'Play in xCloud',
      kind: 'play',
      route: 'cloud',
      title: context ? `Play in xCloud · ${context}` : 'Play in xCloud',
      onSelect: () => window.open(option.url, '_blank', 'noopener,noreferrer'),
    })
  }
  if (xcloudOptions.length === 0 && game.xcloud_url) {
    derivedActions.push({
      id: 'xcloud',
      label: 'Play in xCloud',
      kind: 'play',
      route: 'cloud',
      onSelect: () => window.open(game.xcloud_url, '_blank', 'noopener,noreferrer'),
    })
  }

  const installedRoutes = (game.devices ?? []).filter(
    (device) =>
      device.connected
      && device.can_play
      && device.installed
      && device.launch_supported
      && Boolean(device.launch_target)
      && Boolean(device.installed_source_id),
  )
  for (const device of installedRoutes) {
    const source = game.source_games.find((candidate) => candidate.id === device.installed_source_id)
    const context = source ? sourceVersionContext(source) : device.installed_source_id!
    derivedActions.push({
      id: `installed:${device.device_id}:${device.installed_source_id}`,
      label: `Play on ${device.display_name}${installedRoutes.length > 1 ? ` · ${context}` : ''}`,
      kind: 'play',
      route: 'local',
      title: `Start ${context} on ${device.display_name}`,
      onSelect: () => {
        void launchGameOnDevice(device.device_id, game.id, device.installed_source_id!)
          .then(() => notify({
            title: `Starting ${game.title}`,
            description: `${context} on ${device.display_name}`,
            tone: 'success',
          }))
          .catch((error: unknown) => notify({
            title: `Could not start ${game.title}`,
            description: error instanceof Error ? error.message : 'MGA Client rejected the play request.',
            tone: 'error',
          }))
      },
    })
  }

  const emulatorRoutes = (game.devices ?? [])
    .flatMap((device) => (device.emulator_routes ?? []).map((route) => ({ device, route })))
    .filter(({ device, route }) => device.connected && device.can_play && route.state === 'ready')
    .sort((left, right) => {
      const defaultOrder = Number(right.route.default) - Number(left.route.default)
      if (defaultOrder !== 0) return defaultOrder
      const deviceOrder = left.device.display_name.localeCompare(right.device.display_name)
      if (deviceOrder !== 0) return deviceOrder
      return left.route.emulator_name.localeCompare(right.route.emulator_name)
    })
  for (const { device, route } of emulatorRoutes) {
    const source = game.source_games.find((candidate) => candidate.id === route.source_game_id)
    const versionLabel = source ? sourceVersionContext(source) : route.source_title
    derivedActions.push({
      id: `emulator:${device.device_id}:${route.emulator_id}:${route.source_game_id}`,
      label: `Play on ${device.display_name} · ${route.emulator_name}${emulatorRoutes.length > 1 ? ` · ${versionLabel}` : ''}`,
      kind: 'play',
      route: 'emulator',
      title: route.reason || `Start this copy with ${route.emulator_name} on ${device.display_name}`,
      onSelect: () => {
        void launchEmulatorGameOnDevice(device.device_id, game.id, route.source_game_id, route.emulator_id)
          .then(() => notify({
            title: `Starting ${game.title}`,
            description: `${route.emulator_name} on ${device.display_name}`,
            tone: 'success',
          }))
          .catch((error: unknown) => notify({
            title: `Could not start ${game.title}`,
            description: error instanceof Error ? error.message : 'MGA Client rejected the play request.',
            tone: 'error',
          }))
      },
    })
  }

  return new GameCardActionResolver({
    primaryAction: options.primaryAction,
    alternateActions: options.alternateActions ?? [],
    derivedActions,
    preferredPlayRoute: options.preferredPlayRoute,
    fallbackAction: {
      id: 'details',
      label: 'Details',
      kind: 'details',
      onSelect: details,
    },
  }).resolve()
}
