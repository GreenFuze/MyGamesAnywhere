import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, CircleCheck, Download, Gamepad2, Loader2, Monitor, RefreshCw, Save, Trophy } from 'lucide-react'
import {
  dispatchDeviceCommand,
  getEndpointEmulators,
  listDeviceCommands,
  listDevices,
  setEndpointEmulatorCore,
  setEndpointEmulatorDefault,
  setupEndpointEmulator,
  type DeviceEmulatorConfiguration,
  type DeviceEmulatorCoreOption,
  type DeviceEmulatorOption,
  type DeviceEndpoint,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { useProfiles } from '@/hooks/useProfiles'

const terminalCommandStates = new Set(['succeeded', 'failed', 'rejected', 'canceled', 'expired'])

function optionTone(option: DeviceEmulatorOption): string {
  if (option.state === 'ready') return 'border-emerald-500/25 bg-emerald-500/7 text-emerald-200'
  if (option.detected) return 'border-violet-500/25 bg-violet-500/7 text-violet-200'
  return 'border-mga-border bg-black/10 text-mga-muted'
}

function optionLabel(option: DeviceEmulatorOption): string {
  if (option.state === 'ready') return 'Ready'
  if (option.detected) return 'Needs setup'
  return 'Not installed'
}

function coreFact(core: DeviceEmulatorCoreOption): string {
  const achievements = core.capabilities.find((fact) => fact.id === 'retroachievements')?.state
  if (achievements === 'supported') return 'Achievements'
  if (achievements === 'unsupported') return 'No achievements'
  return 'Achievements unknown'
}

function firmwareFact(core: DeviceEmulatorCoreOption): string {
  if (core.firmware_state === 'present') return ' · Firmware ready'
  if (core.firmware_state === 'missing') return ' · Firmware needed'
  if (core.firmware_state === 'unknown') return ' · Firmware unchecked'
  return ''
}

function saveSupportFact(option: DeviceEmulatorOption): { label: string; title: string } | null {
  if (!option.detected) return null
  if (option.save_probe_state === 'complete') {
    return {
      label: 'Save support detected',
      title: option.save_route_overrides
        ? 'MGA found emulator save settings, including game-specific choices. Backups are not enabled yet.'
        : 'MGA found this emulator’s save settings. Backups are not enabled yet.',
    }
  }
  if (option.save_probe_state === 'partial') {
    return { label: 'Save setup needs attention', title: 'MGA could not completely read this emulator’s save settings.' }
  }
  return { label: 'Save support unknown', title: 'Reconnect with an updated MGA Client to check local save support.' }
}

function commandProgressLabel(phase?: string, percent?: number): string {
  const label = phase === 'install' ? 'Installing' : phase === 'download' ? 'Downloading' : 'Working'
  return percent === undefined ? `${label}…` : `${label} ${Math.round(percent)}%`
}

export function EmulatorsTab() {
  const { currentProfile } = useProfiles()
  const queryClient = useQueryClient()
  const devices = useQuery({
    queryKey: ['devices', currentProfile?.id],
    queryFn: listDevices,
    enabled: Boolean(currentProfile),
    refetchInterval: 5000,
  })

  const refresh = async () => {
    await devices.refetch()
    await queryClient.invalidateQueries({ queryKey: ['device-emulators', currentProfile?.id] })
  }

  return (
    <div className="space-y-5">
      <section className="rounded-mga border border-mga-border bg-mga-surface p-5 shadow-lg">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.2em] text-mga-accent"><Gamepad2 size={16} /> Play your way</div>
            <h2 className="mt-2 text-xl font-black text-mga-text">Emulators</h2>
            <p className="mt-1 max-w-2xl text-sm leading-6 text-mga-muted">Choose how each device plays your classic games.</p>
          </div>
          <Button variant="outline" onClick={() => void refresh()} disabled={devices.isFetching}>
            {devices.isFetching ? <Loader2 size={16} className="animate-spin" /> : <RefreshCw size={16} />} Check again
          </Button>
        </div>
      </section>

      {devices.isLoading ? <p className="text-sm text-mga-muted">Checking paired devices…</p> : null}
      {devices.error ? <p className="text-sm text-red-300">Sign in again to view emulator settings.</p> : null}
      <div className="space-y-4">
        {devices.data?.map((device) => (
          <DeviceEmulatorPanel key={device.id} device={device} profileId={currentProfile?.id ?? ''} single={devices.data.length === 1} />
        ))}
      </div>
    </div>
  )
}

function DeviceEmulatorPanel({ device, profileId, single }: { device: DeviceEndpoint; profileId: string; single: boolean }) {
  const queryClient = useQueryClient()
  const [activeCommandId, setActiveCommandId] = useState('')
  const [activity, setActivity] = useState('')
  const processedCommand = useRef('')
  const configuration = useQuery({
    queryKey: ['device-emulators', profileId, device.id],
    queryFn: () => getEndpointEmulators(device.id),
    enabled: Boolean(profileId),
    staleTime: 5000,
  })
  const commands = useQuery({
    queryKey: ['device-commands', profileId, device.id, activeCommandId],
    queryFn: () => listDeviceCommands(device.id),
    enabled: Boolean(activeCommandId),
    refetchInterval: activeCommandId ? 1000 : false,
  })
  const saveConfiguration = (saved: DeviceEmulatorConfiguration) => {
    queryClient.setQueryData(['device-emulators', profileId, saved.endpoint_id], saved)
  }
  const updateDefault = useMutation({
    mutationFn: ({ platform, emulatorId }: { platform: string; emulatorId: string }) => setEndpointEmulatorDefault(device.id, platform, emulatorId),
    onSuccess: saveConfiguration,
  })
  const updateCore = useMutation({
    mutationFn: ({ platform, emulatorId, coreId }: { platform: string; emulatorId: string; coreId: string }) => setEndpointEmulatorCore(device.id, platform, emulatorId, coreId),
    onSuccess: saveConfiguration,
  })
  const setup = useMutation({
    mutationFn: ({ emulatorId, action }: { emulatorId: string; action: 'install' | 'update' }) => setupEndpointEmulator(device.id, emulatorId, action),
    onSuccess: (command, variables) => {
      processedCommand.current = ''
      setActivity(variables.action === 'install' ? 'Starting installation…' : 'Checking for updates…')
      setActiveCommandId(command.id)
    },
  })

  const activeCommand = commands.data?.find((command) => command.id === activeCommandId)
  useEffect(() => {
    if (!activeCommand) return
    if (!terminalCommandStates.has(activeCommand.status)) {
      setActivity(commandProgressLabel(activeCommand.progress_stage || activeCommand.progress_phase, activeCommand.progress_stage_percent ?? activeCommand.progress_percent))
      return
    }
    if (processedCommand.current === activeCommand.id) return
    processedCommand.current = activeCommand.id
    if (activeCommand.status !== 'succeeded') {
      setActivity(activeCommand.error_message || 'Emulator setup did not finish.')
      setActiveCommandId('')
      return
    }
    if (activeCommand.name === 'emulator.setup') {
      setActivity('Refreshing emulator details…')
      void dispatchDeviceCommand(device.id, 'inventory.refresh')
        .then((command) => {
          processedCommand.current = ''
          setActiveCommandId(command.id)
        })
        .catch((error: unknown) => {
          setActivity(error instanceof Error ? error.message : 'Installed, but device details could not be refreshed.')
          setActiveCommandId('')
        })
      return
    }
    setActivity('Emulator details updated.')
    setActiveCommandId('')
    void configuration.refetch()
    void queryClient.invalidateQueries({ queryKey: ['devices', profileId] })
  }, [activeCommand, configuration, device.id, profileId, queryClient])

  const platforms = configuration.data?.platforms ?? []
  const detectedCount = platforms.flatMap((platform) => platform.emulators).filter((option, index, all) => option.detected && all.findIndex((candidate) => candidate.id === option.id) === index).length
  const error = updateDefault.error || updateCore.error || setup.error

  return (
    <details className="group rounded-mga border border-mga-border bg-mga-surface shadow-lg" open={single}>
      <summary className="flex cursor-pointer list-none items-center justify-between gap-3 p-5 [&::-webkit-details-marker]:hidden">
        <div>
          <h3 className="flex items-center gap-2 font-bold text-mga-text"><Monitor size={16} /> {device.display_name}</h3>
          <p className="mt-1 text-xs text-mga-muted">{device.host_name} · {device.os_user} · {detectedCount} found</p>
        </div>
        <div className="flex items-center gap-3">
          <span className={device.status === 'ready' || device.status === 'busy' ? 'text-xs text-emerald-300' : device.status === 'update_required' ? 'text-xs text-violet-300' : 'text-xs text-mga-muted'}>
            {device.status === 'ready' || device.status === 'busy' ? 'Connected' : device.status === 'update_required' ? 'Update needed' : 'Not connected'}
          </span>
          <ChevronDown size={17} className="text-mga-muted transition-transform group-open:rotate-180" />
        </div>
      </summary>
      <div className="border-t border-mga-border px-5 pb-5 pt-4">
        {activity ? <p className="mb-3 flex items-center gap-2 text-sm text-mga-accent">{activeCommandId ? <Loader2 size={14} className="animate-spin" /> : <CircleCheck size={14} />}{activity}</p> : null}
        {configuration.isLoading ? <p className="text-sm text-mga-muted">Checking emulator choices…</p> : null}
        {configuration.error ? <p className="text-sm text-red-300">Could not load emulator choices for this device.</p> : null}
        <div className="grid gap-3 xl:grid-cols-2">
          {platforms.map((platform) => (
            <section key={platform.platform} className="rounded-[16px] border border-mga-border bg-black/10 p-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h4 className="text-sm font-semibold text-mga-text">{platform.platform_name}</h4>
                  <p className="mt-1 text-xs text-mga-muted">{platform.emulators.length > 1 ? `${platform.emulators.length} ways to play` : 'Ready when you are'}</p>
                </div>
                <label>
                  <span className="sr-only">Main emulator for {platform.platform_name}</span>
                  <select
                    className="h-9 max-w-48 rounded-full border border-mga-border bg-mga-bg px-3 text-sm text-mga-text"
                    value={platform.selected_default ?? ''}
                    disabled={device.access_level !== 'owner' || updateDefault.isPending}
                    title={device.access_level === 'owner' ? 'Sets the main Play action; all choices remain available.' : 'Only a device owner can change this choice.'}
                    onChange={(event) => updateDefault.mutate({ platform: platform.platform, emulatorId: event.target.value })}
                  >
                    <option value="">Automatic</option>
                    {platform.emulators.map((option) => <option key={option.id} value={option.id}>{option.name}</option>)}
                  </select>
                </label>
              </div>
              <div className="mt-3 space-y-2">
                {platform.emulators.map((option) => (
                  <div key={option.id} className={`rounded-xl border p-3 ${optionTone(option)}`} title={option.reason || option.setup_hint}>
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div>
                        <div className="text-sm font-semibold">{option.name}{option.version ? <span className="ml-2 font-normal opacity-70">{option.version}</span> : null}</div>
                        <div className="mt-0.5 text-xs opacity-80">{optionLabel(option)}{platform.resolved_default === option.id ? ' · Main' : ''}</div>
                      </div>
                      {option.setup_available ? (
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={device.access_level !== 'owner' || setup.isPending || Boolean(activeCommandId)}
                          title={device.access_level === 'owner' ? undefined : 'Only the device owner can install or update emulators.'}
                          onClick={() => {
                            const action = option.detected ? 'update' : 'install'
                            const verb = action === 'install' ? 'install' : 'check for updates to'
                            if (window.confirm(`Use Windows Package Manager to ${verb} ${option.name} on ${device.display_name}?`)) setup.mutate({ emulatorId: option.id, action })
                          }}
                        >
                          {setup.isPending ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}{option.detected ? 'Update' : 'Install'}
                        </Button>
                      ) : null}
                    </div>
                    {saveSupportFact(option) ? (
                      <div className="mt-2 flex items-center gap-1.5 text-xs opacity-80" title={saveSupportFact(option)?.title}>
                        <Save size={12} /> {saveSupportFact(option)?.label}
                      </div>
                    ) : null}
                    {option.cores?.length ? (
                      <div className="mt-3 border-t border-current/10 pt-3">
                        <label className="flex items-center justify-between gap-3 text-xs">
                          <span>Core</span>
                          <select
                            className="h-8 max-w-52 rounded-full border border-mga-border bg-mga-bg px-2.5 text-xs text-mga-text"
                            value={option.selected_core ?? ''}
                            disabled={device.access_level !== 'owner' || updateCore.isPending}
                            onChange={(event) => updateCore.mutate({ platform: platform.platform, emulatorId: option.id, coreId: event.target.value })}
                          >
                            <option value="">Automatic</option>
                            {option.cores.map((core) => <option key={core.id} value={core.id}>{core.name}{core.detected ? '' : ' (not installed)'}</option>)}
                          </select>
                        </label>
                        <div className="mt-2 flex flex-wrap gap-1.5">
                          {option.cores.filter((core) => core.detected).map((core) => (
                            <span key={core.id} className="rounded-full border border-current/15 px-2 py-0.5 text-[11px]" title={core.reason || `Firmware: ${core.firmware_state}`}>
                              {core.name}{option.resolved_core === core.id ? ' · Active' : ''} · <Trophy size={10} className="inline" /> {coreFact(core)}{firmwareFact(core)}
                            </span>
                          ))}
                        </div>
                      </div>
                    ) : null}
                  </div>
                ))}
              </div>
            </section>
          ))}
        </div>
        {error ? <p className="mt-3 text-sm text-red-300">{error instanceof Error ? error.message : 'Could not save emulator settings.'}</p> : null}
      </div>
    </details>
  )
}
