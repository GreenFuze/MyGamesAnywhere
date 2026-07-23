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
import { SecretInput } from "@/components/ui/secret-input";
import { ProgressBar } from "@/components/ui/progress-bar";
import { ConfirmDialog } from "@/components/ui/dialog";
import { ActionMenu, type ActionMenuItem } from "@/components/ui/action-menu";
import { StatusPill, type StatusTone } from "@/components/ui/status-pill";
import { Tooltip } from "@/components/ui/tooltip";
import { Loader2, ChevronDown, ChevronRight } from "lucide-react";
import { FileValidationDialog } from "./FileValidationDialog";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export type IntegrationScanState = {
  active?: boolean;
  badge?: string;
  badgeVariant?: "default" | "accent" | "muted";
  badgeClassName?: string;
  detail?: string;
  statusLabel?: string;
  statusDetail?: string;
  statusClassName?: string;
  statusDot?: "ok" | "oauth_required" | "unavailable" | "error" | "pending";
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
  onStoreKey?: (passphrase: string, currentPassphrase?: string) => void;
  onClearKey?: () => void;

  // Save-sync-specific props.
  activeSaveSyncIntegrationId?: string | null;
  onSetActiveSaveSync?: (integrationId: string) => void;
  onStartAuth?: (integration: Integration, options?: { force?: boolean }) => void;
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
  tone: StatusTone;
} {
  switch (status?.status) {
    case "ok":
      return {
        label: "Connected",
        detail:
          status.message || "Configuration validated and the integration is ready.",
        className: "text-green-400",
        tone: "success",
      };
    case "oauth_required":
      return {
        label: "Sign-in required",
        detail:
          status.message ||
          "This integration needs browser sign-in before it can be used.",
        className: "text-amber-300",
        tone: "warning",
      };
    case "unavailable":
      return {
        label: "Unavailable",
        detail:
          status.message ||
          "The plugin is not available in this server session.",
        className: "text-amber-300",
        tone: "warning",
      };
    case "error":
      return {
        label: "Error",
        detail:
          status.message ||
          "The integration configuration did not validate successfully.",
        className: "text-red-400",
        tone: "danger",
      };
    default:
      return {
        label: "Pending",
        detail: "Status has not been checked yet in this session.",
        className: "text-mga-muted",
        tone: "muted",
      };
  }
}

function toneForStatus(status: string): StatusTone {
  if (status === "ok") return "success";
  if (status === "error") return "danger";
  if (status === "oauth_required" || status === "unavailable") return "warning";
  return "muted";
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
  const [expanded, setExpanded] = useState(false);
  const [fileValidationOpen, setFileValidationOpen] = useState(false);
  const { format: formatDT } = useDateTimeFormat();

  // Sync passphrase local state.
  const [passphrase, setPassphrase] = useState("");
  const [currentPassphrase, setCurrentPassphrase] = useState("");
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
  const refreshesMetadata = plugin?.provides?.includes("metadata.game.lookup") ?? false;
  const refreshesAchievements = plugin?.provides?.includes("achievements.game.get") ?? false;
  const supportsFileValidation =
    capability === "source" &&
    (plugin?.provides?.includes("source.filesystem.list") ?? false);
  const refreshLabel = refreshesMetadata && refreshesAchievements
    ? "Refresh game info"
    : refreshesAchievements
      ? "Refresh achievements"
      : "Refresh game info";
  const refreshBusyLabel = "Refreshing…";
  const showAuthAction =
    oauthCapable &&
    onStartAuth &&
    (!status ||
      status.status === "oauth_required" ||
      status.status === "error" ||
      refreshState?.statusDot === "oauth_required");
  const authLabel =
    status?.status === "oauth_required" || status?.status === "error" || refreshState?.statusDot === "oauth_required"
      ? "Re-auth"
      : "Connect";
  const effectiveStatusInfo =
    refreshState?.statusLabel || refreshState?.statusDetail
      ? {
          label: refreshState.statusLabel ?? statusInfo.label,
          detail: refreshState.statusDetail ?? refreshState.detail ?? statusInfo.detail,
          className: refreshState.statusClassName ?? "text-amber-300",
          tone: toneForStatus(refreshState.statusDot ?? "pending"),
        }
      : statusInfo;
  const effectiveDotStatus = refreshState?.statusDot ?? status?.status ?? "pending";
  const showStatusDetail = effectiveDotStatus !== "ok" || Boolean(operationState?.active);

  const secondaryActions: ActionMenuItem[] = [
    {
      label: isChecking ? "Checking connection…" : "Check connection",
      onSelect: () => onCheck(integration.id),
      disabled: isChecking,
    },
  ];
  if (refreshable && capability === "source") {
    secondaryActions.push({
      label: refreshState?.active ? refreshBusyLabel : refreshLabel,
      onSelect: () => onRefresh?.(integration.id),
      disabled: refreshDisabled || refreshState?.active,
    });
  }
  if (supportsFileValidation) {
    secondaryActions.push({
      label: "Check files",
      onSelect: () => setFileValidationOpen(true),
      disabled: mutationDisabled || Boolean(scanState?.active),
    });
  }
  if (capability === "sync" && onPull) {
    secondaryActions.push({
      label: syncState?.pulling ? "Restoring…" : "Restore settings",
      onSelect: onPull,
      disabled: syncState?.pushing || syncState?.pulling,
    });
  }
  secondaryActions.push(
    {
      label: "Edit connection",
      onSelect: () => onEdit(integration),
      disabled: mutationDisabled,
    },
    {
      label: "Remove connection",
      onSelect: () => onDelete(integration),
      disabled: mutationDisabled,
      danger: true,
    },
  );

  return (
    <div className="relative flex flex-col gap-3 rounded-mga border border-mga-border bg-mga-surface p-4 transition-colors hover:border-mga-muted/50">
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

        {/* Sync-imported integration not yet re-authorized on this device */}
        {integration.needs_reauth && (
          <Badge
            variant="muted"
            className="text-[10px] shrink-0 border border-amber-500/40 text-amber-300"
          >
            Auth Required
          </Badge>
        )}

        {isChecking ? (
          <Loader2
            size={14}
            className="text-mga-accent animate-spin shrink-0"
          />
        ) : (
          <StatusPill
            label={effectiveStatusInfo.label}
            detail={effectiveStatusInfo.detail}
            tone={effectiveStatusInfo.tone}
            className="shrink-0"
          />
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
      {(summary || pluginLabel(integration.plugin_id)) && (
        <p
          className="truncate text-xs text-mga-muted"
          title={summary || pluginLabel(integration.plugin_id)}
        >
          {summary || pluginLabel(integration.plugin_id)}
        </p>
      )}

      {/* Sync status indicators */}
      {capability === "sync" && syncStatus && expanded && (
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

      {showStatusDetail ? (
        <p className={`truncate text-xs ${effectiveStatusInfo.className}`} title={effectiveStatusInfo.detail}>
          {effectiveStatusInfo.detail}
        </p>
      ) : null}

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

      {/* One primary action; secondary actions stay in More. */}
      <div className="mt-1 flex items-center gap-2 border-t border-mga-border/50 pt-3">
        {showAuthAction ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => onStartAuth?.(integration, { force: authLabel === "Re-auth" })}
            disabled={mutationDisabled || isChecking || authPending}
            className="text-xs"
          >
            {authPending ? "Connecting…" : authLabel}
          </Button>
        ) : capability === "source" && onScan ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => onScan(integration.id)}
            disabled={scanDisabled || scanState?.active}
            className="text-xs"
          >
            {scanState?.active ? "Scanning…" : "Rescan"}
          </Button>
        ) : refreshable ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => onRefresh?.(integration.id)}
            disabled={refreshDisabled || refreshState?.active}
            className="text-xs"
          >
            {refreshState?.active ? refreshBusyLabel : "Refresh"}
          </Button>
        ) : capability === "sync" && onPush ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setConfirmPush(true)}
            disabled={syncState?.pushing || syncState?.pulling}
            className="text-xs"
          >
            {syncState?.pushing ? "Backing up…" : "Back up"}
          </Button>
        ) : capability === "save_sync" && onSetActiveSaveSync && !isActiveSaveSync ? (
          <Button variant="outline" size="sm" onClick={() => onSetActiveSaveSync(integration.id)}>
            Use for saves
          </Button>
        ) : null}

        {isExpandable && (
          <Tooltip content={expanded ? "Hide details" : "Show details"}>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => setExpanded(!expanded)}
              aria-expanded={expanded}
              className="text-xs"
            >
              {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
              Details
            </Button>
          </Tooltip>
        )}
        <ActionMenu items={secondaryActions} className="ml-auto" />
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
          <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto_auto] md:items-end">
            {syncStatus?.has_stored_key && (
              <SecretInput
                label="Current Passphrase"
                value={currentPassphrase}
                onChange={(e) => setCurrentPassphrase(e.target.value)}
                placeholder="Required to replace stored key"
              />
            )}
            <div className={syncStatus?.has_stored_key ? "" : "md:col-span-2"}>
              <SecretInput
                label={syncStatus?.has_stored_key ? "New Passphrase" : "Encryption Passphrase"}
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
                placeholder={syncStatus?.has_stored_key ? "Enter replacement passphrase..." : "Enter passphrase..."}
              />
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                onStoreKey(passphrase, currentPassphrase);
                setPassphrase("");
                setCurrentPassphrase("");
              }}
              disabled={!passphrase || (syncStatus?.has_stored_key && !currentPassphrase)}
            >
              {syncStatus?.has_stored_key ? "Replace Key" : "Store Key"}
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
          title="Back up settings"
          message="Replace the cloud backup with your current MGA connections and settings?"
          confirmLabel="Back up"
        />
      )}
      {supportsFileValidation ? (
        <FileValidationDialog
          integration={integration}
          open={fileValidationOpen}
          onClose={() => setFileValidationOpen(false)}
        />
      ) : null}
    </div>
  );
}
