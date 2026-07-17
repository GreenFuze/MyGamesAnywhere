import { useQuery } from '@tanstack/react-query'
import { Gamepad2, Loader2, Monitor, RefreshCw } from 'lucide-react'
import { listDevices } from '@/api/client'
import { Button } from '@/components/ui/button'
import { useProfiles } from '@/hooks/useProfiles'

const EMULATOR_IDS = new Set(['retroarch', 'scummvm', 'dosbox', 'duckstation', 'pcsx2'])

export function EmulatorsTab() {
  const { currentProfile } = useProfiles()
  const devices = useQuery({
    queryKey: ['devices', currentProfile?.id],
    queryFn: listDevices,
    enabled: Boolean(currentProfile),
    refetchInterval: 5000,
  })

  return (
    <div className="space-y-5">
      <section className="rounded-mga border border-mga-border bg-mga-surface p-5 shadow-lg">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.2em] text-mga-accent"><Gamepad2 size={16} /> This device</div>
            <h2 className="mt-2 text-xl font-black text-mga-text">Emulators</h2>
            <p className="mt-1 max-w-2xl text-sm leading-6 text-mga-muted">Emulators belong to a PC and Windows user. MGA checks each paired device separately.</p>
          </div>
          <Button variant="outline" onClick={() => devices.refetch()} disabled={devices.isFetching}>
            {devices.isFetching ? <Loader2 size={16} className="animate-spin" /> : <RefreshCw size={16} />} Check again
          </Button>
        </div>
      </section>

      {devices.isLoading ? <p className="text-sm text-mga-muted">Checking paired devices…</p> : null}
      {devices.error ? <p className="text-sm text-red-300">Sign in again to view emulator settings.</p> : null}
      <div className="grid gap-4 xl:grid-cols-2">
        {devices.data?.map((device) => {
          const emulators = device.inventory?.runtimes.filter((runtime) => EMULATOR_IDS.has(runtime.id)) ?? []
          return (
            <section key={device.id} className="rounded-mga border border-mga-border bg-mga-surface p-5 shadow-lg">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <h3 className="flex items-center gap-2 font-bold text-mga-text"><Monitor size={16} /> {device.display_name}</h3>
                  <p className="mt-1 text-xs text-mga-muted">{device.host_name} · {device.os_user}</p>
                </div>
                <span className={device.status === 'ready' ? 'text-xs text-emerald-300' : 'text-xs text-mga-muted'}>{device.status === 'ready' ? 'Connected' : 'Not connected'}</span>
              </div>
              {emulators.length ? (
                <div className="mt-4 space-y-2">
                  {emulators.map((emulator) => (
                    <div key={emulator.id} className="rounded-mga border border-emerald-500/20 bg-emerald-500/5 p-3">
                      <p className="text-sm font-semibold text-mga-text">{emulator.name}</p>
                      <p className="mt-1 break-all text-xs text-mga-muted">Found for this Windows user{emulator.path ? ` · ${emulator.path}` : ''}</p>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="mt-4 rounded-mga border border-dashed border-mga-border p-4 text-sm text-mga-muted">
                  {device.inventory ? 'No supported emulator was found for this Windows user.' : 'Check the device from Settings → Devices to look for emulators.'}
                </div>
              )}
              <p className="mt-3 text-xs leading-5 text-mga-muted">Game-system choices, cores, firmware, and RetroAchievements support will appear here as MGA adds managed emulator setup.</p>
            </section>
          )
        })}
      </div>
    </div>
  )
}
