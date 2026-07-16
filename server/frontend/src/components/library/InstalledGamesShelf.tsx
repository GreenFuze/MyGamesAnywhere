import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, LoaderCircle, Monitor } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import {
  getInstalledGames,
  launchGameOnDevice,
  listDeviceCommands,
  listDevices,
  type DeviceCommand,
  type InstalledGameItem,
} from '@/api/client'
import { HorizontalGameShelf } from '@/components/library/HorizontalGameShelf'
import type { GameCardPrimaryAction } from '@/components/library/GameCard'
import { Button } from '@/components/ui/button'
import { useClientEndpointAssociation } from '@/hooks/useClientEndpointAssociation'
import { useProfiles } from '@/hooks/useProfiles'
import { resolveInstalledGameAction } from '@/lib/installedGameAction'

const terminalCommandStates = new Set<DeviceCommand['status']>([
  'succeeded',
  'failed',
  'rejected',
  'canceled',
  'expired',
])

type ActiveLaunch = {
  endpointID: string
  commandID: string
  gameID: string
  title: string
}

export function InstalledGamesShelf() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { currentProfile } = useProfiles()
  const profileID = currentProfile?.id ?? ''
  const [activeLaunch, setActiveLaunch] = useState<ActiveLaunch | null>(null)
  const [notice, setNotice] = useState('')

  const devicesQuery = useQuery({
    queryKey: ['devices', profileID],
    queryFn: listDevices,
    enabled: Boolean(profileID),
    retry: false,
    refetchInterval: 3000,
  })
  const devices = devicesQuery.data ?? []
  const { associated, associatedID, requiresSelection, selectEndpoint } = useClientEndpointAssociation(profileID, devices)
  const installedQuery = useQuery({
    queryKey: ['installed-games', profileID, associatedID],
    queryFn: () => getInstalledGames(associatedID),
    enabled: Boolean(profileID && associatedID),
    retry: false,
    refetchInterval: 5000,
  })
  const commandQuery = useQuery({
    queryKey: ['device-commands', activeLaunch?.endpointID],
    queryFn: () => listDeviceCommands(activeLaunch!.endpointID),
    enabled: Boolean(activeLaunch),
    retry: false,
    refetchInterval: (query) => {
      const command = query.state.data?.find((item) => item.id === activeLaunch?.commandID)
      return command && terminalCommandStates.has(command.status) ? false : 750
    },
  })
  const trackedCommand = commandQuery.data?.find((item) => item.id === activeLaunch?.commandID)

  const launch = useMutation({
    mutationFn: (item: InstalledGameItem) => {
      if (!associatedID) throw new Error('Choose a device before starting the game.')
      return launchGameOnDevice(associatedID, item.game.id, item.source_game_id)
    },
    onMutate: () => setNotice(''),
    onSuccess: (command, item) => setActiveLaunch({
      endpointID: associatedID,
      commandID: command.id,
      gameID: item.game.id,
      title: item.game.title,
    }),
    onError: (error) => setNotice(error instanceof Error ? error.message : 'MGA couldn’t start the game.'),
  })

  useEffect(() => {
    if (!activeLaunch || !trackedCommand || !terminalCommandStates.has(trackedCommand.status)) return
    if (trackedCommand.status === 'succeeded') {
      setNotice(`${activeLaunch.title} started on ${installedQuery.data?.device.display_name ?? associated?.display_name ?? 'the device'}.`)
    } else {
      setNotice(trackedCommand.error_message || 'MGA couldn’t start the game on this device.')
    }
    setActiveLaunch(null)
    void queryClient.invalidateQueries({ queryKey: ['installed-games', profileID, associatedID] })
    void queryClient.invalidateQueries({ queryKey: ['devices', profileID] })
  }, [activeLaunch, associated?.display_name, associatedID, installedQuery.data?.device.display_name, profileID, queryClient, trackedCommand])

  const itemsByGameID = useMemo(
    () => new Map((installedQuery.data?.games ?? []).map((item) => [item.game.id, item])),
    [installedQuery.data?.games],
  )
  const device = installedQuery.data?.device
  const deviceName = device?.display_name ?? associated?.display_name
  const attentionCount = installedQuery.data?.attention_count ?? 0
  const games = installedQuery.data?.games.map((item) => item.game) ?? []

  const openDeviceControls = () => {
    if (!associatedID) return
    navigate(`/settings?tab=devices&device=${encodeURIComponent(associatedID)}`)
  }
  const openLaunchChoice = (item: InstalledGameItem) => {
    navigate({ pathname: `/game/${encodeURIComponent(item.game.id)}`, hash: '#device-play' })
  }
  const primaryActionFor = (gameID: string): GameCardPrimaryAction | undefined => {
    const item = itemsByGameID.get(gameID)
    if (!item || !device) return undefined
    const action = resolveInstalledGameAction({
      launching: activeLaunch?.gameID === gameID,
      deviceStatus: device.status,
      connected: device.connected,
      accessLevel: device.access_level,
      launchSupported: item.launch_supported,
      launchTarget: item.launch_target ?? '',
      canPlay: item.can_play,
    })
    const onSelect = action.intent === 'launch'
      ? () => launch.mutate(item)
      : action.intent === 'details'
        ? () => openLaunchChoice(item)
        : action.intent === 'device'
          ? openDeviceControls
          : () => undefined
    return {
      label: action.label,
      onSelect,
      disabled: action.disabled || (action.intent === 'launch' && launch.isPending),
      kind: action.kind,
      title: action.intent === 'device' ? 'Open device controls' : undefined,
    }
  }

  return (
    <section className="space-y-3" aria-labelledby="installed-games-heading">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <Monitor className="h-5 w-5 shrink-0 text-mga-accent" aria-hidden="true" />
          <h2 id="installed-games-heading" className="truncate text-2xl font-semibold tracking-tight text-mga-text">
            Installed Games{deviceName ? ` · ${deviceName}` : ''}
          </h2>
          {installedQuery.data ? <span className="text-sm text-mga-muted">{games.length}</span> : null}
        </div>
        {devices.length > 1 ? (
          <label className="flex items-center gap-2 text-sm text-mga-muted">
            <span>Device</span>
            <select
              value={associatedID}
              onChange={(event) => selectEndpoint(event.target.value)}
              className="max-w-64 rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-mga-text"
              aria-label="Device for installed games"
            >
              <option value="" disabled>Choose a device</option>
              {devices.map((current) => (
                <option key={current.id} value={current.id}>{current.display_name} · {current.os_user}</option>
              ))}
            </select>
          </label>
        ) : null}
      </div>

      {attentionCount > 0 && associatedID ? (
        <Link
          to={`/settings?tab=devices&device=${encodeURIComponent(associatedID)}`}
          className="inline-flex items-center gap-2 text-sm font-medium text-amber-300 hover:text-amber-200"
        >
          <AlertTriangle className="h-4 w-4" />
          {attentionCount} {attentionCount === 1 ? 'game needs' : 'games need'} attention
        </Link>
      ) : null}
      {notice ? (
        <p className="rounded-mga border border-mga-border bg-mga-surface px-3 py-2 text-sm text-mga-text" role="status">
          {notice}
        </p>
      ) : null}

      {devicesQuery.isPending ? (
        <ShelfMessage><LoaderCircle className="h-4 w-4 animate-spin" /> Checking devices…</ShelfMessage>
      ) : devicesQuery.isError ? (
        <ShelfMessage>
          MGA couldn’t check installed games on this device.
          <Button size="sm" variant="outline" onClick={() => devicesQuery.refetch()}>Retry</Button>
        </ShelfMessage>
      ) : devices.length === 0 ? (
        <ShelfMessage>Connect MGA Client to see installed games.</ShelfMessage>
      ) : requiresSelection ? (
        <ShelfMessage>Choose a device to see installed games.</ShelfMessage>
      ) : installedQuery.isPending ? (
        <ShelfMessage><LoaderCircle className="h-4 w-4 animate-spin" /> Checking installed games…</ShelfMessage>
      ) : installedQuery.isError ? (
        <ShelfMessage>
          MGA couldn’t check installed games on this device.
          <Button size="sm" variant="outline" onClick={() => installedQuery.refetch()}>Retry</Button>
        </ShelfMessage>
      ) : games.length === 0 ? (
        <ShelfMessage>No games installed on this device.</ShelfMessage>
      ) : (
        <>
          {device && !device.connected ? (
            <p className="text-sm text-mga-muted">This device is offline. Its last known installed games are still shown.</p>
          ) : null}
          <HorizontalGameShelf
            games={games}
            label={`Installed Games${deviceName ? ` on ${deviceName}` : ''}`}
            cardVariant="play"
            renderPrimaryAction={(game) => primaryActionFor(game.id)}
          />
        </>
      )}
    </section>
  )
}

function ShelfMessage({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-28 flex-wrap items-center justify-center gap-3 rounded-[18px] border border-white/[0.05] bg-[#09070d] px-5 py-6 text-sm text-mga-muted">
      {children}
    </div>
  )
}
