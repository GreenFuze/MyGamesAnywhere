import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, Download, Laptop, LoaderCircle, Power, Settings } from 'lucide-react'
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
import { Button } from '@/components/ui/button'
import { useProfiles } from '@/hooks/useProfiles'
import { cn } from '@/lib/utils'

const connectedStates = new Set<DeviceEndpoint['status']>(['ready', 'busy', 'update_required', 'error'])

class ClientEndpointAssociation {
  static key(profileID: string) {
    return `mga.clientEndpoint.${profileID}`
  }

  static get(profileID: string): string {
    try {
      return localStorage.getItem(this.key(profileID)) ?? ''
    } catch {
      return ''
    }
  }

  static set(profileID: string, endpointID: string) {
    try {
      localStorage.setItem(this.key(profileID), endpointID)
    } catch {
      // Association is a convenience; live server state remains authoritative.
    }
  }
}

export function ClientStatusControl() {
  const { currentProfile } = useProfiles()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const menuRef = useRef<HTMLDivElement | null>(null)
  const [open, setOpen] = useState(false)
  const [pendingLaunchID, setPendingLaunchID] = useState('')
  const [launchStartedAt, setLaunchStartedAt] = useState(0)
  const [associationRevision, setAssociationRevision] = useState(0)
  const [confirmStop, setConfirmStop] = useState(false)

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
  const associatedID = useMemo(
    () => profileID ? ClientEndpointAssociation.get(profileID) : '',
    [profileID, associationRevision],
  )
  const associated = devices.find((device) => device.id === associatedID)
    ?? (devices.length === 1 ? devices[0] : undefined)
  const connected = Boolean(associated && connectedStates.has(associated.status))
  const onlineCount = devices.filter((device) => connectedStates.has(device.status)).length

  const connect = useMutation({
    mutationFn: async () => {
      if (devices.length === 0) {
        return { kind: 'pair' as const, pairing: await createDevicePairingChallenge() }
      }
      return { kind: 'launch' as const, launch: await createDeviceClientLaunch() }
    },
    onSuccess: (result) => {
      if (result.kind === 'pair') {
        if (!result.pairing.pair_uri) throw new Error('MGA Server did not return a client pairing URI')
        window.location.href = result.pairing.pair_uri
        return
      }
      if (!result.launch.launch_uri) throw new Error('MGA Server did not return a client launch URI')
      setPendingLaunchID(result.launch.id)
      setLaunchStartedAt(Date.now())
      window.location.href = result.launch.launch_uri
    },
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
    if (!currentProfile || launch?.status !== 'acknowledged' || !launch.endpoint_id) return
    ClientEndpointAssociation.set(currentProfile.id, launch.endpoint_id)
    setAssociationRevision((revision) => revision + 1)
    setPendingLaunchID('')
    void queryClient.invalidateQueries({ queryKey: ['devices', currentProfile.id] })
  }, [currentProfile, launchQuery.data, queryClient])

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

  const presentation = statusPresentation(deviceAuthority, associated, connected, onlineCount)
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
                <Button className="w-full" onClick={() => connect.mutate()} disabled={connect.isPending || Boolean(pendingLaunchID) || devicesQuery.isLoading}>
                  {connect.isPending || pendingLaunchID ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Power className="h-4 w-4" />}
                  {pendingLaunchID ? 'Waiting…' : 'Connect'}
                </Button>
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

              {launchUnanswered || launchQuery.data?.status === 'expired' || connect.error || launchQuery.error ? (
                <div className="rounded-mga border border-amber-500/30 bg-amber-500/10 p-2.5 text-xs leading-5 text-mga-muted">
                  Client did not respond. Open it or pair it again.
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

function statusPresentation(authorized: boolean, endpoint: DeviceEndpoint | undefined, connected: boolean, onlineCount: number) {
  if (!authorized) return { label: 'Client unavailable', dot: 'bg-slate-500', border: 'border-slate-500/30', text: 'text-mga-muted' }
  if (!endpoint) {
    if (onlineCount > 0) return { label: `${onlineCount} clients online`, dot: 'bg-amber-400', border: 'border-amber-500/35', text: 'text-amber-300' }
    return { label: 'Connect client', dot: 'bg-red-400', border: 'border-red-500/35', text: 'text-red-300' }
  }
  if (!connected) return { label: 'Connect client', dot: 'bg-red-400', border: 'border-red-500/35', text: 'text-red-300' }
  if (endpoint.status === 'update_required') return { label: 'Client needs update', dot: 'bg-purple-400', border: 'border-purple-500/40', text: 'text-purple-300' }
  if (endpoint.status === 'error') return { label: 'Client error', dot: 'bg-red-400', border: 'border-red-500/40', text: 'text-red-300' }
  if (endpoint.status === 'busy') return { label: 'Client busy', dot: 'bg-amber-400', border: 'border-amber-500/40', text: 'text-amber-300' }
  return { label: 'Client ready', dot: 'bg-emerald-400', border: 'border-emerald-500/35', text: 'text-emerald-300' }
}
