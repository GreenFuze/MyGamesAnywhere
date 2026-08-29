import { useLocation, useNavigate } from 'react-router'
import { type GameDetailResponse } from '@/api/client'
import { browserPlaySourceOptionLabel, listBrowserPlaySelections } from '@/lib/browserPlay'
import {
  GameCardActionResolver,
  type GameCardActionResolution,
  type GameCardPlayRoute,
  type GameCardPrimaryAction,
} from '@/lib/gameCardActions'
import { buildGameRouteState } from '@/lib/gameNavigation'
import { launchOptionVersionContext } from '@/lib/sourceCapabilities'

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
