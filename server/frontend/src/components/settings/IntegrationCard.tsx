import { useState } from "react";
import type {
  Integration,
  IntegrationStatusEntry,
  PluginInfo,
  SyncStatus,
} from "@/api/client";
import { pluginLabel, ConfigSummaryBuilder } from "@/lib/gameUtils";
import { useDateTimeFormat } from "@/hooks/useDateTimeFormat";
import { PluginIcon } from "./PluginIcon";
import { IntegrationGamesList } from "./IntegrationGamesList";
import { StatusDot } from "@/components/ui/status-dot";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ProgressBar } from "@/components/ui/progress-bar";
import { ConfirmDialog } from "@/components/ui/dialog";
import { Loader2, ChevronDown, ChevronRight } from "lucide-react";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export type IntegrationScanState = {
  active?: boolean;
  badge?: string;
  badgeVariant?: "default" | "accent" | "muted";
  badgeClassName?: string;
  detail?: string;
  progress?: {
    progress: number;
    total: number;
    indeterminate?: boolean;
    label?: string;
  };
};

interface IntegrationCardProps {
  integration: Integration;
  plugin: PluginInfo | undefined;
  status: IntegrationStatusEntry | undefined;
  isChecking: boolean;
  capability: string;
  gameCount?: number;
  onCheck: (id: string) => void;
  onEdit: (integration: Integration) => void;
  onDelete: (integration: Integration) => void;
  mutationDisabled?: boolean;

  // Source-specific props.
  scanState?: IntegrationScanState;
  scanDisabled?: boolean;
  onScan?: (id: string) => void;
  refreshState?: IntegrationScanState;
  refreshDisabled?: boolean;
  onRefresh?: (id: string) => void;

  // Sync-specific props.
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

  // Save-sync-specific props.
  activeSaveSyncIntegrationId?: string | null;
  onSetActiveSaveSync?: (integrationId: string) => void;
  onStartAuth?: (integration: Integration) => void;
  authPending?: boolean;
}

function supportsOAuth(plugin: PluginInfo | undefined): boolean {
  return plugin?.provides?.includes("auth.oauth.callback") ?? false;
}

function statusPresentation(
  status: IntegrationStatusEntry | undefined,
): {
  label: string;
  detail: string;
  className: string;
} {
  switch (status?.status) {
    case "ok":
      return {
        label: "Connected",
        detail:
          status.message || "Configuration validated and the integration is ready.",
        className: "text-green-400",
      };
    case "oauth_required":
      return {
        label: "Sign-in required",
        detail:
          status.message ||
          "This integration needs browser sign-in before it can be used.",
        className: "text-amber-300",
      };
    case "unavailable":
      return {
        label: "Unavailable",
        detail:
          status.message ||
          "The plugin is not available in this server session.",
        className: "text-amber-300",
      };
    case "error":
      return {
        label: "Error",
        detail:
          status.message ||
          "The integration configuration did not validate successfully.",
        className: "text-red-400",
      };
    default:
      return {
        label: "Pending",
        detail: "Status has not been checked yet in this session.",
        className: "text-mga-muted",
      };
  }
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function IntegrationCard({
  integration,
  plugin,
  status,
  isChecking,
  capability,
  gameCount,
  onCheck,
  onEdit,
  onDelete,
  mutationDisabled,
  scanState,
  scanDisabled,
  onScan,
  refreshState,
  refreshDisabled,
  onRefresh,
  syncStatus,
  syncState,
  onPush,
  onPull,
  onStoreKey,
  onClearKey,
  activeSaveSyncIntegrationId,
  onSetActiveSaveSync,
  onStartAuth,
  authPending,
}: IntegrationCardProps) {
  const summary = ConfigSummaryBuilder.summarize(
    integration.plugin_id,
    integration.config_json,
  );
  const primaryCapability =
    plugin?.capabilities?.[0] ?? integration.integration_type;
  const [expanded, setExpanded] = useState(false);
  const { format: formatDT } = useDateTimeFormat();

  // Sync passphrase local state.
  const [passphrase, setPassphrase] = useState("");
  const [confirmPush, setConfirmPush] = useState(false);

  // Determine if this card is expandable (has a games list or sync controls).
  const isExpandable =
    capability === "source" ||
    capability === "metadata" ||
    capability === "sync";
  const isActiveSaveSync =
    capability === "save_sync" &&
    activeSaveSyncIntegrationId === integration.id;
  const oauthCapable = supportsOAuth(plugin);
  const statusInfo = statusPresentation(status);
  const operationState = refreshState ?? scanState;
  const refreshable =
    typeof onRefresh === "function" &&
    ((plugin?.provides?.includes("metadata.game.lookup") ?? false) ||
      (plugin?.provides?.includes("achievements.game.get") ?? false));
  const showAuthAction =
    oauthCapable &&
    onStartAuth &&
    (!status || status.status === "oauth_required");
  const authLabel =
    status?.status === "oauth_required" ? "Re-auth" : "Connect";

  return (
    <div className="border border-mga-border rounded-mga bg-mga-surface p-4 flex flex-col gap-2 transition-colors hover:border-mga-muted/50">
      {/* Top row: icon + label + game count + status */}
      <div className="flex items-center gap-2.5">
        <PluginIcon
          pluginId={integration.plugin_id}
          size={22}
          className="text-mga-accent shrink-0"
        />
        <span className="font-medium text-mga-text truncate flex-1">
          {integration.label}
        </span>

        {/* Game count badge for source/metadata */}
        {gameCount != null && gameCount > 0 && (
          <Badge variant="muted" className="text-[10px] shrink-0">
            {capability === "metadata"
              ? `enriched ${gameCount}`
              : `${gameCount} games`}
          </Badge>
        )}
        {isActiveSaveSync && (
          <Badge variant="accent" className="text-[10px] shrink-0">
            Active
          </Badge>
        )}

        {isChecking ? (
          <Loader2
            size={14}
            className="text-mga-accent animate-spin shrink-0"
          />
        ) : (
          <StatusDot
            status={status?.status ?? "pending"}
            className="shrink-0"
          />
        )}
      </div>

      {/* Plugin label + capability badge */}
      <div className="flex items-center gap-2 text-xs">
        <span className="text-mga-muted">
          {pluginLabel(integration.plugin_id)}
        </span>
        {primaryCapability && (
          <Badge variant="muted">{primaryCapability}</Badge>
        )}
      </div>

      {operationState?.badge && (
        <div className="flex items-center gap-2 text-xs">
          <Badge
            variant={operationState.badgeVariant ?? "muted"}
            className={operationState.badgeClassName}
          >
            {operationState.badge}
          </Badge>
          {operationState.detail && (
            <span className="text-mga-muted truncate" title={operationState.detail}>
              {operationState.detail}
            </span>
          )}
        </div>
      )}

      {/* Config summary */}
      {summary && (
        <p
          className="text-xs text-mga-muted font-mono truncate"
          title={summary}
        >
          {summary}
        </p>
      )}

      {/* Sync status indicators */}
      {capability === "sync" && syncStatus && (
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs">
          <span className="text-mga-muted">
            Configured:{" "}
            <StatusDot
              status={syncStatus.configured ? "ok" : "unavailable"}
              label={syncStatus.configured ? "Yes" : "No"}
            />
          </span>
          <span className="text-mga-muted">
            Key:{" "}
            <StatusDot
              status={syncStatus.has_stored_key ? "ok" : "unavailable"}
              label={syncStatus.has_stored_key ? "Stored" : "None"}
            />
          </span>
          {syncStatus.last_push && (
            <span className="text-mga-muted">
              Last push:{" "}
              <span className="text-mga-text">
                {formatDT(syncStatus.last_push)}
              </span>
            </span>
          )}
          {syncStatus.last_pull && (
            <span className="text-mga-muted">
              Last pull:{" "}
              <span className="text-mga-text">
                {formatDT(syncStatus.last_pull)}
              </span>
            </span>
          )}
        </div>
      )}

      <div className="space-y-1">
        <p className={`text-xs font-medium uppercase tracking-wide ${statusInfo.className}`}>
          {statusInfo.label}
        </p>
        <p className={`text-xs ${statusInfo.className} truncate`} title={statusInfo.detail}>
          {statusInfo.detail}
        </p>
      </div>

      {operationState?.progress && (
        <ProgressBar
          value={
            operationState.progress.indeterminate || operationState.progress.total <= 0
              ? undefined
              : (operationState.progress.progress / operationState.progress.total) * 100
          }
          label={operationState.progress.label ?? "Working..."}
        />
      )}

      {/* Sync messages */}
      {capability === "sync" && syncState?.message && (
        <p className="text-xs text-mga-accent truncate">{syncState.message}</p>
      )}
      {capability === "sync" && syncState?.error && (
        <p className="text-xs text-red-400 truncate">{syncState.error}</p>
      )}

      {/* Actions */}
      <div className="flex items-center gap-2 mt-1 pt-2 border-t border-mga-border/50">
        <Button
          variant="outline"
          size="sm"
          onClick={() => onCheck(integration.id)}
          disabled={isChecking}
          className="text-xs"
        >
          {isChecking ? "Checking..." : "Check"}
        </Button>

        {/* Scan button for source integrations */}
        {capability === "source" && onScan && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => onScan(integration.id)}
            disabled={scanDisabled || scanState?.active}
            className="text-xs"
          >
            {scanState?.active ? "Scanning..." : "Scan"}
          </Button>
        )}

        {refreshable && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => onRefresh(integration.id)}
            disabled={refreshDisabled || refreshState?.active}
            className="text-xs"
          >
            {refreshState?.active ? "Refreshing..." : "Refresh"}
          </Button>
        )}

        {/* Push / Pull buttons for sync integrations */}
        {capability === "sync" && onPush && onPull && (
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setConfirmPush(true)}
              disabled={syncState?.pushing || syncState?.pulling}
              className="text-xs"
            >
              {syncState?.pushing ? "Pushing..." : "Push"}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={onPull}
              disabled={syncState?.pushing || syncState?.pulling}
              className="text-xs"
            >
              {syncState?.pulling ? "Pulling..." : "Pull"}
            </Button>
          </>
        )}

        {capability === "save_sync" && onSetActiveSaveSync && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => onSetActiveSaveSync(integration.id)}
            className="text-xs"
            disabled={isActiveSaveSync}
          >
            {isActiveSaveSync ? "Active" : "Use for Save Sync"}
          </Button>
        )}
        {showAuthAction && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => onStartAuth(integration)}
            className="text-xs"
            disabled={mutationDisabled || isChecking || authPending}
          >
            {authPending ? "Connecting..." : authLabel}
          </Button>
        )}

        <Button
          variant="ghost"
          size="sm"
          onClick={() => onEdit(integration)}
          className="text-xs"
          disabled={mutationDisabled}
        >
          Edit
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="text-xs text-red-400 hover:text-red-300"
          onClick={() => onDelete(integration)}
          disabled={mutationDisabled}
        >
          Delete
        </Button>

        {/* Expand/collapse toggle */}
        {isExpandable && (
          <button
            type="button"
            onClick={() => setExpanded(!expanded)}
            className="ml-auto text-mga-muted hover:text-mga-text transition-colors p-1"
          >
            {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </button>
        )}
      </div>

      {/* Expandable content: games list (source / metadata) */}
      {(capability === "source" || capability === "metadata") && (
        <IntegrationGamesList
          integrationId={integration.id}
          type={capability}
          expanded={expanded}
        />
      )}

      {/* Expandable content: sync controls (passphrase management) */}
      {capability === "sync" && expanded && onStoreKey && onClearKey && (
        <div className="space-y-3 pt-2 border-t border-mga-border/50">
          <div className="flex gap-2 items-end">
            <div className="flex-1">
              <Input
                label="Encryption Passphrase"
                type="password"
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
                placeholder="Enter passphrase..."
              />
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                onStoreKey(passphrase);
                setPassphrase("");
              }}
              disabled={!passphrase}
            >
              Store Key
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="text-red-400 hover:text-red-300"
              onClick={onClearKey}
              disabled={!syncStatus?.has_stored_key}
            >
              Clear Key
            </Button>
          </div>
        </div>
      )}

      {/* Push confirmation dialog */}
      {capability === "sync" && (
        <ConfirmDialog
          open={confirmPush}
          onClose={() => setConfirmPush(false)}
          onConfirm={() => {
            onPush?.();
            setConfirmPush(false);
          }}
          title="Push to Cloud"
          message="This will upload your current integrations and settings, overwriting the remote copy. Continue?"
          confirmLabel="Push"
        />
      )}
    </div>
  );
}
