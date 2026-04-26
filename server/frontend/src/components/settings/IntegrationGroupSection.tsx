import { useState, type ReactNode } from "react";
import type {
  Integration,
  IntegrationStatusEntry,
  PluginInfo,
  LibraryStats,
  SyncStatus,
} from "@/api/client";
import { CAPABILITY_META } from "@/lib/gameUtils";
import { PluginIcon } from "./PluginIcon";
import { IntegrationCard, type IntegrationScanState } from "./IntegrationCard";
import { StatusDot } from "@/components/ui/status-dot";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ChevronDown, ChevronRight } from "lucide-react";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface IntegrationGroupSectionProps {
  capability: string;
  integrations: Integration[];
  plugins: PluginInfo[];
  statusMap: Map<string, IntegrationStatusEntry>;
  checkingIds: Set<string>;
  onCheck: (id: string) => void;
  onEdit: (integration: Integration) => void;
  onDelete: (integration: Integration) => void;

  // Scan / metadata.
  stats?: LibraryStats;
  scanStateByIntegrationId?: Map<string, IntegrationScanState>;
  refreshStateByIntegrationId?: Map<string, IntegrationScanState>;
  onScan?: (id: string) => void;
  onRefresh?: (id: string) => void;
  onScanGroup?: () => void;
  onRefreshMetadata?: () => void;
  scanControlsDisabled?: boolean;
  refreshControlsDisabled?: boolean;
  mutationControlsDisabled?: boolean;
  sourceScanActive?: boolean;
  metadataRefreshActive?: boolean;

  // Sync.
  syncStatus?: SyncStatus;
  syncState?: {
    pushing: boolean;
    pulling: boolean;
    message: string;
    error: string;
  };
  onPush?: () => void;
  onPull?: () => void;
  onStoreKey?: (passphrase: string) => void;
  onClearKey?: () => void;

  // Save sync.
  activeSaveSyncIntegrationId?: string | null;
  onSetActiveSaveSync?: (integrationId: string) => void;
  saveSyncHeaderControls?: ReactNode;
  onStartAuth?: (integration: Integration) => void;
  authPendingIds?: Set<string>;
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
  scanStateByIntegrationId,
  refreshStateByIntegrationId,
  onScan,
  onRefresh,
  onScanGroup,
  onRefreshMetadata,
  scanControlsDisabled,
  refreshControlsDisabled,
  mutationControlsDisabled,
  sourceScanActive,
  metadataRefreshActive,
  syncStatus,
  syncState,
  onPush,
  onPull,
  onStoreKey,
  onClearKey,
  activeSaveSyncIntegrationId,
  onSetActiveSaveSync,
  saveSyncHeaderControls,
  onStartAuth,
  authPendingIds,
}: IntegrationGroupSectionProps) {
  const [expanded, setExpanded] = useState(true);

  const meta = CAPABILITY_META[capability] ?? {
    label: capability,
    icon: "Puzzle",
    order: 99,
  };
  const pluginMap = new Map(plugins.map((p) => [p.plugin_id, p]));

  // Compute aggregate health dot.
  const aggregateStatus = computeAggregateStatus(integrations, statusMap);

  // Resolve game count for a card based on capability.
  const getGameCount = (integ: Integration): number | undefined => {
    if (!stats) return undefined;

    if (capability === "source") {
      return stats.by_integration_id?.[integ.id] ?? 0;
    }
    if (capability === "metadata") {
      return stats.by_metadata_plugin_id?.[integ.plugin_id] ?? 0;
    }
    return undefined;
  };

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
          <PluginIcon
            capability={capability}
            size={18}
            className="text-mga-accent shrink-0"
          />
          <span className="font-medium text-mga-text flex-1">{meta.label}</span>
          <Badge variant="muted">{integrations.length}</Badge>
          <StatusDot status={aggregateStatus} />
        </button>

        {/* Group action buttons */}
        <div className="flex items-center gap-2 pr-4">
          {capability === "source" && onScanGroup && (
            <Button
              variant="outline"
              size="sm"
              onClick={onScanGroup}
              disabled={scanControlsDisabled}
              className="text-xs"
            >
              {sourceScanActive ? "Rescanning sources..." : "Rescan All"}
            </Button>
          )}
          {capability === "metadata" && onRefreshMetadata && (
            <Button
              variant="outline"
              size="sm"
              onClick={onRefreshMetadata}
              disabled={scanControlsDisabled}
              className="text-xs"
            >
              {metadataRefreshActive
                ? "Refreshing metadata..."
                : "Refresh Metadata"}
            </Button>
          )}
          {capability === "save_sync" && saveSyncHeaderControls}
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
              mutationDisabled={mutationControlsDisabled}
              // Source-specific.
              scanState={scanStateByIntegrationId?.get(integ.id)}
              refreshState={refreshStateByIntegrationId?.get(integ.id)}
              scanDisabled={
                capability === "source" ? scanControlsDisabled : undefined
              }
              onScan={onScan}
              refreshDisabled={refreshControlsDisabled}
              onRefresh={onRefresh}
              // Sync-specific.
              syncStatus={capability === "sync" ? syncStatus : undefined}
              syncState={capability === "sync" ? syncState : undefined}
              onPush={capability === "sync" ? onPush : undefined}
              onPull={capability === "sync" ? onPull : undefined}
              onStoreKey={capability === "sync" ? onStoreKey : undefined}
              onClearKey={capability === "sync" ? onClearKey : undefined}
              activeSaveSyncIntegrationId={
                capability === "save_sync"
                  ? activeSaveSyncIntegrationId
                  : undefined
              }
              onSetActiveSaveSync={
                capability === "save_sync" ? onSetActiveSaveSync : undefined
              }
              onStartAuth={onStartAuth}
              authPending={authPendingIds?.has(integ.id)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Compute an aggregate status for a group of integrations. */
function computeAggregateStatus(
  integrations: Integration[],
  statusMap: Map<string, IntegrationStatusEntry>,
): "ok" | "error" | "unavailable" | "oauth_required" | "pending" {
  let hasError = false;
  let hasUnavailable = false;
  let hasOAuthRequired = false;
  let allChecked = true;

  for (const integ of integrations) {
    const s = statusMap.get(integ.id);
    if (!s) {
      allChecked = false;
      continue;
    }
    if (s.status === "error") hasError = true;
    if (s.status === "unavailable") hasUnavailable = true;
    if (s.status === "oauth_required") hasOAuthRequired = true;
  }

  if (!allChecked && !hasError && !hasUnavailable) return "pending";
  if (hasError) return "error";
  if (hasOAuthRequired) return "oauth_required";
  if (hasUnavailable) return "unavailable";
  return "ok";
}
