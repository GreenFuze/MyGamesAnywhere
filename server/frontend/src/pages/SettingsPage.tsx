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
import { useProfiles } from '@/hooks/useProfiles'

const TABS: Tab[] = [
  { id: 'integrations', label: 'Integrations' },
  { id: 'update', label: 'Update' },
  { id: 'profiles', label: 'Profiles' },
  { id: 'devices', label: 'Devices' },
  { id: 'plugins', label: 'Plugins' },
  { id: 'cache', label: 'Cache' },
  { id: 'duplicates', label: 'Duplicates' },
  { id: 'appearance', label: 'Appearance' },
  { id: 'undetected', label: 'Undetected Games' },
]

const TAB_COMPONENTS: Record<string, React.FC> = {
  update: UpdateTab,
  profiles: ProfilesTab,
  devices: DevicesTab,
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
  const availableTabs = currentProfile?.role === 'admin_player' ? TABS : TABS.filter((tab) => tab.id === 'profiles' || tab.id === 'devices' || tab.id === 'appearance')
  const fallbackTab = currentProfile?.role === 'admin_player' ? 'integrations' : 'devices'
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
        <p className="text-sm text-mga-muted mt-1">
          Manage integrations, devices, updates, cached source data, plugins, and appearance
        </p>
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
