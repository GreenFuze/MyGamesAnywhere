import { useQuery } from '@tanstack/react-query'
import { listPlugins } from '@/api/client'
import { Badge } from '@/components/ui/badge'

export function PluginsTab() {
  const { data: plugins = [], isLoading } = useQuery({
    queryKey: ['plugins'],
    queryFn: listPlugins,
  })

  if (isLoading) {
    return <div className="text-mga-muted text-sm py-8 text-center">Loading plugins...</div>
  }

  if (plugins.length === 0) {
    return (
      <div className="text-mga-muted text-sm py-12 text-center border border-mga-border rounded-mga bg-mga-surface">
        No plugins discovered. Make sure plugin binaries are in the plugins directory.
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <h3 className="text-sm font-medium text-mga-text">
        Discovered Plugins ({plugins.length})
      </h3>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
        {plugins.map((plugin) => (
          <div
            key={plugin.plugin_id}
            className="border border-mga-border rounded-mga bg-mga-surface p-4"
          >
            <div className="flex items-start justify-between">
              <div>
                <h4 className="font-medium text-mga-text">{plugin.plugin_id}</h4>
                <p className="text-xs text-mga-muted mt-0.5">v{plugin.plugin_version}</p>
              </div>
            </div>

            {/* Provides */}
            {plugin.provides.length > 0 && (
              <div className="mt-3">
                <span className="text-xs text-mga-muted uppercase tracking-wider">Provides</span>
                <div className="flex flex-wrap gap-1.5 mt-1">
                  {plugin.provides.map((p) => (
                    <Badge key={p} variant="accent">{p}</Badge>
                  ))}
                </div>
              </div>
            )}

            {/* Capabilities */}
            {plugin.capabilities.length > 0 && (
              <div className="mt-3">
                <span className="text-xs text-mga-muted uppercase tracking-wider">Capabilities</span>
                <div className="flex flex-wrap gap-1.5 mt-1">
                  {plugin.capabilities.map((cap) => (
                    <Badge key={cap} variant="muted">{cap}</Badge>
                  ))}
                </div>
              </div>
            )}

            {/* Config schema indicator */}
            {plugin.config && Object.keys(plugin.config).length > 0 && (
              <div className="mt-3">
                <span className="text-xs text-mga-muted">Has config schema</span>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
