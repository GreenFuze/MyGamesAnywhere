import { Navigate, useSearchParams } from 'react-router'
import { Tabs, type Tab } from '@/components/ui/tabs'
import { IntegrationsTab } from '@/components/settings/IntegrationsTab'
import { PluginsTab } from '@/components/settings/PluginsTab'
import { AppearanceTab } from '@/components/settings/AppearanceTab'
import { CacheTab } from '@/components/settings/CacheTab'
import { ProfilesTab } from '@/components/settings/ProfilesTab'
import { UpdateTab } from '@/components/settings/SettingsTab'
import { MySettingsTab } from '@/components/settings/MySettingsTab'
import { useProfiles } from '@/hooks/useProfiles'
import {
  resolveSettingsRoute,
  type SettingsTabId,
} from '@/lib/navigationRoutes'

const TABS: Tab[] = [
  { id: 'my-settings', label: 'My Settings' },
  { id: 'integrations', label: 'Connections' },
  { id: 'devices', label: 'Devices' },
  { id: 'emulators', label: 'Emulators' },
  { id: 'profiles', label: 'Profiles' },
  { id: 'cache', label: 'Storage' },
  { id: 'appearance', label: 'Appearance' },
  { id: 'update', label: 'Updates' },
  { id: 'plugins', label: 'Advanced' },
]

const TAB_COMPONENTS: Record<string, React.FC> = {
  'my-settings': MySettingsTab,
  update: UpdateTab,
  profiles: ProfilesTab,
  plugins: PluginsTab,
  cache: CacheTab,
  appearance: AppearanceTab,
}

export function SettingsPage() {
  const { currentProfile } = useProfiles()
  const [searchParams, setSearchParams] = useSearchParams()
  const isAdmin = currentProfile?.role === 'admin_player'
  const routeResolution = resolveSettingsRoute(searchParams, isAdmin)
  if (routeResolution.redirectTo) {
    return <Navigate to={routeResolution.redirectTo} replace />
  }
  const availableTabs = isAdmin ? TABS : TABS.filter((tab) => tab.id === 'my-settings' || tab.id === 'profiles' || tab.id === 'devices' || tab.id === 'emulators' || tab.id === 'appearance')
  const activeTab: SettingsTabId = routeResolution.activeTab

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
          <IntegrationsTab
            firstRunRestore={searchParams.get('first_run') === 'restore'}
            focusIntegrationId={searchParams.get('integration') ?? undefined}
            focusPluginId={searchParams.get('plugin') ?? undefined}
          />
        ) : ActiveComponent ? (
          <ActiveComponent />
        ) : null}
      </div>
    </div>
  )
}
