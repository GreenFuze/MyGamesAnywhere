import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, Download, Laptop, LoaderCircle, Power, Settings, Shield } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import {
  createDeviceClientLaunch,
  createDevicePairingChallenge,
  dispatchDeviceCommand,
  getAuthSession,
  getCredentialStatus,
  getDeviceClientDownload,
  getDeviceClientLaunch,
  listDevices,
  type DeviceEndpoint,
} from '@/api/client'
import { Button, buttonVariants } from '@/components/ui/button'
import { useBrowserClientAssociation } from '@/hooks/useBrowserClientAssociation'
import { useProfiles } from '@/hooks/useProfiles'
import {
  findNewConnectedEndpointIDs,
  resolveNewlyPairedEndpointID,
} from '@/lib/browserClientAssociation'
import { resolveClientStatusPresentation } from '@/lib/clientStatusPresentation'
import { cn } from '@/lib/utils'

const connectedStates = new Set<DeviceEndpoint['status']>(['ready', 'busy', 'update_required', 'error'])

export function ClientStatusControl() {
  const { currentProfile } = useProfiles()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const menuRef = useRef<HTMLDivElement | null>(null)
  const [open, setOpen] = useState(false)
  const [pendingLaunchID, setPendingLaunchID] = useState('')
  const [launchStartedAt, setLaunchStartedAt] = useState(0)
  const [launchMessage, setLaunchMessage] = useState('')
  const [confirmStop, setConfirmStop] = useState(false)
  const [pairingAttempt, setPairingAttempt] = useState<{
    baselineEndpointIDs: string[]
    startedAt: number
  } | null>(null)

  const profileID = currentProfile?.id ?? ''
  const sessionQuery = useQuery({
    queryKey: ['auth-session'],
    queryFn: getAuthSession,
    retry: false,
    enabled: Boolean(profileID),
  })
  const authorized = Boolean(
    currentProfile
      && sessionQuery.data?.authenticated
      && sessionQuery.data.profile?.id === currentProfile.id
      && !sessionQuery.data.must_change,
  )
  const credentialQuery = useQuery({
    queryKey: ['credential-status', profileID],
    queryFn: getCredentialStatus,
    enabled: Boolean(profileID),
    retry: false,
  })
  const deviceAuthority = authorized && Boolean(credentialQuery.data?.configured)
  const devicesQuery = useQuery({
    queryKey: ['devices', profileID],
    queryFn: listDevices,
    enabled: deviceAuthority,
    refetchInterval: 3000,
    retry: false,
  })
  const downloadQuery = useQuery({
    queryKey: ['device-client-download'],
    queryFn: getDeviceClientDownload,
    enabled: open,
  })
  const launchQuery = useQuery({
    queryKey: ['device-client-launch', pendingLaunchID],
    queryFn: () => getDeviceClientLaunch(pendingLaunchID),
    enabled: Boolean(pendingLaunchID) && deviceAuthority,
    refetchInterval: (query) => query.state.data?.status === 'waiting' ? 750 : false,
    retry: false,
  })

  const devices = devicesQuery.data ?? []
  const {
    associated,
    selectEndpoint,
    storedID: browserEndpointID,
  } = useBrowserClientAssociation(devices)
  const connected = Boolean(associated && connectedStates.has(associated.status))
  const requiresPairing = Boolean(browserEndpointID && !associated) || devices.length === 0

  const preparedClientActionQuery = useQuery({
    queryKey: ['device-client-actions', profileID, requiresPairing ? 'pair' : 'launch'],
    queryFn: async () => {
      const pairing = await createDevicePairingChallenge()
      if (!pairing.pair_uri) throw new Error('MGA Server did not return a client pairing URI')
      if (requiresPairing) {
        return { pairing }
      }
      const [standard, elevated] = await Promise.all([
        createDeviceClientLaunch('standard'),
        createDeviceClientLaunch('elevated'),
      ])
      if (!standard.launch_uri || !elevated.launch_uri) throw new Error('MGA Server did not return MGA Client launch links')
      return { pairing, standard, elevated }
    },
    enabled: open && deviceAuthority && devicesQuery.isSuccess && !connected && !pendingLaunchID && !pairingAttempt,
    retry: false,
    staleTime: 30_000,
  })
  const stop = useMutation({
    mutationFn: () => {
      if (!associated) throw new Error('No local MGA Client endpoint is selected')
      return dispatchDeviceCommand(associated.id, 'endpoint.stop')
    },
    onSuccess: async () => {
      setConfirmStop(false)
      await queryClient.invalidateQueries({ queryKey: ['devices', profileID] })
    },
  })

  useEffect(() => {
    const launch = launchQuery.data
    if (launch?.status === 'expired') {
      setLaunchMessage('MGA Client did not open. Try again.')
      setPendingLaunchID('')
      return
    }
    if (!currentProfile || launch?.status !== 'acknowledged' || !launch.endpoint_id) return
    selectEndpoint(launch.endpoint_id)
    setPendingLaunchID('')
    setLaunchMessage('')
    void queryClient.invalidateQueries({ queryKey: ['devices', currentProfile.id] })
  }, [currentProfile, launchQuery.data, queryClient, selectEndpoint])

  useEffect(() => {
    if (!pairingAttempt) return
    const candidates = findNewConnectedEndpointIDs(pairingAttempt.baselineEndpointIDs, devices)
    if (candidates.length > 1) {
      setPairingAttempt(null)
      setLaunchMessage('More than one new client appeared. Try pairing again.')
      return
    }
    const endpointID = resolveNewlyPairedEndpointID(pairingAttempt.baselineEndpointIDs, devices)
    if (!endpointID) return
    selectEndpoint(endpointID)
    setPairingAttempt(null)
    setLaunchMessage('')
  }, [devices, pairingAttempt, selectEndpoint])

  useEffect(() => {
    if (!pairingAttempt) return
    const remaining = Math.max(0, 120_000 - (Date.now() - pairingAttempt.startedAt))
    const timeout = window.setTimeout(() => {
      setPairingAttempt(null)
      setLaunchMessage('Pairing expired. Try again.')
    }, remaining)
    return () => window.clearTimeout(timeout)
  }, [pairingAttempt])

  useEffect(() => {
    if (!open) return
    const close = (event: PointerEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) {
        setOpen(false)
        setConfirmStop(false)
      }
    }
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false)
        setConfirmStop(false)
      }
    }
    document.addEventListener('pointerdown', close)
    document.addEventListener('keydown', escape)
    return () => {
      document.removeEventListener('pointerdown', close)
      document.removeEventListener('keydown', escape)
    }
  }, [open])

  const presentation = resolveClientStatusPresentation(deviceAuthority, associated, connected)
  const canStop = Boolean(
    associated
      && connected
      && ['manage', 'owner'].includes(associated.access_level)
      && associated.capabilities.includes('endpoint.stop'),
  )
  const launchUnanswered = Boolean(
    pendingLaunchID
      && launchQuery.data?.status === 'waiting'
      && launchStartedAt
      && Date.now() - launchStartedAt > 4000,
  )
  const preparedClientAction = preparedClientActionQuery.data
  const beginLaunch = (launchID: string) => {
    setPendingLaunchID(launchID)
    setLaunchStartedAt(Date.now())
    setLaunchMessage('')
  }
  const beginPairing = () => {
    setPairingAttempt({
      baselineEndpointIDs: devices.map((device) => device.id),
      startedAt: Date.now(),
    })
    setLaunchMessage('')
  }
  const retryLaunch = () => {
    setPendingLaunchID('')
    setPairingAttempt(null)
    setLaunchStartedAt(0)
    setLaunchMessage('')
    void preparedClientActionQuery.refetch()
  }

  return (
    <div ref={menuRef} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        className={cn(
          'flex items-center gap-2 rounded-mga border bg-mga-bg px-2.5 py-1.5 text-sm font-semibold transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent',
          presentation.border,
          presentation.text,
        )}
        aria-expanded={open}
        aria-label={`${presentation.label}. Open MGA Client controls`}
      >
        <span className={cn('h-2.5 w-2.5 rounded-full', presentation.dot)} aria-hidden="true" />
        <span className="hidden md:inline">{presentation.label}</span>
        <span className="md:hidden">Client</span>
        <ChevronDown className="h-3.5 w-3.5" />
      </button>

      {open ? (
        <div className="absolute right-0 top-[calc(100%+0.5rem)] z-50 w-80 rounded-mga border border-mga-border bg-mga-surface p-3 shadow-2xl">
          <div className="flex items-start gap-3">
            <div className="grid h-10 w-10 shrink-0 place-items-center rounded-mga bg-mga-bg">
              <Laptop className="h-5 w-5 text-mga-accent" />
            </div>
            <div className="min-w-0">
              <div className="font-bold text-mga-text">{presentation.label}</div>
              <p className="mt-0.5 text-xs leading-5 text-mga-muted">
                {associated
                  ? `${associated.display_name} (${associated.os_user})`
                  : browserEndpointID
                    ? 'Pair this Windows user with the active profile to continue.'
                  : deviceAuthority
                    ? 'Open or pair MGA Client for this Windows user.'
                    : authorized
                      ? 'Add a password or PIN to this profile before controlling devices.'
                      : 'Sign in to a protected MGA profile to control its clients.'}
              </p>
            </div>
          </div>

          {deviceAuthority ? (
            <div className="mt-3 space-y-2">
              {!connected ? (
                pendingLaunchID || pairingAttempt ? (
                  <Button className="w-full" disabled><LoaderCircle className="h-4 w-4 animate-spin" /> Waiting…</Button>
                ) : preparedClientAction && !preparedClientAction.standard ? (
                  <a className={cn(buttonVariants(), 'w-full')} href={preparedClientAction.pairing.pair_uri} onClick={beginPairing}>
                    <Power className="h-4 w-4" /> Pair MGA Client
                  </a>
                ) : preparedClientAction?.standard ? (
                  <a className={cn(buttonVariants(), 'w-full')} href={preparedClientAction.standard.launch_uri} onClick={() => beginLaunch(preparedClientAction.standard.id)}>
                    <Power className="h-4 w-4" /> Run MGA Client
                  </a>
                ) : (
                  <Button className="w-full" disabled><LoaderCircle className="h-4 w-4 animate-spin" /> Preparing…</Button>
                )
              ) : null}

              {!connected && !requiresPairing ? (
                preparedClientAction?.elevated && !pendingLaunchID ? (
                  <a className={cn(buttonVariants({ variant: 'outline' }), 'w-full')} href={preparedClientAction.elevated.launch_uri} onClick={() => beginLaunch(preparedClientAction.elevated.id)}>
                    <Shield className="h-4 w-4" /> Run MGA Client as administrator
                  </a>
                ) : (
                  <Button variant="outline" className="w-full" disabled><Shield className="h-4 w-4" /> Run MGA Client as administrator</Button>
                )
              ) : null}

              {!connected && !requiresPairing && preparedClientAction?.pairing && !pendingLaunchID && !pairingAttempt ? (
                <a className={cn(buttonVariants({ variant: 'outline' }), 'w-full')} href={preparedClientAction.pairing.pair_uri} onClick={beginPairing}>
                  <Laptop className="h-4 w-4" /> Pair this Windows user
                </a>
              ) : null}

              {!connected ? (
                <Button
                  variant="outline"
                  className="w-full"
                  disabled={!downloadQuery.data?.download_url}
                  onClick={() => downloadQuery.data?.download_url && window.open(downloadQuery.data.download_url, '_blank', 'noopener,noreferrer')}
                >
                  <Download className="h-4 w-4" /> Download
                </Button>
              ) : null}

              {canStop && !confirmStop ? (
                <Button variant="outline" className="w-full" onClick={() => setConfirmStop(true)}>
                  <Power className="h-4 w-4" /> Stop
                </Button>
              ) : null}
              {canStop && confirmStop ? (
                <div className="rounded-mga border border-red-500/35 bg-red-500/10 p-2.5">
                  <p className="text-xs leading-5 text-mga-muted">Stop MGA Client on this device?</p>
                  <div className="mt-2 flex gap-2">
                    <Button variant="outline" size="sm" className="flex-1 border-red-500/40 text-red-300 hover:bg-red-500/15" onClick={() => stop.mutate()} disabled={stop.isPending}>
                      Confirm stop
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => setConfirmStop(false)}>Cancel</Button>
                  </div>
                </div>
              ) : null}

              {launchUnanswered || launchMessage || preparedClientActionQuery.error || launchQuery.error ? (
                <div className="rounded-mga border border-amber-500/30 bg-amber-500/10 p-2.5 text-xs leading-5 text-mga-muted">
                  <p>{launchMessage || 'MGA Client did not respond. Open it or try again.'}</p>
                  {pendingLaunchID || pairingAttempt ? <Button variant="outline" size="sm" className="mt-2 w-full" onClick={retryLaunch}>Try again</Button> : null}
                </div>
              ) : null}
              {stop.error ? <p className="text-xs text-red-300">{stop.error instanceof Error ? stop.error.message : 'Could not stop MGA Client.'}</p> : null}
              <Button
                variant="ghost"
                size="sm"
                className="w-full"
                onClick={() => {
                  setOpen(false)
                  navigate('/settings?tab=devices')
                }}
              >
                <Settings className="h-4 w-4" /> Manage devices
              </Button>
            </div>
          ) : authorized ? (
            <Button
              variant="outline"
              size="sm"
              className="mt-3 w-full"
              onClick={() => {
                setOpen(false)
                navigate('/settings?tab=profiles')
              }}
            >
              Manage profile security
            </Button>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
