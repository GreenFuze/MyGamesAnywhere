import { useSearchParams } from 'react-router-dom'
import { Tabs, type Tab } from '@/components/ui/tabs'
import { IntegrationsTab } from '@/components/settings/IntegrationsTab'
import { PluginsTab } from '@/components/settings/PluginsTab'
import { AppearanceTab } from '@/components/settings/AppearanceTab'
import { UndetectedGamesTab } from '@/components/settings/UndetectedGamesTab'
import { CacheTab } from '@/components/settings/CacheTab'

const TABS: Tab[] = [
  { id: 'integrations', label: 'Integrations' },
  { id: 'plugins', label: 'Plugins' },
  { id: 'cache', label: 'Cache' },
  { id: 'appearance', label: 'Appearance' },
  { id: 'undetected', label: 'Undetected Games' },
]

const TAB_COMPONENTS: Record<string, React.FC> = {
  integrations: IntegrationsTab,
  plugins: PluginsTab,
  cache: CacheTab,
  appearance: AppearanceTab,
  undetected: UndetectedGamesTab,
}

export function SettingsPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const activeTab = tabParam && TAB_COMPONENTS[tabParam] ? tabParam : 'integrations'

  const handleTabChange = (id: string) => {
    const next = new URLSearchParams(searchParams)
    next.set('tab', id)
    setSearchParams(next, { replace: true })
  }

  const ActiveComponent = TAB_COMPONENTS[activeTab] ?? IntegrationsTab

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-mga-text">Settings</h1>
        <p className="text-sm text-mga-muted mt-1">
          Manage integrations, cached source data, plugins, and appearance
        </p>
      </div>

      <Tabs tabs={TABS} active={activeTab} onChange={handleTabChange} />

      <div className="pb-8">
        <ActiveComponent />
      </div>
    </div>
  )
}
