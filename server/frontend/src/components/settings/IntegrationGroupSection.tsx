import { useState } from 'react'
import type { Integration, IntegrationStatusEntry, PluginInfo, LibraryStats, SyncStatus } from '@/api/client'
import { CAPABILITY_META } from '@/lib/gameUtils'
import { PluginIcon } from './PluginIcon'
import { IntegrationCard } from './IntegrationCard'
import { StatusDot } from '@/components/ui/status-dot'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ChevronDown, ChevronRight } from 'lucide-react'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface IntegrationGroupSectionProps {
  capability: string
  integrations: Integration[]
  plugins: PluginInfo[]
  statusMap: Map<string, IntegrationStatusEntry>
  checkingIds: Set<string>
  onCheck: (id: string) => void
  onEdit: (integration: Integration) => void
  onDelete: (integration: Integration) => void

  // Scan / metadata.
  stats?: LibraryStats
  scanningIds?: Set<string>
  integrationProgress?: Map<string, { progress: number; total: number }>
  onScan?: (id: string) => void
  onScanGroup?: () => void
  onRefreshMetadata?: () => void
  scanning?: boolean

  // Sync.
  syncStatus?: SyncStatus
  syncState?: { pushing: boolean; pulling: boolean; message: string; error: string }
  onPush?: () => void
  onPull?: () => void
  onStoreKey?: (passphrase: string) => void
  onClearKey?: () => void
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/** Collapsible accordion section grouping integrations by capability. */
export function IntegrationGroupSection({
  capability,
  integrations,
  plugins,
  statusMap,
  checkingIds,
  onCheck,
  onEdit,
  onDelete,
  stats,
  scanningIds,
  integrationProgress,
  onScan,
  onScanGroup,
  onRefreshMetadata,
  scanning,
  syncStatus,
  syncState,
  onPush,
  onPull,
  onStoreKey,
  onClearKey,
}: IntegrationGroupSectionProps) {
  const [expanded, setExpanded] = useState(true)

  const meta = CAPABILITY_META[capability] ?? { label: capability, icon: 'Puzzle', order: 99 }
  const pluginMap = new Map(plugins.map((p) => [p.plugin_id, p]))

  // Compute aggregate health dot.
  const aggregateStatus = computeAggregateStatus(integrations, statusMap)

  // Resolve game count for a card based on capability.
  const getGameCount = (integ: Integration): number | undefined => {
    if (!stats) return undefined

    if (capability === 'source') {
      return stats.by_integration_id?.[integ.id] ?? 0
    }
    if (capability === 'metadata') {
      return stats.by_metadata_plugin_id?.[integ.plugin_id] ?? 0
    }
    return undefined
  }

  return (
    <div className="border border-mga-border rounded-mga overflow-hidden">
      {/* Accordion header */}
      <div className="flex items-center bg-mga-elevated">
        <button
          type="button"
          onClick={() => setExpanded(!expanded)}
          className="flex-1 flex items-center gap-3 px-4 py-3 hover:bg-mga-elevated/80 transition-colors text-left"
        >
          {expanded ? (
            <ChevronDown size={16} className="text-mga-muted shrink-0" />
          ) : (
            <ChevronRight size={16} className="text-mga-muted shrink-0" />
          )}
          <PluginIcon capability={capability} size={18} className="text-mga-accent shrink-0" />
          <span className="font-medium text-mga-text flex-1">{meta.label}</span>
          <Badge variant="muted">{integrations.length}</Badge>
          <StatusDot status={aggregateStatus} />
        </button>

        {/* Group action buttons */}
        <div className="flex items-center gap-2 pr-4">
          {capability === 'source' && onScanGroup && (
            <Button variant="outline" size="sm" onClick={onScanGroup} disabled={scanning} className="text-xs">
              {scanning ? 'Scanning...' : 'Scan All Sources'}
            </Button>
          )}
          {capability === 'metadata' && onRefreshMetadata && (
            <Button variant="outline" size="sm" onClick={onRefreshMetadata} disabled={scanning} className="text-xs">
              {scanning ? 'Refreshing...' : 'Refresh Metadata'}
            </Button>
          )}
        </div>
      </div>

      {/* Accordion body */}
      {expanded && (
        <div className="p-4 grid grid-cols-1 lg:grid-cols-2 gap-3">
          {integrations.map((integ) => (
            <IntegrationCard
              key={integ.id}
              integration={integ}
              plugin={pluginMap.get(integ.plugin_id)}
              status={statusMap.get(integ.id)}
              isChecking={checkingIds.has(integ.id)}
              capability={capability}
              gameCount={getGameCount(integ)}
              onCheck={onCheck}
              onEdit={onEdit}
              onDelete={onDelete}
              // Source-specific.
              isScanning={scanningIds?.has(integ.id)}
              scanProgress={integrationProgress?.get(integ.id)}
              onScan={onScan}
              // Sync-specific.
              syncStatus={capability === 'sync' ? syncStatus : undefined}
              syncState={capability === 'sync' ? syncState : undefined}
              onPush={capability === 'sync' ? onPush : undefined}
              onPull={capability === 'sync' ? onPull : undefined}
              onStoreKey={capability === 'sync' ? onStoreKey : undefined}
              onClearKey={capability === 'sync' ? onClearKey : undefined}
            />
          ))}
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Compute an aggregate status for a group of integrations. */
function computeAggregateStatus(
  integrations: Integration[],
  statusMap: Map<string, IntegrationStatusEntry>,
): 'ok' | 'error' | 'unavailable' | 'pending' {
  let hasError = false
  let hasUnavailable = false
  let allChecked = true

  for (const integ of integrations) {
    const s = statusMap.get(integ.id)
    if (!s) {
      allChecked = false
      continue
    }
    if (s.status === 'error') hasError = true
    if (s.status === 'unavailable') hasUnavailable = true
  }

  if (!allChecked && !hasError && !hasUnavailable) return 'pending'
  if (hasError) return 'error'
  if (hasUnavailable) return 'unavailable'
  return 'ok'
}
