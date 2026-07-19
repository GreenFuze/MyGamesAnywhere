import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearchParams } from 'react-router-dom'
import {
  Activity,
  AlertTriangle,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Clipboard,
  Download,
  HardDrive,
  KeyRound,
  Laptop,
  Plus,
  Save,
  Send,
  ShieldCheck,
  Trash2,
  UserRound,
} from 'lucide-react'
import {
  createDevicePairingChallenge,
  dispatchDeviceCommand,
  getAuthSession,
  getCredentialStatus,
  deleteDeviceGrant,
  getDeviceClientDownload,
	getEndpointInstallPreference,
  getInstallationValidationStatus,
  listDeviceCommands,
  listDeviceGrants,
  listDevices,
  renameDevice,
  revokeDevice,
  setDeviceGrant,
	setEndpointInstallPreference,
  setInstallationValidationSchedule,
  validateDeviceInstallations,
  type DeviceAccessLevel,
  type DeviceEndpoint,
  type DevicePairingChallenge,
  type InstallationValidationEndpointStatus,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { ActionMenu, type ActionMenuItem } from '@/components/ui/action-menu'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Tooltip } from '@/components/ui/tooltip'
import { InstallationValidationScheduleCard } from '@/components/settings/InstallationValidationScheduleCard'
import { useProfiles } from '@/hooks/useProfiles'
import { cn } from '@/lib/utils'
import { installationReasonLabel, validationStatusLabel } from '@/lib/installationValidation'

const statePresentation = {
  ready: { label: 'Ready', dot: 'bg-emerald-400', text: 'text-emerald-300', border: 'border-emerald-500/30' },
  busy: { label: 'Busy', dot: 'bg-amber-400', text: 'text-amber-300', border: 'border-amber-500/30' },
  offline: { label: 'Offline', dot: 'bg-slate-500', text: 'text-slate-300', border: 'border-slate-500/30' },
  update_required: { label: 'Update required', dot: 'bg-purple-400', text: 'text-purple-300', border: 'border-purple-500/40' },
  error: { label: 'Error', dot: 'bg-red-400', text: 'text-red-300', border: 'border-red-500/40' },
} as const

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / 1024 ** index).toFixed(index >= 3 ? 1 : 0)} ${units[index]}`
}

const ownershipStateLabel: Record<string, string> = {
  managed_here: 'Managed here',
  managed_elsewhere: 'Managed by another MGA server',
  released: 'Ready to pick up',
  installing_here: 'Installing from this server',
  installing_elsewhere: 'Installing from another server',
  legacy_unclaimed: 'Needs local ownership review',
  interrupted: 'Install was interrupted — cleanup needed',
}

function openOwnershipAction(action: 'release' | 'adopt', localInstallationID: string) {
  const query = new URLSearchParams({ server: window.location.origin, installation_id: localInstallationID })
  window.location.href = `mga://${action}?${query.toString()}`
}

export function DevicesTab() {
  const { currentProfile } = useProfiles()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const selectedDeviceID = searchParams.get('device')?.trim() ?? ''
  const [pairing, setPairing] = useState<DevicePairingChallenge | null>(null)
  const sessionQuery = useQuery({ queryKey: ['auth-session'], queryFn: getAuthSession, retry: false })
  const credentialQuery = useQuery({
    queryKey: ['credential-status', currentProfile?.id],
    queryFn: getCredentialStatus,
    enabled: Boolean(currentProfile),
  })
  const authorized = Boolean(
    currentProfile
      && sessionQuery.data?.authenticated
      && sessionQuery.data.profile?.id === currentProfile.id
      && !sessionQuery.data.must_change,
  )
  const devicesQuery = useQuery({
    queryKey: ['devices', currentProfile?.id],
    queryFn: listDevices,
    enabled: authorized,
    refetchInterval: 3000,
  })
  const downloadQuery = useQuery({ queryKey: ['device-client-download'], queryFn: getDeviceClientDownload })
  const validationQuery = useQuery({
    queryKey: ['installation-validation-schedule', currentProfile?.id],
    queryFn: getInstallationValidationStatus,
    enabled: authorized,
    refetchInterval: 3000,
  })
  const saveValidationSchedule = useMutation({
    mutationFn: setInstallationValidationSchedule,
    onSuccess: (status) => queryClient.setQueryData(['installation-validation-schedule', currentProfile?.id], status),
  })

  const createPairing = useMutation({
    mutationFn: createDevicePairingChallenge,
    onSuccess: setPairing,
  })

  if (!currentProfile) return null
  if (sessionQuery.isLoading || credentialQuery.isLoading) return <PanelMessage text="Checking device-management session…" />
  if (credentialQuery.data && !credentialQuery.data.configured) {
    return (
      <section className="mx-auto max-w-xl rounded-mga border border-mga-border bg-mga-surface p-6 shadow-xl">
        <KeyRound className="h-9 w-9 text-mga-accent" />
        <h2 className="mt-3 text-xl font-black text-mga-text">Protected profile required</h2>
        <p className="mt-2 text-sm leading-6 text-mga-muted">
          Device authority requires this profile to have a password or PIN. Profile credentials are managed only in Settings → Profiles.
        </p>
        <Button onClick={() => navigate('/settings?tab=profiles')} className="mt-5 w-full">
          <UserRound className="h-4 w-4" /> Open Profile Settings
        </Button>
      </section>
    )
  }
  if (!authorized) return <PanelMessage text="Sign out and sign in to this profile again to manage devices." />

  return (
    <div className="space-y-5">
      <section className="rounded-mga border border-mga-border bg-mga-surface p-5 shadow-lg">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.2em] text-mga-accent">
              <Activity className="h-4 w-4" /> Connected devices
            </div>
            <h2 className="mt-2 text-xl font-black text-mga-text">Devices</h2>
            <p className="mt-1 max-w-2xl text-sm leading-6 text-mga-muted">
              Each Windows user connects separately, even when they share the same PC.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              onClick={() => downloadQuery.data?.download_url && window.open(downloadQuery.data.download_url, '_blank', 'noopener,noreferrer')}
              disabled={!downloadQuery.data?.download_url}
            >
              <Download className="h-4 w-4" /> Download client
            </Button>
            <Button onClick={() => createPairing.mutate()} disabled={createPairing.isPending}>
              <Plus className="h-4 w-4" /> Pair device
            </Button>
          </div>
        </div>
        {createPairing.error ? <ErrorText error={createPairing.error} /> : null}
      </section>

      {pairing ? <PairingPanel pairing={pairing} onClose={() => setPairing(null)} /> : null}

      <InstallationValidationScheduleCard
        status={validationQuery.data}
        loading={validationQuery.isLoading}
        saving={saveValidationSchedule.isPending}
        error={validationQuery.error instanceof Error ? validationQuery.error.message : saveValidationSchedule.error instanceof Error ? saveValidationSchedule.error.message : undefined}
        onChange={(config) => saveValidationSchedule.mutate(config)}
      />

      {devicesQuery.isLoading ? <PanelMessage text="Loading devices…" /> : null}
      {devicesQuery.error ? <ErrorText error={devicesQuery.error} /> : null}
      {devicesQuery.data?.length === 0 ? (
        <section className="rounded-mga border border-dashed border-mga-border bg-mga-surface/70 p-10 text-center">
          <Laptop className="mx-auto h-10 w-10 text-mga-muted" />
          <h3 className="mt-3 text-lg font-bold text-mga-text">No devices paired</h3>
          <p className="mt-1 text-sm text-mga-muted">Pair this PC or another device to install and launch games.</p>
        </section>
      ) : null}
      <div className="grid gap-4 xl:grid-cols-2">
        {devicesQuery.data?.map((device) => (
          <DeviceCard
            key={device.id}
            device={device}
            validationStatus={validationQuery.data?.devices.find((status) => status.endpoint_id === device.id)}
            selectedByLink={device.id === selectedDeviceID}
          />
        ))}
      </div>
    </div>
  )
}

function PairingPanel({ pairing, onClose }: { pairing: DevicePairingChallenge; onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    await navigator.clipboard.writeText(pairing.pair_command)
    setCopied(true)
  }
  return (
    <section className="rounded-mga border border-mga-accent/40 bg-mga-accent/5 p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="font-bold text-mga-text">Pair MGA Client</h3>
          <p className="mt-1 text-sm text-mga-muted">Open the client as the Windows user who will play here. Expires {new Date(pairing.expires_at).toLocaleTimeString()}.</p>
        </div>
        <Button variant="outline" size="sm" onClick={onClose}>Close</Button>
      </div>
      <div className="mt-4 flex items-center gap-3 rounded-mga border border-mga-border bg-black/30 p-3">
        <code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap text-sm text-mga-text">{pairing.pair_command}</code>
        <Button size="sm" onClick={copy}>{copied ? <Check className="h-4 w-4" /> : <Clipboard className="h-4 w-4" />}{copied ? 'Copied' : 'Copy'}</Button>
      </div>
      <Button className="mt-3" onClick={() => { window.location.href = pairing.pair_uri }}>
        <Laptop className="h-4 w-4" /> Open MGA Client
      </Button>
    </section>
  )
}

function DeviceCard({ device, validationStatus, selectedByLink = false }: { device: DeviceEndpoint; validationStatus?: InstallationValidationEndpointStatus; selectedByLink?: boolean }) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const cardRef = useRef<HTMLElement | null>(null)
  const state = statePresentation[device.status]
  const [expanded, setExpanded] = useState(false)
  const [name, setName] = useState(device.display_name)
  useEffect(() => {
    if (!selectedByLink) return
    setExpanded(true)
    const frame = window.requestAnimationFrame(() => cardRef.current?.scrollIntoView({ block: 'center' }))
    return () => window.cancelAnimationFrame(frame)
  }, [selectedByLink])
  const commandsQuery = useQuery({
    queryKey: ['device-commands', device.id],
    queryFn: () => listDeviceCommands(device.id),
    enabled: expanded,
    refetchInterval: expanded && device.status !== 'offline' ? 2000 : false,
  })
  const action = useMutation({
    mutationFn: (command: 'endpoint.ping' | 'endpoint.refresh' | 'inventory.refresh') => dispatchDeviceCommand(device.id, command),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['device-commands', device.id] })
      await queryClient.invalidateQueries({ queryKey: ['devices'] })
    },
  })
  const rename = useMutation({
    mutationFn: () => renameDevice(device.id, name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['devices'] }),
  })
  const revoke = useMutation({
    mutationFn: () => revokeDevice(device.id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['devices'] }),
  })
  const validateInstallations = useMutation({
    mutationFn: () => validateDeviceInstallations(device.id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['device-commands', device.id] })
      await queryClient.invalidateQueries({ queryKey: ['installation-validation-schedule'] })
    },
  })
  const latest = commandsQuery.data?.[0]
  const canManage = accessAllows(device.access_level, 'manage')
  const isOwner = device.access_level === 'owner'
  const deviceActions: ActionMenuItem[] = [
    {
      label: 'Check installed games',
      onSelect: () => validateInstallations.mutate(),
      disabled: device.status !== 'ready' || !device.capabilities.includes('game.validate_installations') || !validationStatus?.eligible_count || validateInstallations.isPending,
    },
    {
      label: 'Scan storage and apps',
      onSelect: () => action.mutate('inventory.refresh'),
      disabled: device.status === 'offline' || !canManage || !device.capabilities.includes('inventory.refresh') || action.isPending,
    },
    {
      label: 'Refresh device info',
      onSelect: () => action.mutate('endpoint.refresh'),
      disabled: device.status === 'offline' || !canManage || !device.capabilities.includes('endpoint.refresh') || action.isPending,
    },
  ]
  const totalBytes = device.inventory?.storage.reduce((total, item) => total + item.total_bytes, 0) ?? 0
  const freeBytes = device.inventory?.storage.reduce((total, item) => total + item.free_bytes, 0) ?? 0
  if (isOwner) {
    deviceActions.push({
      label: 'Remove device',
      onSelect: () => {
        if (window.confirm(`Remove ${device.display_name}? You will need to pair the client again.`)) revoke.mutate()
      },
      disabled: revoke.isPending,
      danger: true,
    })
  }
  return (
    <article
      ref={cardRef}
      data-device-id={device.id}
      className={cn('overflow-hidden rounded-mga border bg-mga-surface shadow-lg', state.border, selectedByLink && 'ring-2 ring-mga-accent/60')}
    >
      <div className="p-4">
        <button
          type="button"
          onClick={() => setExpanded((current) => !current)}
          className="flex w-full items-start justify-between gap-3 rounded-mga text-left focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent"
          aria-expanded={expanded}
          aria-label={`${expanded ? 'Collapse' : 'Expand'} ${device.display_name}`}
        >
          <div className="flex min-w-0 items-center gap-3">
            <div className="grid h-11 w-11 shrink-0 place-items-center rounded-mga bg-mga-bg"><Laptop className="h-5 w-5 text-mga-accent" /></div>
            <div className="min-w-0">
              <h3 className="truncate text-lg font-black text-mga-text">{device.display_name}</h3>
              <div className="mt-1 flex items-center gap-2 text-sm text-mga-muted"><UserRound className="h-3.5 w-3.5" />{device.os_user} on {device.host_name}</div>
              <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-mga-muted">
                <span>{device.platform} / {device.arch}</span>
                <span>Access: <span className="font-semibold text-mga-text">{accessLevelLabel(device.access_level)}</span></span>
              </div>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <div className={cn('inline-flex items-center gap-2 rounded-full border bg-black/20 px-2.5 py-1 text-xs font-bold', state.border, state.text)}>
              <span className={cn('h-2 w-2 rounded-full', state.dot)} />{state.label}
            </div>
            {expanded ? <ChevronDown className="h-5 w-5 text-mga-muted" /> : <ChevronRight className="h-5 w-5 text-mga-muted" />}
          </div>
        </button>
        {device.status_reason ? <p className="mt-3 text-sm text-red-300">{device.status_reason}</p> : null}
        {expanded ? (
          <div className="mt-4 border-t border-mga-border pt-4">
            <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
              <DeviceFact label="Platform" value={`${device.platform} / ${device.arch}`} />
              <DeviceFact label="Client" value={device.client_version} />
              <DeviceFact label="Access" value={accessLevelLabel(device.access_level)} />
              <DeviceFact label="Last seen" value={device.last_seen_at ? new Date(device.last_seen_at).toLocaleString() : 'Never'} />
              <DeviceFact label="Free storage" value={device.inventory ? `${formatBytes(freeBytes)} of ${formatBytes(totalBytes)}` : 'Not scanned yet'} />
              <DeviceFact label="Game apps" value={device.inventory?.runtimes.length ? device.inventory.runtimes.map((runtime) => runtime.name).join(', ') : 'None found'} />
              <DeviceFact label="Installed by MGA" value={String(device.installations?.length ?? 0)} />
            </dl>
			{device.installations?.length ? (
			  <div className="mt-3 rounded-mga border border-mga-border bg-mga-bg/70 p-3">
				<div className="flex flex-wrap items-center justify-between gap-2">
				  <span className="inline-flex items-center gap-1.5 text-xs font-semibold text-mga-text"><ShieldCheck className="h-3.5 w-3.5" /> Installed games</span>
				  <span className="text-[11px] text-mga-muted">{validationStatusLabel(validationStatus)}</span>
				</div>
				<div className="mt-2 space-y-2">
				  {device.installations.map((installation) => {
					const presentation = installationStatePresentation(installation.install_state)
					const Icon = presentation.icon
					return (
					  <div key={`${installation.game_id}:${installation.source_game_id}`} className="flex items-start gap-2 rounded-mga bg-black/20 px-3 py-2">
						<Icon className={cn('mt-0.5 h-4 w-4 shrink-0', presentation.color)} />
						<div className="min-w-0 flex-1">
						  <div className="flex flex-wrap items-center justify-between gap-2">
							<span className="truncate text-xs font-semibold text-mga-text" title={installation.install_path}>{installationFolderName(installation.install_path)}</span>
							<span className={cn('text-[10px] font-bold', presentation.color)}>{presentation.label}</span>
						  </div>
						  {installation.install_state !== 'installed' ? <p className="mt-1 text-[11px] text-mga-muted">{installationReasonLabel(installation.verification_reason_code || installation.state_reason)}</p> : null}
						  {installation.last_verified_at ? <p className="mt-1 text-[10px] text-mga-muted">Checked {new Date(installation.last_verified_at).toLocaleString()}</p> : null}
						  {installation.install_state !== 'installed' ? (
							<Button
							  size="sm"
							  variant="outline"
							  className="mt-2"
							  onClick={() => navigate({ pathname: `/game/${encodeURIComponent(installation.game_id)}`, hash: '#device-play' })}
							>
							  Review and resolve
							</Button>
						  ) : null}
						</div>
					  </div>
					)
				  })}
				</div>
			  </div>
			) : null}
			{device.inventory ? (
			  <div className="mt-3 rounded-mga border border-mga-border bg-mga-bg/70 p-3">
				<div className="flex items-center justify-between gap-3 text-xs text-mga-muted">
				  <span className="inline-flex items-center gap-1.5"><HardDrive className="h-3.5 w-3.5" /> Device scan</span>
				  <span>{new Date(device.inventory.captured_at).toLocaleString()}</span>
				</div>
				{device.inventory.storage.length ? (
				  <div className="mt-2 space-y-2">
					{device.inventory.storage.map((storage) => {
					  const usedPercent = storage.total_bytes > 0 ? Math.round(((storage.total_bytes - storage.free_bytes) / storage.total_bytes) * 100) : 0
					  return (
						<div key={storage.id}>
						  <div className="flex justify-between gap-3 text-xs"><span className="font-semibold text-mga-text">{storage.root}</span><span className="text-mga-muted">{formatBytes(storage.free_bytes)} free</span></div>
						  <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-black/30"><div className="h-full rounded-full bg-mga-accent" style={{ width: `${Math.min(100, Math.max(0, usedPercent))}%` }} /></div>
						</div>
					  )
					})}
				  </div>
				) : <p className="mt-2 text-xs text-mga-muted">No writable storage was reported.</p>}
				{device.inventory.managed_installations?.length ? (
				  <div className="mt-3 border-t border-mga-border pt-3">
					<div className="mb-2 flex items-center justify-between gap-2 text-xs"><span className="font-semibold text-mga-text">Games found by MGA Client</span><span className="text-mga-muted">{device.inventory.managed_installations.length}</span></div>
					<div className="space-y-2">
					  {device.inventory.managed_installations.map((item) => (
						<div key={item.local_installation_id} className="rounded-mga bg-black/20 px-3 py-2">
						  <div className="flex flex-wrap items-center justify-between gap-2"><span className="text-xs font-semibold text-mga-text">{item.title}</span><span className="text-[10px] font-bold text-mga-muted">{ownershipStateLabel[item.state] ?? item.state}</span></div>
						  {item.install_path ? <p className="mt-1 truncate text-[10px] text-mga-muted" title={item.install_path}>{item.install_path}</p> : null}
						  {item.native_products?.length ? (
							<div className="mt-1 space-y-1">
							  {item.native_products.map((product) => <p key={`${product.provider}:${product.product_id}`} className="truncate text-[10px] text-mga-muted" title={`${product.publisher || ''} ${product.version || ''}`.trim()}>Windows: {product.display_name}{product.version ? ` · ${product.version}` : ''}</p>)}
							</div>
						  ) : null}
						  {item.can_manage ? <Button size="sm" variant="outline" className="mt-2" onClick={() => openOwnershipAction('release', item.local_installation_id)}>Release</Button> : null}
						  {item.can_adopt ? <Button size="sm" variant="outline" className="mt-2" onClick={() => openOwnershipAction('adopt', item.local_installation_id)}>Pick up</Button> : null}
						</div>
					  ))}
					</div>
				  </div>
				) : null}
			  </div>
			) : null}
            <details className="mt-3 text-xs text-mga-muted">
              <summary className="cursor-pointer select-none hover:text-mga-text">Technical details</summary>
              <dl className="mt-2 grid grid-cols-2 gap-x-4 gap-y-2">
                <DeviceFact label="Protocol" value={`v${device.protocol_version}`} />
                <DeviceFact label="Features" value={String(device.capabilities.length)} />
              </dl>
            </details>
            {isOwner ? (
              <div className="mt-4 flex gap-2">
                <Input aria-label="Device name" value={name} onChange={(event) => setName(event.target.value)} />
                <Button variant="outline" size="sm" onClick={() => rename.mutate()} disabled={!name.trim() || name === device.display_name || rename.isPending}><Save className="h-4 w-4" /> Rename</Button>
              </div>
            ) : null}
			<DeviceInstallFolderPanel device={device} isOwner={isOwner} expanded={expanded} />
            <div className="mt-4 flex flex-wrap gap-2 border-t border-mga-border pt-4">
              <Tooltip content="Check that MGA Client is responding">
                <Button size="sm" onClick={() => action.mutate('endpoint.ping')} disabled={device.status === 'offline' || !device.capabilities.includes('endpoint.ping') || action.isPending}><Send className="h-4 w-4" /> Check</Button>
              </Tooltip>
              <ActionMenu items={deviceActions} className="ml-auto" />
            </div>
            {latest ? (
              <div className="mt-3 rounded-mga bg-mga-bg px-3 py-2 text-xs text-mga-muted">
                Latest: <span className="font-mono text-mga-text">{latest.name}</span> · <span className={latest.status === 'succeeded' ? 'text-emerald-300' : latest.status === 'failed' || latest.status === 'rejected' ? 'text-red-300' : 'text-amber-300'}>{latest.status}</span>
                {latest.error_message ? ` · ${latest.error_message}` : ''}
                {latest.progress_message ? ` · ${latest.progress_message}` : ''}
                {latest.progress_percent !== undefined ? ` (${latest.progress_percent}%)` : ''}
              </div>
            ) : null}
            {action.error || validateInstallations.error || rename.error || revoke.error ? <ErrorText error={action.error || validateInstallations.error || rename.error || revoke.error} /> : null}
            {isOwner ? <DeviceAccessPanel device={device} /> : null}
          </div>
        ) : null}
      </div>
    </article>
  )
}

function DeviceInstallFolderPanel({ device, isOwner, expanded }: { device: DeviceEndpoint; isOwner: boolean; expanded: boolean }) {
  const { currentProfile } = useProfiles()
  const queryClient = useQueryClient()
  const [root, setRoot] = useState('')
  const preference = useQuery({
    queryKey: ['endpoint-install-preference', device.id, currentProfile?.id],
    queryFn: () => getEndpointInstallPreference(device.id),
    enabled: expanded,
  })
  useEffect(() => {
    if (preference.data) setRoot(preference.data.endpoint_root || '')
  }, [preference.data])
  const save = useMutation({
    mutationFn: (value: string) => setEndpointInstallPreference(device.id, value),
    onSuccess: async (saved) => {
      setRoot(saved.endpoint_root || '')
      queryClient.setQueryData(['endpoint-install-preference', device.id, currentProfile?.id], saved)
      await queryClient.invalidateQueries({ queryKey: ['endpoint-install-preference', device.id] })
    },
  })

  return (
    <div className="mt-4 rounded-mga border border-mga-border bg-mga-bg/70 p-3">
      <div className="flex items-center gap-2 text-xs font-semibold text-mga-text"><HardDrive className="h-3.5 w-3.5" /> Install folder</div>
      {isOwner ? (
        <>
          <div className="mt-3 flex gap-2">
            <Input
              aria-label={`Install folder for ${device.display_name}`}
              value={root}
              onChange={(event) => setRoot(event.target.value)}
              placeholder={preference.data?.profile_root || '%USERPROFILE%\\Games'}
              disabled={preference.isLoading || save.isPending}
            />
            <Button variant="outline" size="sm" onClick={() => save.mutate(root.trim())} disabled={save.isPending || root.trim() === (preference.data?.endpoint_root || '')}>
              <Save className="h-4 w-4" /> Save
            </Button>
          </div>
          <p className="mt-2 text-xs text-mga-muted">
            {preference.data?.endpoint_root ? 'This device uses its own folder.' : `Using your default: ${preference.data?.profile_root || '%USERPROFILE%\\Games'}`}
          </p>
          {preference.data?.endpoint_root ? <Button variant="ghost" size="sm" className="mt-2" onClick={() => save.mutate('')} disabled={save.isPending}>Use my default</Button> : null}
        </>
      ) : (
        <p className="mt-2 text-xs text-mga-muted">{preference.data?.effective_root || 'Loading…'}</p>
      )}
      {preference.error || save.error ? <ErrorText error={preference.error || save.error} /> : null}
    </div>
  )
}

function DeviceAccessPanel({ device }: { device: DeviceEndpoint }) {
  const { profiles } = useProfiles()
  const queryClient = useQueryClient()
  const [profileId, setProfileId] = useState('')
  const [accessLevel, setAccessLevel] = useState<DeviceAccessLevel>('view')
  const grants = useQuery({ queryKey: ['device-grants', device.id], queryFn: () => listDeviceGrants(device.id) })
  const save = useMutation({
    mutationFn: ({ targetProfileId, level }: { targetProfileId: string; level: DeviceAccessLevel }) => setDeviceGrant(device.id, targetProfileId, level),
    onSuccess: async () => {
      setProfileId('')
      await queryClient.invalidateQueries({ queryKey: ['device-grants', device.id] })
      await queryClient.invalidateQueries({ queryKey: ['devices'] })
    },
  })
  const remove = useMutation({
    mutationFn: (targetProfileId: string) => deleteDeviceGrant(device.id, targetProfileId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['device-grants', device.id] })
      await queryClient.invalidateQueries({ queryKey: ['devices'] })
    },
  })
  const grantedProfiles = new Set(grants.data?.map((grant) => grant.profile_id) ?? [])
  const availableProfiles = profiles.filter((profile) => !grantedProfiles.has(profile.id))
  const levelOptions = [
    { value: 'view', label: 'View' },
    { value: 'play', label: 'Play' },
    { value: 'manage', label: 'Manage' },
    { value: 'owner', label: 'Owner' },
  ]

  return (
    <div className="mt-4 border-t border-mga-border pt-4">
      <h4 className="text-xs font-bold uppercase tracking-[0.18em] text-mga-muted">Who can use this device</h4>
      <p className="mt-2 text-xs leading-5 text-mga-muted">
        Choose what each MGA profile can do here.
      </p>
      <div className="mt-3 space-y-2">
        {grants.data?.map((grant) => (
          <div key={grant.profile_id} className="flex flex-wrap items-center gap-2 rounded-mga bg-mga-bg px-3 py-2">
            <span className="min-w-0 flex-1 truncate text-sm font-semibold text-mga-text">{grant.profile_display_name}</span>
            <Select
              aria-label={`Access for ${grant.profile_display_name}`}
              value={grant.access_level}
              onChange={(event) => save.mutate({ targetProfileId: grant.profile_id, level: event.target.value as DeviceAccessLevel })}
              options={levelOptions}
            />
            <Button variant="outline" size="sm" onClick={() => remove.mutate(grant.profile_id)} disabled={remove.isPending}><Trash2 className="h-4 w-4" /> Remove</Button>
          </div>
        ))}
      </div>
      {availableProfiles.length ? (
        <div className="mt-3 grid gap-2 sm:grid-cols-[1fr_9rem_auto] sm:items-end">
          <Select
            label="Share with profile"
            value={profileId}
            onChange={(event) => setProfileId(event.target.value)}
            options={[{ value: '', label: 'Choose profile' }, ...availableProfiles.map((profile) => ({ value: profile.id, label: profile.display_name }))]}
          />
          <Select label="Access" value={accessLevel} onChange={(event) => setAccessLevel(event.target.value as DeviceAccessLevel)} options={levelOptions} />
          <Button size="sm" onClick={() => save.mutate({ targetProfileId: profileId, level: accessLevel })} disabled={!profileId || save.isPending}><Plus className="h-4 w-4" /> Share</Button>
        </div>
      ) : null}
      {grants.error || save.error || remove.error ? <ErrorText error={grants.error || save.error || remove.error} /> : null}
    </div>
  )
}

function accessAllows(granted: DeviceAccessLevel, required: DeviceAccessLevel): boolean {
  const rank: Record<DeviceAccessLevel, number> = { view: 1, play: 2, manage: 3, owner: 4 }
  return rank[granted] >= rank[required]
}

function accessLevelLabel(level: DeviceAccessLevel): string {
  return level.charAt(0).toUpperCase() + level.slice(1)
}

function installationFolderName(path: string): string {
  return path.split(/[\\/]/).filter(Boolean).at(-1) || path
}

function installationStatePresentation(state: string) {
  switch (state) {
    case 'installed': return { label: 'Ready', color: 'text-emerald-300', icon: CheckCircle2 }
    case 'missing': return { label: 'Missing', color: 'text-red-300', icon: AlertTriangle }
    case 'needs_repair': return { label: 'Needs repair', color: 'text-amber-300', icon: AlertTriangle }
    default: return { label: 'Needs attention', color: 'text-amber-300', icon: AlertTriangle }
  }
}

function DeviceFact({ label, value }: { label: string; value: string }) {
  return <div><dt className="text-xs uppercase tracking-wider text-mga-muted">{label}</dt><dd className="mt-0.5 truncate font-semibold text-mga-text">{value}</dd></div>
}

function PanelMessage({ text }: { text: string }) {
  return <div className="rounded-mga border border-mga-border bg-mga-surface p-6 text-sm text-mga-muted">{text}</div>
}

function ErrorText({ error }: { error: unknown }) {
  return <p className="mt-3 text-sm text-red-400">{error instanceof Error ? error.message : 'The operation failed.'}</p>
}
