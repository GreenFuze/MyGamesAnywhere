import { useSearchParams } from 'react-router-dom'
import { Tabs, type Tab } from '@/components/ui/tabs'
import { IntegrationsTab } from '@/components/settings/IntegrationsTab'
import { PluginsTab } from '@/components/settings/PluginsTab'
import { AppearanceTab } from '@/components/settings/AppearanceTab'
import { UndetectedGamesTab } from '@/components/settings/UndetectedGamesTab'
import { CacheTab } from '@/components/settings/CacheTab'
import { DuplicatesTab } from '@/components/settings/DuplicatesTab'
import { ProfilesTab } from '@/components/settings/ProfilesTab'
import { UpdateTab } from '@/components/settings/SettingsTab'
import { DevicesTab } from '@/components/settings/DevicesTab'
import { MySettingsTab } from '@/components/settings/MySettingsTab'
import { EmulatorsTab } from '@/components/settings/EmulatorsTab'
import { useProfiles } from '@/hooks/useProfiles'

const TABS: Tab[] = [
  { id: 'my-settings', label: 'My Settings' },
  { id: 'integrations', label: 'Connections' },
  { id: 'devices', label: 'Devices' },
  { id: 'emulators', label: 'Emulators' },
  { id: 'profiles', label: 'Profiles' },
  { id: 'cache', label: 'Storage' },
  { id: 'appearance', label: 'Appearance' },
  { id: 'update', label: 'Updates' },
  { id: 'duplicates', label: 'Copies' },
  { id: 'undetected', label: 'Unidentified' },
  { id: 'plugins', label: 'Advanced' },
]

const TAB_COMPONENTS: Record<string, React.FC> = {
  'my-settings': MySettingsTab,
  update: UpdateTab,
  profiles: ProfilesTab,
  devices: DevicesTab,
  emulators: EmulatorsTab,
  plugins: PluginsTab,
  cache: CacheTab,
  duplicates: DuplicatesTab,
  appearance: AppearanceTab,
  undetected: UndetectedGamesTab,
}

export function SettingsPage() {
  const { currentProfile } = useProfiles()
  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const normalizedTabParam = tabParam === 'settings' ? 'update' : tabParam
  const availableTabs = currentProfile?.role === 'admin_player' ? TABS : TABS.filter((tab) => tab.id === 'my-settings' || tab.id === 'profiles' || tab.id === 'devices' || tab.id === 'emulators' || tab.id === 'appearance')
  const fallbackTab = currentProfile?.role === 'admin_player' ? 'integrations' : 'my-settings'
  const activeTab = normalizedTabParam && availableTabs.some((tab) => tab.id === normalizedTabParam) ? normalizedTabParam : fallbackTab

  const handleTabChange = (id: string) => {
    const next = new URLSearchParams(searchParams)
    next.set('tab', id)
    setSearchParams(next, { replace: true })
  }

  const ActiveComponent = TAB_COMPONENTS[activeTab]

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-mga-text">Settings</h1>
      </div>

      <Tabs tabs={availableTabs} active={activeTab} onChange={handleTabChange} />

      <div className="pb-8">
        {activeTab === 'integrations' ? (
          <IntegrationsTab firstRunRestore={searchParams.get('first_run') === 'restore'} />
        ) : ActiveComponent ? (
          <ActiveComponent />
        ) : null}
      </div>
    </div>
  )
}
