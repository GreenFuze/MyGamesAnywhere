import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ApiError,
  listIntegrations,
  listPlugins,
  getIntegrationStatus,
  checkIntegrationStatus,
  deleteIntegration,
  triggerScan,
  cancelScanJob,
  getScanJob,
  getStats,
  getSyncStatus,
  syncPush,
  syncPull,
  storeKey,
  clearKey,
  getFrontendConfig,
  setFrontendConfig,
  startSaveSyncMigration,
  type Integration,
  type IntegrationStatusEntry,
  type PluginInfo,
  type ScanJobIntegrationStatus,
  type ScanJobMetadataProviderStatus,
  type ScanJobProgress,
  type ScanJobRecentEvent,
  type ScanJobStatus,
  type SaveSyncMigrationStatus,
} from "@/api/client";
import { CAPABILITY_META } from "@/lib/gameUtils";
import { useSSE } from "@/hooks/useSSE";
import { IntegrationGroupSection } from "./IntegrationGroupSection";
import { LibraryStatsSummary } from "./LibraryStatsSummary";
import { ScanSummary } from "./ScanSummary";
import { AddIntegrationWizard, EditIntegrationDialog } from "./IntegrationForm";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { ProgressBar } from "@/components/ui/progress-bar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Plus, RefreshCw } from "lucide-react";

// ---------------------------------------------------------------------------
// Scan progress type (migrated from ScanTab)
// ---------------------------------------------------------------------------

type ScanEventLogEntry = {
  id: string;
  ts: string;
  text: string;
};

type IntegrationScanState = {
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

type MetadataParticipationState = {
  integrationId: string;
  label?: string;
  pluginId?: string;
  status: string;
  phase?: string;
  reason?: string;
  error?: string;
  progress?: ScanJobProgress;
  sourceLabel?: string;
};

function readTimestamp(data: unknown): string {
  if (
    data &&
    typeof data === "object" &&
    typeof (data as { ts?: unknown }).ts === "string"
  ) {
    return (data as { ts: string }).ts;
  }
  return new Date().toISOString();
}

function formatLogTime(ts: string): string {
  const parsed = new Date(ts);
  if (Number.isNaN(parsed.getTime())) return "";
  return parsed.toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

const activeScanJobStorageKey = "mga.activeScanJobId";

function readStoredScanJobId(): string | null {
  if (typeof window === "undefined") return null;
  return window.sessionStorage.getItem(activeScanJobStorageKey);
}

function writeStoredScanJobId(jobId: string | null) {
  if (typeof window === "undefined") return;
  if (jobId) {
    window.sessionStorage.setItem(activeScanJobStorageKey, jobId);
  } else {
    window.sessionStorage.removeItem(activeScanJobStorageKey);
  }
}

function scanJobIsTerminal(job: Pick<ScanJobStatus, "status">) {
  return (
    job.status === "completed" ||
    job.status === "failed" ||
    job.status === "cancelled"
  );
}

function readJobId(data: unknown): string | null {
  if (
    data &&
    typeof data === "object" &&
    typeof (data as { job_id?: unknown }).job_id === "string"
  ) {
    return (data as { job_id: string }).job_id;
  }
  return null;
}

function cloneJobProgress(
  progress?: ScanJobProgress,
): ScanJobProgress | undefined {
  return progress ? { ...progress } : undefined;
}

function cloneMetadataProvider(
  provider: ScanJobMetadataProviderStatus,
): ScanJobMetadataProviderStatus {
  return {
    ...provider,
    status: normalizeScanStatus(provider.status),
    progress: cloneJobProgress(provider.progress),
  };
}

function cloneJobIntegration(
  integration: ScanJobIntegrationStatus,
): ScanJobIntegrationStatus {
  return {
    ...integration,
    status: normalizeScanStatus(integration.status),
    source_progress: cloneJobProgress(integration.source_progress),
    metadata_progress: cloneJobProgress(integration.metadata_progress),
    metadata_providers: integration.metadata_providers?.map(
      cloneMetadataProvider,
    ),
  };
}

function toIntegrationStateMap(
  integrations?: ScanJobIntegrationStatus[],
): Map<string, ScanJobIntegrationStatus> {
  const next = new Map<string, ScanJobIntegrationStatus>();
  for (const integration of integrations ?? []) {
    next.set(integration.integration_id, cloneJobIntegration(integration));
  }
  return next;
}

function ensureMetadataProviderState(
  integration: ScanJobIntegrationStatus,
  next: Pick<
    ScanJobMetadataProviderStatus,
    "integration_id" | "label" | "plugin_id"
  >,
): ScanJobMetadataProviderStatus {
  const providers = integration.metadata_providers ?? [];
  if (next.integration_id) {
    const existing = providers.find(
      (provider) => provider.integration_id === next.integration_id,
    );
    if (existing) {
      if (next.label) existing.label = next.label;
      if (next.plugin_id) existing.plugin_id = next.plugin_id;
      integration.metadata_providers = providers;
      return existing;
    }
  }

  if (!next.integration_id && next.plugin_id) {
    const existing = providers.find(
      (provider) =>
        !provider.integration_id && provider.plugin_id === next.plugin_id,
    );
    if (existing) {
      if (next.label) existing.label = next.label;
      integration.metadata_providers = providers;
      return existing;
    }
  }

  const provider: ScanJobMetadataProviderStatus = {
    integration_id: next.integration_id,
    label: next.label,
    plugin_id: next.plugin_id,
    status: "pending",
  };
  integration.metadata_providers = [...providers, provider];
  return provider;
}

function formatScanStatusText(job: ScanJobStatus): string {
  if (job.status === "cancelling") return "Cancelling scan...";
  if (job.current_integration_label) {
    return `${job.current_phase ?? "Scanning"}: ${job.current_integration_label}`;
  }
  return job.current_phase ?? "";
}

function toLogEntry(event: ScanJobRecentEvent): ScanEventLogEntry | null {
  if (!event.message) return null;
  const ts = event.ts ?? new Date().toISOString();
  return {
    id: `${ts}:${event.type}:${event.message}`,
    ts,
    text: event.message,
  };
}

function shouldAppendProgressEvent(current?: number, total?: number) {
  if (!current || current <= 0) return false;
  if (!total || total <= 0) return true;
  if (current === 1 || current === total) return true;
  return current % 25 === 0;
}

function shouldAppendMetadataProgressEvent(current?: number, total?: number) {
  if (!current || current <= 0) return false;
  if (!total || total <= 0) return true;
  if (current === total) return true;
  return current % 10 === 0;
}

function formatProgressLabel(
  progress?: ScanJobProgress,
  fallback = "Working...",
) {
  if (!progress) return fallback;
  const unit = progress.unit ? ` ${progress.unit}` : "";
  if (progress.indeterminate || progress.total == null || progress.total <= 0) {
    return `${progress.current ?? 0}${unit}`.trim() || fallback;
  }
  return `${progress.current ?? 0}/${progress.total}${unit}`.trim();
}

function formatMetadataPhase(phase?: string) {
  switch (phase) {
    case "identify":
      return "Identifying";
    case "consensus":
      return "Building consensus";
    case "fill":
      return "Filling gaps";
    case "finished":
      return "Completed";
    default:
      return phase?.replace(/_/g, " ") ?? "";
  }
}

function normalizeScanStatus(status?: string, fallback = "pending") {
  return status && status.trim().length > 0 ? status : fallback;
}

function formatSourceSkipReason(reason?: string) {
  switch (reason) {
    case "plugin_not_found":
      return "Source unavailable";
    case "invalid_config":
      return "Needs configuration";
    case "no_source_capability":
      return "Not a game source";
    case "no_games":
      return "No games found";
    default:
      return "Skipped";
  }
}

function formatSourceDetail(integration: ScanJobIntegrationStatus) {
  if (normalizeScanStatus(integration.status) === "skipped") {
    const reasonLabel = formatSourceSkipReason(integration.reason);
    if (integration.error) {
      return `${reasonLabel}: ${integration.error}`;
    }
    return reasonLabel;
  }
  if (integration.metadata_label) {
    const phase = formatMetadataPhase(integration.metadata_phase);
    return phase
      ? `Metadata via ${integration.metadata_label}: ${phase}`
      : `Metadata via ${integration.metadata_label}`;
  }
  if (integration.metadata_phase === "consensus") {
    return "Metadata: Building consensus";
  }
  switch (integration.phase) {
    case "queued":
      return "Waiting to start";
    case "starting":
      return "Preparing source scan";
    case "listing source content":
      return "Source scan: listing content";
    case "source listing complete":
      return "Source scan: listing complete";
    case "scanning files":
      return "Source scan";
    case "game detection complete":
      return "Source scan complete";
    case "metadata enrichment":
      return "Preparing metadata lookups";
    case "persisting results":
      return "Persisting results";
    case "completed":
      return "Completed";
    case "cancelling":
      return "Cancelling scan";
    case "cancelled":
      return "Cancelled";
    case "failed":
      return integration.error || "Scan failed";
    default:
      return integration.phase
        ? integration.phase.replace(/_/g, " ")
        : undefined;
  }
}

function sourceBadgePresentation(integration: ScanJobIntegrationStatus) {
  const status = normalizeScanStatus(integration.status);
  switch (status) {
    case "pending":
      return { badge: "Waiting to start", badgeVariant: "muted" as const };
    case "running":
      return { badge: "Running", badgeVariant: "accent" as const };
    case "cancelling":
      return { badge: "Cancelling", badgeVariant: "default" as const };
    case "completed":
      return { badge: "Completed", badgeVariant: "accent" as const };
    case "cancelled":
      return { badge: "Cancelled", badgeVariant: "default" as const };
    case "failed":
      return {
        badge: "Error",
        badgeVariant: "default" as const,
        badgeClassName: "bg-red-500/15 text-red-300",
      };
    case "skipped": {
      const label = formatSourceSkipReason(integration.reason);
      const needsAttention =
        integration.reason === "plugin_not_found" ||
        integration.reason === "invalid_config";
      return {
        badge: label,
        badgeVariant: needsAttention
          ? ("default" as const)
          : ("muted" as const),
        badgeClassName: needsAttention
          ? "bg-amber-500/15 text-amber-300"
          : undefined,
      };
    }
    default:
      return {
        badge: status.replace(/_/g, " "),
        badgeVariant: "muted" as const,
      };
  }
}

function sourceCardState(
  integration: ScanJobIntegrationStatus,
): IntegrationScanState {
  const status = normalizeScanStatus(integration.status);
  const badge = sourceBadgePresentation(integration);
  const showSourceProgress =
    status !== "skipped" && status !== "failed" && status !== "cancelled";
  return {
    active: status === "running" || status === "cancelling",
    ...badge,
    detail: formatSourceDetail(integration),
    progress:
      showSourceProgress && integration.source_progress
        ? {
            progress: integration.source_progress.current ?? 0,
            total: integration.source_progress.total ?? 0,
            indeterminate: integration.source_progress.indeterminate,
            label: `Source scan: ${formatProgressLabel(integration.source_progress, "Working...")}`,
          }
        : undefined,
  };
}

function metadataStatusRank(status?: string) {
  switch (normalizeScanStatus(status)) {
    case "running":
      return 5;
    case "error":
      return 4;
    case "completed":
      return 3;
    case "not_used":
      return 2;
    case "pending":
      return 1;
    default:
      return 0;
  }
}

function metadataCardState(
  participation: MetadataParticipationState,
): IntegrationScanState {
  const status = normalizeScanStatus(participation.status);
  switch (status) {
    case "running": {
      const phase = formatMetadataPhase(participation.phase);
      return {
        badge: "Looking up",
        badgeVariant: "accent",
        detail: participation.sourceLabel
          ? `Looking up metadata for ${participation.sourceLabel}${phase ? ` • ${phase}` : ""}`
          : `Looking up metadata${phase ? ` • ${phase}` : ""}`,
      };
    }
    case "pending":
      return {
        badge: "Participating",
        badgeVariant: "muted",
        detail: participation.sourceLabel
          ? `Selected for ${participation.sourceLabel}`
          : "Selected for this scan",
      };
    case "completed":
      return {
        badge: "Completed",
        badgeVariant: "accent",
        detail: participation.sourceLabel
          ? `Used during ${participation.sourceLabel}`
          : "Metadata lookup complete",
      };
    case "not_used":
      return {
        badge:
          participation.reason === "no_lookup_needed"
            ? "No lookup needed"
            : "Not used in this scan",
        badgeVariant: "muted",
      };
    case "error":
      if (participation.reason === "invalid_config") {
        return {
          badge: "Needs configuration",
          badgeVariant: "default",
          badgeClassName: "bg-amber-500/15 text-amber-300",
          detail: participation.error,
        };
      }
      return {
        badge: "Error",
        badgeVariant: "default",
        badgeClassName: "bg-red-500/15 text-red-300",
        detail: participation.error || "Metadata lookup failed",
      };
    default:
      return {
        badge: status.replace(/_/g, " "),
        badgeVariant: "muted",
      };
  }
}

function formatMetadataPanelLabel(integration: ScanJobIntegrationStatus) {
  const provider = integration.metadata_label ?? integration.metadata_plugin_id;
  const phase = formatMetadataPhase(integration.metadata_phase);
  const prefix = provider ? `Metadata via ${provider}` : "Metadata";
  if (phase) {
    return `${prefix} (${phase})`;
  }
  return prefix;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function IntegrationsTab() {
  const queryClient = useQueryClient();
  const { subscribe, connected } = useSSE();
  const scanEventLogRef = useRef<HTMLDivElement | null>(null);
  const scanEventLogShouldStickRef = useRef(true);
  const scanEventLogLastKeyRef = useRef<string | null>(null);

  // ── Data queries ──

  const { data: integrations = [], isLoading: loadingIntegrations } = useQuery({
    queryKey: ["integrations"],
    queryFn: listIntegrations,
  });

  const { data: plugins = [] } = useQuery({
    queryKey: ["plugins"],
    queryFn: listPlugins,
  });

  const { data: stats } = useQuery({
    queryKey: ["stats"],
    queryFn: getStats,
  });

  const { data: syncStatus } = useQuery({
    queryKey: ["sync", "status"],
    queryFn: getSyncStatus,
  });

  const { data: frontendConfig = {} } = useQuery({
    queryKey: ["frontend-config"],
    queryFn: getFrontendConfig,
  });

  // ── Status check state ──

  const [statusMap, setStatusMap] = useState<
    Map<string, IntegrationStatusEntry>
  >(new Map());
  const [checkingAll, setCheckingAll] = useState(false);
  const [checkProgress, setCheckProgress] = useState({ current: 0, total: 0 });
  const [checkingIds, setCheckingIds] = useState<Set<string>>(new Set());

  // ── Scan state (absorbed from ScanTab) ──

  const [scanning, setScanning] = useState(false);
  const [scanJobStatus, setScanJobStatus] = useState("idle");
  const [scanMetadataOnly, setScanMetadataOnly] = useState(false);
  const [activeScanJobId, setActiveScanJobId] = useState<string | null>(() =>
    readStoredScanJobId(),
  );
  const [scanError, setScanError] = useState("");
  const [currentPhase, setCurrentPhase] = useState("");
  const [scanStatusText, setScanStatusText] = useState("");
  const [scanTotalCount, setScanTotalCount] = useState(0);
  const [scanCompletedCount, setScanCompletedCount] = useState(0);
  const [scanIntegrations, setScanIntegrations] = useState<
    Map<string, ScanJobIntegrationStatus>
  >(new Map());
  const [scanEventLog, setScanEventLog] = useState<ScanEventLogEntry[]>([]);

  // ── Sync state (absorbed from SyncTab) ──

  const [pushing, setPushing] = useState(false);
  const [pulling, setPulling] = useState(false);
  const [syncError, setSyncError] = useState("");
  const [syncMessage, setSyncMessage] = useState("");

  // ── Save sync state ──

  const activeSaveSyncIntegrationId =
    typeof frontendConfig.saveSyncActiveIntegrationId === "string"
      ? frontendConfig.saveSyncActiveIntegrationId
      : null;
  const [saveSyncMessage, setSaveSyncMessage] = useState("");
  const [saveSyncError, setSaveSyncError] = useState("");
  const [saveSyncMigration, setSaveSyncMigration] =
    useState<SaveSyncMigrationStatus | null>(null);
  const [migrationDialogOpen, setMigrationDialogOpen] = useState(false);
  const [migrationSourceId, setMigrationSourceId] = useState("");
  const [migrationTargetId, setMigrationTargetId] = useState("");
  const [migrationScope, setMigrationScope] = useState<"all" | "game">("all");
  const [migrationCanonicalGameId, setMigrationCanonicalGameId] = useState("");
  const [migrationDeleteSource, setMigrationDeleteSource] = useState(false);
  const [migrationStarting, setMigrationStarting] = useState(false);

  // ── UI state ──

  const [wizardOpen, setWizardOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Integration | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Integration | null>(null);
  const deferredAutoCheckRef = useRef(false);
  const scanInProgress = scanning || activeScanJobId != null;
  const sourceScanActive = scanning && !scanMetadataOnly;
  const metadataRefreshActive = scanning && scanMetadataOnly;
  const latestScanEventKey =
    scanEventLog.length > 0 ? scanEventLog[scanEventLog.length - 1].id : null;

  const sourceScanStateByIntegrationId = useMemo(() => {
    if (!scanning) return new Map<string, IntegrationScanState>();
    const next = new Map<string, IntegrationScanState>();
    for (const integration of scanIntegrations.values()) {
      next.set(integration.integration_id, sourceCardState(integration));
    }
    return next;
  }, [scanIntegrations, scanning]);

  const metadataScanStateByIntegrationId = useMemo(() => {
    if (!scanning) return new Map<string, IntegrationScanState>();
    const aggregate = new Map<string, MetadataParticipationState>();

    for (const sourceIntegration of scanIntegrations.values()) {
      for (const provider of sourceIntegration.metadata_providers ?? []) {
        if (!provider.integration_id) continue;
        const candidate: MetadataParticipationState = {
          integrationId: provider.integration_id,
          label: provider.label,
          pluginId: provider.plugin_id,
          status: provider.status,
          phase: provider.phase,
          reason: provider.reason,
          error: provider.error,
          progress: provider.progress,
          sourceLabel: sourceIntegration.label,
        };
        const existing = aggregate.get(provider.integration_id);
        if (
          !existing ||
          metadataStatusRank(candidate.status) >=
            metadataStatusRank(existing.status)
        ) {
          aggregate.set(provider.integration_id, candidate);
        }
      }
    }

    const next = new Map<string, IntegrationScanState>();
    for (const participation of aggregate.values()) {
      next.set(participation.integrationId, metadataCardState(participation));
    }
    return next;
  }, [scanIntegrations, scanning]);

  const appendScanEvent = useCallback((text: string, data?: unknown) => {
    const ts = readTimestamp(data);
    setScanEventLog((prev) => {
      const next = [...prev, { id: `${ts}:${text}`, ts, text }];
      return next.slice(-30);
    });
  }, []);

  const setScanIntegration = useCallback(
    (
      integrationId: string,
      mutate: (integration: ScanJobIntegrationStatus) => void,
    ) => {
      setScanIntegrations((prev) => {
        const next = new Map(prev);
        const current = next.get(integrationId);
        const integration = current
          ? cloneJobIntegration(current)
          : { integration_id: integrationId, status: "pending" };
        mutate(integration);
        next.set(integrationId, integration);
        return next;
      });
    },
    [],
  );

  const persistActiveScanJobId = useCallback((jobId: string | null) => {
    setActiveScanJobId(jobId);
    writeStoredScanJobId(jobId);
  }, []);

  const clearScanState = useCallback(() => {
    setScanning(false);
    setScanJobStatus("idle");
    setScanMetadataOnly(false);
    setCurrentPhase("");
    setScanStatusText("");
    setScanTotalCount(0);
    setScanCompletedCount(0);
    setScanIntegrations(new Map());
  }, []);

  const adoptScanJob = useCallback(
    (
      job: ScanJobStatus,
      opts?: { appendMessage?: string; resetLog?: boolean },
    ) => {
      persistActiveScanJobId(scanJobIsTerminal(job) ? null : job.job_id);
      if (opts?.resetLog) {
        setScanEventLog([]);
      }
      if (opts?.appendMessage) {
        appendScanEvent(opts.appendMessage);
      }

      setScanJobStatus(job.status);
      setScanMetadataOnly(job.metadata_only);
      setScanError(job.status === "failed" ? (job.error ?? "Scan failed") : "");
      setCurrentPhase(job.current_phase ?? "");
      setScanTotalCount(job.integration_count);
      setScanCompletedCount(job.integrations_completed);
      setScanStatusText(formatScanStatusText(job));
      setScanIntegrations(toIntegrationStateMap(job.integrations));
      if (job.recent_events && job.recent_events.length > 0) {
        setScanEventLog(
          job.recent_events
            .map(toLogEntry)
            .filter((entry): entry is ScanEventLogEntry => entry != null)
            .slice(-30),
        );
      } else if (opts?.resetLog) {
        setScanEventLog([]);
      }

      if (scanJobIsTerminal(job)) {
        clearScanState();
        if (job.status === "completed" && opts?.appendMessage) {
          setScanStatusText("");
        }
        return;
      }

      setScanning(true);
    },
    [appendScanEvent, clearScanState, persistActiveScanJobId],
  );

  useEffect(() => {
    if (!latestScanEventKey) {
      scanEventLogLastKeyRef.current = null;
      scanEventLogShouldStickRef.current = true;
      return;
    }
    if (scanEventLogLastKeyRef.current === latestScanEventKey) {
      return;
    }
    scanEventLogLastKeyRef.current = latestScanEventKey;
    if (!scanEventLogShouldStickRef.current) {
      return;
    }
    const container = scanEventLogRef.current;
    if (!container) {
      return;
    }
    container.scrollTop = container.scrollHeight;
  }, [latestScanEventKey]);

  const handleScanEventLogScroll = useCallback(() => {
    const container = scanEventLogRef.current;
    if (!container) {
      return;
    }
    const distanceFromBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight;
    scanEventLogShouldStickRef.current = distanceFromBottom <= 16;
  }, []);

  // ── SSE: Status check events ──

  useEffect(() => {
    const unsubs = [
      subscribe("integration_status_run_started", (data: unknown) => {
        const d = data as { total?: number };
        setCheckingAll(true);
        setCheckProgress({ current: 0, total: d.total ?? 0 });
      }),

      subscribe("integration_status_checked", (data: unknown) => {
        const d = data as {
          index?: number;
          total?: number;
          integration_id?: string;
          plugin_id?: string;
          label?: string;
          status?: string;
          message?: string;
        };
        if (d.integration_id) {
          const entry: IntegrationStatusEntry = {
            integration_id: d.integration_id,
            plugin_id: d.plugin_id ?? "",
            label: d.label ?? "",
            status: (d.status as "ok" | "error" | "unavailable") ?? "error",
            message: d.message ?? "",
          };
          setStatusMap((prev) => {
            const next = new Map(prev);
            next.set(d.integration_id!, entry);
            return next;
          });
          setCheckingIds((prev) => {
            const next = new Set(prev);
            next.delete(d.integration_id!);
            return next;
          });
        }
        if (d.index !== undefined) {
          setCheckProgress((prev) => ({ ...prev, current: d.index! }));
        }
      }),

      subscribe("integration_status_run_complete", () => {
        setCheckingAll(false);
        setCheckingIds(new Set());
      }),
    ];
    return () => unsubs.forEach((u) => u());
  }, [subscribe]);

  const matchesActiveScanEvent = useCallback(
    (data: unknown) => {
      const jobId = readJobId(data);
      if (!jobId || !activeScanJobId) return false;
      return jobId === activeScanJobId;
    },
    [activeScanJobId],
  );

  useEffect(() => {
    if (!activeScanJobId) return;

    let cancelled = false;
    void (async () => {
      try {
        const job = await getScanJob(activeScanJobId);
        if (cancelled) return;
        adoptScanJob(job);
      } catch (err) {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 404) {
          persistActiveScanJobId(null);
          clearScanState();
          setScanError("");
          return;
        }
        setScanError(
          err instanceof Error
            ? err.message
            : "Failed to restore scan progress",
        );
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [activeScanJobId, adoptScanJob, clearScanState, persistActiveScanJobId]);

  useEffect(() => {
    if (!activeScanJobId || connected) return;

    const timer = window.setInterval(() => {
      void (async () => {
        try {
          const job = await getScanJob(activeScanJobId);
          adoptScanJob(job);
        } catch (err) {
          if (err instanceof ApiError && err.status === 404) {
            persistActiveScanJobId(null);
            clearScanState();
            setScanError("");
            return;
          }
          setScanError(
            err instanceof Error
              ? err.message
              : "Failed to refresh scan progress",
          );
        }
      })();
    }, 10_000);

    return () => window.clearInterval(timer);
  }, [
    activeScanJobId,
    adoptScanJob,
    clearScanState,
    connected,
    persistActiveScanJobId,
  ]);

  // ── SSE: Scan events ──

  useEffect(() => {
    const unsubs = [
      // Scan lifecycle.
      subscribe("scan_started", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as {
          integration_count?: number;
          metadata_only?: boolean;
          integrations?: ScanJobIntegrationStatus[];
        };
        setScanning(true);
        setScanJobStatus("running");
        setScanMetadataOnly(Boolean(d.metadata_only));
        setScanError("");
        setCurrentPhase(d.metadata_only ? "metadata refresh" : "source scan");
        setScanStatusText("Starting scan...");
        setScanTotalCount(d.integration_count ?? 0);
        setScanCompletedCount(0);
        setScanIntegrations(toIntegrationStateMap(d.integrations));
        setScanEventLog([]);
        appendScanEvent(
          `Scan started${d.integration_count ? ` for ${d.integration_count} integration${d.integration_count === 1 ? "" : "s"}` : ""}.`,
          data,
        );
      }),

      subscribe("scan_integration_started", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as {
          integration_id?: string;
          label?: string;
          plugin_id?: string;
        };
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            integration.label = d.label ?? integration.label;
            integration.plugin_id = d.plugin_id ?? integration.plugin_id;
            integration.status = "running";
            integration.phase = "starting";
            integration.error = undefined;
          });
          setScanStatusText(`Scanning ${d.label ?? d.integration_id}...`);
        }
        setScanJobStatus("running");
        appendScanEvent(
          `Integration started: ${d.label ?? d.integration_id ?? "unknown source"}.`,
          data,
        );
      }),

      // Source discovery progress.
      subscribe("scan_source_list_started", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as { integration_id?: string; plugin_id?: string };
        setScanStatusText(`Listing files from ${d.plugin_id ?? "source"}...`);
        setCurrentPhase("listing source content");
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            integration.status = "running";
            integration.phase = "listing source content";
            integration.plugin_id = d.plugin_id ?? integration.plugin_id;
            integration.source_progress = {
              current: 0,
              unit: "items",
              indeterminate: true,
            };
          });
        }
        appendScanEvent(
          `Listing source content from ${d.plugin_id ?? "source"}.`,
          data,
        );
      }),
      subscribe("scan_source_list_complete", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as {
          integration_id?: string;
          file_count?: number;
          game_count?: number;
        };
        if (d.file_count) {
          setScanStatusText(`Found ${d.file_count} files`);
        } else if (d.game_count) {
          setScanStatusText(`Found ${d.game_count} games`);
        }
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            integration.phase = "source listing complete";
            if (d.file_count != null && d.file_count > 0) {
              integration.source_progress = {
                current: 0,
                total: d.file_count,
                unit: "files",
              };
            } else if (d.game_count != null && d.game_count >= 0) {
              integration.source_progress = {
                current: d.game_count,
                total: d.game_count,
                unit: "games",
              };
              integration.games_found = d.game_count;
            }
          });
        }
        appendScanEvent(
          d.file_count
            ? `Source listing complete: ${d.file_count} files found.`
            : `Source listing complete: ${d.game_count ?? 0} games found.`,
          data,
        );
      }),

      // Scanner pipeline progress.
      subscribe("scan_scanner_started", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as { integration_id?: string; file_count?: number };
        setScanStatusText(`Detecting games in ${d.file_count ?? "?"} files...`);
        setCurrentPhase("scanning files");
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            integration.phase = "scanning files";
            integration.source_progress = {
              current: 0,
              total: d.file_count ?? 0,
              unit: "files",
            };
          });
        }
        appendScanEvent(
          `Scanner started for ${d.file_count ?? "?"} files.`,
          data,
        );
      }),
      subscribe("scan_scanner_progress", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as {
          integration_id?: string;
          processed_count?: number;
          file_count?: number;
        };
        setScanStatusText(
          `Scanning files: ${d.processed_count ?? "?"} / ${d.file_count ?? "?"}`,
        );
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            integration.phase = "scanning files";
            integration.source_progress = {
              current: d.processed_count ?? 0,
              total: d.file_count ?? 0,
              unit: "files",
            };
          });
        }
        if (shouldAppendProgressEvent(d.processed_count, d.file_count)) {
          appendScanEvent(
            `Scanner progress: ${d.processed_count ?? "?"} / ${d.file_count ?? "?"} files.`,
            data,
          );
        }
      }),
      subscribe("scan_scanner_complete", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as { integration_id?: string; group_count?: number };
        setScanStatusText(`Detected ${d.group_count ?? "?"} games`);
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            integration.phase = "game detection complete";
            if (integration.source_progress?.total) {
              integration.source_progress = {
                ...integration.source_progress,
                current: integration.source_progress.total,
              };
            }
          });
        }
        appendScanEvent(`Scanner grouped ${d.group_count ?? 0} games.`, data);
      }),

      // Metadata enrichment progress.
      subscribe("scan_metadata_started", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as {
          integration_id?: string;
          game_count?: number;
          resolver_count?: number;
          metadata_providers?: ScanJobMetadataProviderStatus[];
        };
        setScanStatusText(
          `Enriching ${d.game_count ?? "?"} games with ${d.resolver_count ?? "?"} providers...`,
        );
        setCurrentPhase("metadata");
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            integration.phase = "metadata enrichment";
            integration.games_found = d.game_count ?? integration.games_found;
            integration.metadata_label = undefined;
            integration.metadata_integration_id = undefined;
            integration.metadata_plugin_id = undefined;
            integration.metadata_phase = undefined;
            integration.metadata_providers = d.metadata_providers?.map(
              cloneMetadataProvider,
            );
            integration.metadata_progress = {
              current: 0,
              total: d.game_count ?? 0,
              unit: "games",
            };
          });
        }
        appendScanEvent(
          `Metadata started for ${d.game_count ?? "?"} games across ${d.resolver_count ?? "?"} providers.`,
          data,
        );
      }),
      subscribe("scan_metadata_phase", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as { integration_id?: string; phase?: string };
        const phaseLabel = formatMetadataPhase(d.phase);
        setCurrentPhase(d.phase ?? "");
        setScanStatusText(`Metadata: ${phaseLabel}...`);
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            integration.phase =
              d.phase === "consensus"
                ? "metadata consensus complete"
                : `metadata ${d.phase ?? "working"}`;
            integration.metadata_phase = d.phase;
            integration.metadata_integration_id = undefined;
            integration.metadata_label = undefined;
            integration.metadata_plugin_id = undefined;
          });
        }
        appendScanEvent(`Metadata phase: ${phaseLabel || "working"}.`, data);
      }),
      subscribe("scan_metadata_plugin_started", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as {
          integration_id?: string;
          metadata_integration_id?: string;
          metadata_label?: string;
          plugin_id?: string;
          batch_size?: number;
          phase?: string;
        };
        const providerLabel = d.metadata_label ?? d.plugin_id ?? "Provider";
        setScanStatusText(
          `${providerLabel}: looking up ${d.batch_size ?? "?"} games...`,
        );
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            integration.phase = d.metadata_label
              ? `metadata via ${d.metadata_label}: ${d.phase ?? "working"}`
              : `metadata ${d.phase ?? "working"}`;
            integration.metadata_phase = d.phase;
            integration.metadata_integration_id = d.metadata_integration_id;
            integration.metadata_label = d.metadata_label;
            integration.metadata_plugin_id = d.plugin_id;
            const provider = ensureMetadataProviderState(integration, {
              integration_id: d.metadata_integration_id ?? "",
              label: d.metadata_label,
              plugin_id: d.plugin_id,
            });
            provider.status = "running";
            provider.phase = d.phase;
            provider.reason = undefined;
            provider.error = undefined;
            integration.metadata_progress = {
              current: 0,
              total: d.batch_size ?? 0,
              unit: "games",
              indeterminate: d.batch_size == null,
            };
            provider.progress = cloneJobProgress(integration.metadata_progress);
          });
        }
        appendScanEvent(
          `${providerLabel} started ${d.phase ?? "metadata"} for ${d.batch_size ?? "?"} games.`,
          data,
        );
      }),
      subscribe("scan_metadata_game_progress", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as {
          integration_id?: string;
          metadata_integration_id?: string;
          metadata_label?: string;
          plugin_id?: string;
          phase?: string;
          game_index?: number;
          game_count?: number;
          game_title?: string;
        };
        const providerLabel = d.metadata_label ?? d.plugin_id ?? "Provider";
        setScanStatusText(
          `${providerLabel}: ${d.game_index ?? "?"} / ${d.game_count ?? "?"}${d.game_title ? ` - ${d.game_title}` : ""}`,
        );
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            integration.phase = d.metadata_label
              ? `metadata via ${d.metadata_label}: ${d.phase ?? "working"}`
              : `metadata ${d.phase ?? "working"}`;
            integration.metadata_phase = d.phase;
            integration.metadata_integration_id = d.metadata_integration_id;
            integration.metadata_label = d.metadata_label;
            integration.metadata_plugin_id = d.plugin_id;
            integration.metadata_progress = {
              current: d.game_index ?? 0,
              total: d.game_count ?? 0,
              unit: "games",
            };
            const provider = ensureMetadataProviderState(integration, {
              integration_id: d.metadata_integration_id ?? "",
              label: d.metadata_label,
              plugin_id: d.plugin_id,
            });
            provider.status = "running";
            provider.phase = d.phase;
            provider.progress = cloneJobProgress(integration.metadata_progress);
          });
        }
        if (shouldAppendMetadataProgressEvent(d.game_index, d.game_count)) {
          appendScanEvent(
            `${providerLabel} ${d.game_index ?? "?"} / ${d.game_count ?? "?"}${d.game_title ? ` - ${d.game_title}` : ""}`,
            data,
          );
        }
      }),
      subscribe("scan_metadata_plugin_complete", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as {
          integration_id?: string;
          metadata_integration_id?: string;
          metadata_label?: string;
          plugin_id?: string;
          phase?: string;
          matched?: number;
          total?: number;
          filled?: number;
          candidates?: number;
        };
        const providerLabel = d.metadata_label ?? d.plugin_id ?? "Provider";
        if (d.matched != null) {
          setScanStatusText(
            `${providerLabel}: matched ${d.matched}/${d.total ?? "?"}`,
          );
          appendScanEvent(
            `${providerLabel} matched ${d.matched}/${d.total ?? "?"}.`,
            data,
          );
        } else if (d.filled != null) {
          setScanStatusText(`${providerLabel}: filled ${d.filled} gaps`);
          appendScanEvent(
            `${providerLabel} filled ${d.filled} metadata gaps.`,
            data,
          );
        }
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            integration.phase = d.metadata_label
              ? `metadata via ${d.metadata_label}: ${d.phase ?? "complete"}`
              : `metadata ${d.phase ?? "complete"}`;
            integration.metadata_phase = d.phase;
            integration.metadata_integration_id = d.metadata_integration_id;
            integration.metadata_label = d.metadata_label;
            integration.metadata_plugin_id = d.plugin_id;
            if (d.total != null) {
              integration.metadata_progress = {
                current: d.total,
                total: d.total,
                unit: "games",
              };
            } else if (d.candidates != null) {
              integration.metadata_progress = {
                current: d.candidates,
                total: d.candidates,
                unit: "games",
              };
            }
            const provider = ensureMetadataProviderState(integration, {
              integration_id: d.metadata_integration_id ?? "",
              label: d.metadata_label,
              plugin_id: d.plugin_id,
            });
            provider.status = "completed";
            provider.phase = d.phase;
            provider.progress = cloneJobProgress(integration.metadata_progress);
          });
        }
      }),
      subscribe("scan_metadata_plugin_error", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as {
          integration_id?: string;
          metadata_integration_id?: string;
          metadata_label?: string;
          plugin_id?: string;
          phase?: string;
          error?: string;
        };
        const providerLabel = d.metadata_label ?? d.plugin_id ?? "Provider";
        setScanStatusText(
          `${providerLabel} error: ${d.error ?? "unknown error"}`,
        );
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            integration.phase = d.metadata_label
              ? `metadata via ${d.metadata_label}: ${d.phase ?? "error"}`
              : `metadata ${d.phase ?? "error"}`;
            integration.metadata_phase = d.phase;
            integration.metadata_integration_id = d.metadata_integration_id;
            integration.metadata_label = d.metadata_label;
            integration.metadata_plugin_id = d.plugin_id;
            integration.error = d.error;
            const provider = ensureMetadataProviderState(integration, {
              integration_id: d.metadata_integration_id ?? "",
              label: d.metadata_label,
              plugin_id: d.plugin_id,
            });
            provider.status = "error";
            provider.phase = d.phase;
            provider.error = d.error;
          });
        }
        appendScanEvent(
          `${providerLabel} error: ${d.error ?? "unknown error"}.`,
          data,
        );
      }),
      subscribe("scan_metadata_consensus_complete", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as {
          integration_id?: string;
          identified?: number;
          unidentified?: number;
        };
        setScanStatusText(
          `Consensus: ${d.identified ?? 0} identified, ${d.unidentified ?? 0} unidentified`,
        );
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            const total = (d.identified ?? 0) + (d.unidentified ?? 0);
            integration.phase = "metadata consensus complete";
            integration.metadata_phase = "consensus";
            integration.metadata_integration_id = undefined;
            integration.metadata_label = undefined;
            integration.metadata_plugin_id = undefined;
            integration.metadata_progress = {
              current: total,
              total,
              unit: "games",
            };
          });
        }
        appendScanEvent(
          `Consensus complete: ${d.identified ?? 0} identified, ${d.unidentified ?? 0} unidentified.`,
          data,
        );
      }),
      subscribe("scan_metadata_finished", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as {
          integration_id?: string;
          identified?: number;
          unidentified?: number;
        };
        setScanStatusText(
          `Metadata complete: ${d.identified ?? 0} identified, ${d.unidentified ?? 0} unidentified`,
        );
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            const total = (d.identified ?? 0) + (d.unidentified ?? 0);
            integration.phase = "metadata complete";
            integration.metadata_phase = "finished";
            integration.metadata_integration_id = undefined;
            integration.metadata_label = undefined;
            integration.metadata_plugin_id = undefined;
            integration.metadata_progress = {
              current: total,
              total,
              unit: "games",
            };
            integration.metadata_providers =
              integration.metadata_providers?.map((provider) =>
                provider.status === "pending"
                  ? { ...provider, status: "not_used" }
                  : provider,
              );
          });
        }
        appendScanEvent(
          `Metadata complete: ${d.identified ?? 0} identified, ${d.unidentified ?? 0} unidentified.`,
          data,
        );
      }),

      // Per-integration completion.
      subscribe("scan_integration_complete", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as {
          integration_id?: string;
          label?: string;
          plugin_id?: string;
          games_found?: number;
        };
        if (d.integration_id) {
          setScanCompletedCount((prev) => prev + 1);
          setScanIntegration(d.integration_id, (integration) => {
            integration.label = d.label ?? integration.label;
            integration.plugin_id = d.plugin_id ?? integration.plugin_id;
            integration.status = "completed";
            integration.phase = "completed";
            integration.games_found = d.games_found ?? integration.games_found;
            integration.reason = undefined;
            integration.metadata_integration_id = undefined;
            integration.metadata_label = undefined;
            integration.metadata_plugin_id = undefined;
            if (integration.source_progress?.total) {
              integration.source_progress = {
                ...integration.source_progress,
                current: integration.source_progress.total,
              };
            }
            if (integration.metadata_progress?.total) {
              integration.metadata_progress = {
                ...integration.metadata_progress,
                current: integration.metadata_progress.total,
              };
            }
          });
        }
        queryClient.invalidateQueries({ queryKey: ["stats"] });
        queryClient.invalidateQueries({ queryKey: ["integration-games"] });
        appendScanEvent(
          `Integration complete: ${d.label ?? d.integration_id ?? "unknown"} (${d.games_found ?? 0} games).`,
          data,
        );
      }),
      subscribe("scan_integration_skipped", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as {
          integration_id?: string;
          label?: string;
          plugin_id?: string;
          reason?: string;
          error?: string;
        };
        if (d.integration_id) {
          setScanCompletedCount((prev) => prev + 1);
          setScanIntegration(d.integration_id, (integration) => {
            integration.label = d.label ?? integration.label;
            integration.plugin_id = d.plugin_id ?? integration.plugin_id;
            integration.status = "skipped";
            integration.phase = "skipped";
            integration.reason = d.reason;
            integration.error = d.error;
            integration.source_progress = undefined;
            integration.metadata_phase = undefined;
            integration.metadata_progress = undefined;
            integration.metadata_integration_id = undefined;
            integration.metadata_label = undefined;
            integration.metadata_plugin_id = undefined;
          });
        }
        appendScanEvent(
          `Integration skipped: ${d.label ?? d.integration_id ?? "unknown"}${d.reason ? ` (${formatSourceSkipReason(d.reason)})` : ""}.`,
          data,
        );
      }),
      subscribe("scan_cancel_requested", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        setScanJobStatus("cancelling");
        setCurrentPhase("cancelling");
        setScanStatusText("Cancelling scan...");
        appendScanEvent("Scan cancellation requested.", data);
      }),
      subscribe("scan_cancelled", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        setScanJobStatus("cancelled");
        setScanning(false);
        setScanMetadataOnly(false);
        setCurrentPhase("");
        setScanStatusText("");
        persistActiveScanJobId(null);
        appendScanEvent("Scan cancelled.", data);
        queryClient.invalidateQueries({ queryKey: ["stats"] });
        queryClient.invalidateQueries({ queryKey: ["games"] });
        queryClient.invalidateQueries({ queryKey: ["integration-games"] });
      }),

      // Scan finished.
      subscribe("scan_complete", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        setScanJobStatus("completed");
        setScanning(false);
        setScanMetadataOnly(false);
        setCurrentPhase("");
        setScanStatusText("");
        persistActiveScanJobId(null);
        appendScanEvent("Scan complete.", data);
        queryClient.invalidateQueries({ queryKey: ["stats"] });
        queryClient.invalidateQueries({ queryKey: ["games"] });
        queryClient.invalidateQueries({ queryKey: ["integration-games"] });
        queryClient.invalidateQueries({ queryKey: ["scan-reports"] });
      }),

      // Scan error.
      subscribe("scan_error", (data: unknown) => {
        if (!matchesActiveScanEvent(data)) return;
        const d = data as { integration_id?: string; error?: string };
        setScanJobStatus("failed");
        setScanMetadataOnly(false);
        setScanError(d.error ?? "Scan failed");
        setScanning(false);
        setScanStatusText("");
        if (d.integration_id) {
          setScanIntegration(d.integration_id, (integration) => {
            integration.status = "failed";
            integration.phase = "failed";
            integration.error = d.error;
          });
        }
        persistActiveScanJobId(null);
        appendScanEvent(`Scan failed: ${d.error ?? "unknown error"}.`, data);
        queryClient.invalidateQueries({ queryKey: ["stats"] });
        queryClient.invalidateQueries({ queryKey: ["integration-games"] });
      }),
    ];
    return () => unsubs.forEach((u) => u());
  }, [
    appendScanEvent,
    matchesActiveScanEvent,
    persistActiveScanJobId,
    queryClient,
    setScanIntegration,
    subscribe,
  ]);

  // ── SSE: Sync events ──

  useEffect(() => {
    const unsubs = [
      subscribe("sync_operation_finished", (data: unknown) => {
        const d = data as { operation?: string; ok?: boolean; error?: string };
        if (d.ok) {
          setSyncMessage(
            `${d.operation === "push" ? "Push" : "Pull"} completed successfully`,
          );
        } else {
          setSyncError(d.error ?? "Operation failed");
        }
        setPushing(false);
        setPulling(false);
        queryClient.invalidateQueries({ queryKey: ["sync", "status"] });
      }),
      subscribe("sync_key_stored", () => {
        setSyncMessage("Encryption key stored");
        queryClient.invalidateQueries({ queryKey: ["sync", "status"] });
      }),
      subscribe("sync_key_cleared", () => {
        setSyncMessage("Encryption key cleared");
        queryClient.invalidateQueries({ queryKey: ["sync", "status"] });
      }),
      subscribe("save_sync_migration_started", (data: unknown) => {
        setSaveSyncMigration(data as SaveSyncMigrationStatus);
        setSaveSyncMessage("Save migration started.");
        setSaveSyncError("");
      }),
      subscribe("save_sync_migration_progress", (data: unknown) => {
        setSaveSyncMigration(data as SaveSyncMigrationStatus);
      }),
      subscribe("save_sync_migration_completed", (data: unknown) => {
        setSaveSyncMigration(data as SaveSyncMigrationStatus);
        setSaveSyncMessage("Save migration completed.");
        setSaveSyncError("");
      }),
      subscribe("save_sync_migration_failed", (data: unknown) => {
        const status = data as SaveSyncMigrationStatus;
        setSaveSyncMigration(status);
        setSaveSyncError(status.error ?? "Save migration failed.");
      }),
    ];
    return () => unsubs.forEach((u) => u());
  }, [subscribe, queryClient]);

  // ── Handlers: Status ──

  const handleCheckAll = useCallback(async () => {
    if (scanInProgress) {
      deferredAutoCheckRef.current = true;
      return;
    }
    setCheckingAll(true);
    setCheckProgress({ current: 0, total: integrations.length });
    setCheckingIds(new Set(integrations.map((i) => i.id)));
    try {
      const results = await getIntegrationStatus();
      setStatusMap((prev) => {
        const next = new Map(prev);
        for (const r of results) next.set(r.integration_id, r);
        return next;
      });
    } catch {
      /* SSE events may still arrive. */
    } finally {
      setCheckingAll(false);
      setCheckingIds(new Set());
    }
  }, [integrations, scanInProgress]);

  const handleCheckOne = useCallback(async (id: string) => {
    setCheckingIds((prev) => new Set([...prev, id]));
    try {
      const result = await checkIntegrationStatus(id);
      setStatusMap((prev) => {
        const next = new Map(prev);
        next.set(result.integration_id, result);
        return next;
      });
    } catch {
      /* Keep existing status. */
    } finally {
      setCheckingIds((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }
  }, []);

  // ── Auto-check on first load ──

  const checkedOnceRef = useRef(false);
  useEffect(() => {
    if (integrations.length > 0 && !checkedOnceRef.current) {
      checkedOnceRef.current = true;
      if (scanInProgress) {
        deferredAutoCheckRef.current = true;
        return;
      }
      handleCheckAll();
    }
  }, [handleCheckAll, integrations.length, scanInProgress]);

  useEffect(() => {
    if (
      integrations.length === 0 ||
      !deferredAutoCheckRef.current ||
      scanInProgress ||
      checkingAll
    ) {
      return;
    }
    deferredAutoCheckRef.current = false;
    void handleCheckAll();
  }, [checkingAll, handleCheckAll, integrations.length, scanInProgress]);

  // ── Handlers: Scan ──

  const handleScanAll = useCallback(async () => {
    setScanError("");
    try {
      const result = await triggerScan();
      adoptScanJob(result.job, {
        resetLog: true,
        appendMessage: result.accepted
          ? "Scan job started."
          : "Rejoined the active scan job.",
      });
    } catch (err) {
      setScanError(err instanceof Error ? err.message : "Scan failed");
      clearScanState();
    }
  }, [adoptScanJob, clearScanState]);

  const handleScanOne = useCallback(
    async (id: string) => {
      setScanError("");
      try {
        const result = await triggerScan([id]);
        adoptScanJob(result.job, {
          resetLog: true,
          appendMessage: result.accepted
            ? `Scan job started for ${id}.`
            : "Rejoined the active scan job.",
        });
      } catch (err) {
        setScanError(err instanceof Error ? err.message : "Scan failed");
        clearScanState();
      }
    },
    [adoptScanJob, clearScanState],
  );

  const handleRefreshMetadata = useCallback(async () => {
    setScanError("");
    try {
      const result = await triggerScan(undefined, { metadataOnly: true });
      adoptScanJob(result.job, {
        resetLog: true,
        appendMessage: result.accepted
          ? "Metadata refresh job started."
          : "Rejoined the active scan job.",
      });
    } catch (err) {
      setScanError(
        err instanceof Error ? err.message : "Metadata refresh failed",
      );
      clearScanState();
    }
  }, [adoptScanJob, clearScanState]);

  const handleCancelScan = useCallback(async () => {
    if (!activeScanJobId) return;
    setScanError("");
    try {
      const result = await cancelScanJob(activeScanJobId);
      adoptScanJob(result.job);
      if (!result.accepted && result.job.status === "cancelled") {
        appendScanEvent("Scan already cancelled.");
      }
    } catch (err) {
      setScanError(
        err instanceof Error ? err.message : "Failed to cancel scan",
      );
    }
  }, [activeScanJobId, adoptScanJob, appendScanEvent]);

  // ── Handlers: Sync ──

  const handlePush = useCallback(async () => {
    setSyncError("");
    setSyncMessage("");
    setPushing(true);
    try {
      const result = await syncPush();
      setSyncMessage(
        `Push complete: ${result.integrations} integrations, ${result.settings} settings exported`,
      );
    } catch (err) {
      setSyncError(err instanceof Error ? err.message : "Push failed");
    } finally {
      setPushing(false);
    }
  }, []);

  const handlePull = useCallback(async () => {
    setSyncError("");
    setSyncMessage("");
    setPulling(true);
    try {
      const result = await syncPull();
      const r = result.result;
      setSyncMessage(
        `Pull complete: ${r.integrations_added} added, ${r.integrations_updated} updated, ${r.settings_added + r.settings_updated} settings synced`,
      );
    } catch (err) {
      setSyncError(err instanceof Error ? err.message : "Pull failed");
    } finally {
      setPulling(false);
    }
  }, []);

  const handleStoreKey = useCallback(
    async (passphrase: string) => {
      if (!passphrase) return;
      setSyncError("");
      setSyncMessage("");
      try {
        await storeKey(passphrase);
        setSyncMessage("Key stored securely");
        queryClient.invalidateQueries({ queryKey: ["sync", "status"] });
      } catch (err) {
        setSyncError(
          err instanceof Error ? err.message : "Failed to store key",
        );
      }
    },
    [queryClient],
  );

  const handleClearKey = useCallback(async () => {
    setSyncError("");
    setSyncMessage("");
    try {
      await clearKey();
      setSyncMessage("Key cleared");
      queryClient.invalidateQueries({ queryKey: ["sync", "status"] });
    } catch (err) {
      setSyncError(err instanceof Error ? err.message : "Failed to clear key");
    }
  }, [queryClient]);

  // ── Handlers: Delete ──

  const handleDelete = useCallback(async () => {
    if (!deleteTarget) return;
    try {
      await deleteIntegration(deleteTarget.id);
      queryClient.invalidateQueries({ queryKey: ["integrations"] });
      queryClient.invalidateQueries({ queryKey: ["stats"] });
      queryClient.invalidateQueries({ queryKey: ["games"] });
      queryClient.invalidateQueries({ queryKey: ["integration-games"] });
      setStatusMap((prev) => {
        const next = new Map(prev);
        next.delete(deleteTarget.id);
        return next;
      });
    } catch {
      /* Fail silently for now. */
    }
  }, [deleteTarget, queryClient]);

  const handleSetActiveSaveSync = useCallback(
    async (integrationId: string) => {
      setSaveSyncError("");
      setSaveSyncMessage("");
      try {
        await setFrontendConfig({
          ...frontendConfig,
          saveSyncActiveIntegrationId: integrationId,
        });
        setSaveSyncMessage("Active save sync integration updated.");
        queryClient.invalidateQueries({ queryKey: ["frontend-config"] });
      } catch (err) {
        setSaveSyncError(
          err instanceof Error
            ? err.message
            : "Failed to update active save sync integration.",
        );
      }
    },
    [frontendConfig, queryClient],
  );

  const handleStartMigration = useCallback(async () => {
    if (!migrationSourceId || !migrationTargetId) {
      setSaveSyncError("Choose both source and target save sync integrations.");
      return;
    }
    if (migrationSourceId === migrationTargetId) {
      setSaveSyncError("Source and target integrations must differ.");
      return;
    }
    if (migrationScope === "game" && !migrationCanonicalGameId.trim()) {
      setSaveSyncError("Canonical game ID is required for game migration.");
      return;
    }

    setMigrationStarting(true);
    setSaveSyncError("");
    setSaveSyncMessage("");
    try {
      const status = await startSaveSyncMigration({
        source_integration_id: migrationSourceId,
        target_integration_id: migrationTargetId,
        scope: migrationScope,
        canonical_game_id:
          migrationScope === "game"
            ? migrationCanonicalGameId.trim()
            : undefined,
        delete_source_after_success: migrationDeleteSource,
      });
      setSaveSyncMigration(status);
      setSaveSyncMessage("Save migration started.");
      setMigrationDialogOpen(false);
    } catch (err) {
      setSaveSyncError(
        err instanceof Error ? err.message : "Failed to start save migration.",
      );
    } finally {
      setMigrationStarting(false);
    }
  }, [
    migrationCanonicalGameId,
    migrationDeleteSource,
    migrationScope,
    migrationSourceId,
    migrationTargetId,
  ]);

  // ── Group integrations by capability ──

  const groups = useMemo(() => {
    return groupIntegrationsByCapability(integrations, plugins);
  }, [integrations, plugins]);

  const saveSyncIntegrations = useMemo(
    () => groups.get("save_sync") ?? [],
    [groups],
  );

  useEffect(() => {
    if (saveSyncIntegrations.length > 0) {
      setMigrationSourceId((prev) => prev || saveSyncIntegrations[0]?.id || "");
      setMigrationTargetId(
        (prev) =>
          prev ||
          saveSyncIntegrations[1]?.id ||
          saveSyncIntegrations[0]?.id ||
          "",
      );
    }
  }, [saveSyncIntegrations]);

  const sortedGroupKeys = useMemo(() => {
    const keys = Array.from(groups.keys());
    return keys.sort((a, b) => {
      const orderA = CAPABILITY_META[a]?.order ?? 99;
      const orderB = CAPABILITY_META[b]?.order ?? 99;
      return orderA - orderB;
    });
  }, [groups]);

  // Build sync state object to pass down.
  const syncStateObj = useMemo(
    () => ({
      pushing,
      pulling,
      message: syncMessage,
      error: syncError,
    }),
    [pushing, pulling, syncMessage, syncError],
  );

  // ── Render ──

  if (loadingIntegrations) {
    return (
      <div className="text-mga-muted text-sm py-8 text-center">
        Loading integrations...
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-mga-text">Integrations</h3>
          <p className="text-xs text-mga-muted mt-0.5">
            {integrations.length} integration
            {integrations.length !== 1 ? "s" : ""} configured
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleCheckAll}
            disabled={
              checkingAll || integrations.length === 0 || scanInProgress
            }
          >
            <RefreshCw
              size={14}
              className={checkingAll ? "animate-spin" : ""}
            />
            {checkingAll ? "Checking..." : "Check All"}
          </Button>
          <Button size="sm" onClick={() => setWizardOpen(true)}>
            <Plus size={14} />
            Add Integration
          </Button>
        </div>
      </div>

      {/* Check All progress bar */}
      {checkingAll && checkProgress.total > 0 && (
        <ProgressBar
          value={(checkProgress.current / checkProgress.total) * 100}
          label={`Checking ${checkProgress.current}/${checkProgress.total}`}
        />
      )}
      {scanInProgress && (
        <p className="text-xs text-mga-muted">
          Automatic integration checks are deferred while a scan is active.
        </p>
      )}

      {/* Library stats summary */}
      {stats && stats.canonical_game_count > 0 && (
        <LibraryStatsSummary stats={stats} />
      )}

      {/* Scan summary / history */}
      {!scanning && <ScanSummary />}

      {/* Global scan progress */}
      {scanning && (
        <div className="border border-mga-border rounded-mga bg-mga-surface p-3 space-y-3">
          <div className="flex items-center justify-between">
            <div className="min-w-0">
              <p className="text-xs font-medium text-mga-text">
                {scanJobStatus === "cancelling"
                  ? "Cancelling scan..."
                  : scanMetadataOnly
                    ? "Refreshing metadata..."
                    : "Scanning sources..."}
                {scanTotalCount > 0 &&
                  ` (${scanCompletedCount}/${scanTotalCount} integrations)`}
              </p>
              {currentPhase && (
                <p className="text-[11px] text-mga-accent font-medium capitalize">
                  {currentPhase}
                </p>
              )}
            </div>
            {activeScanJobId && (
              <Button
                variant="outline"
                size="sm"
                onClick={handleCancelScan}
                disabled={scanJobStatus === "cancelling"}
                className="text-xs"
              >
                {scanJobStatus === "cancelling" ? "Cancelling..." : "Cancel"}
              </Button>
            )}
          </div>
          <ProgressBar
            value={
              scanTotalCount > 0
                ? (scanCompletedCount / scanTotalCount) * 100
                : undefined
            }
            label={
              scanTotalCount > 0
                ? `${scanCompletedCount}/${scanTotalCount}`
                : "Scanning..."
            }
          />
          {scanStatusText && (
            <p className="text-xs text-mga-muted truncate">{scanStatusText}</p>
          )}
          {scanIntegrations.size > 0 && (
            <div className="space-y-2 border-t border-mga-border/60 pt-3">
              {Array.from(scanIntegrations.values()).map((integration) => (
                <div
                  key={integration.integration_id}
                  className="rounded-mga border border-mga-border/60 bg-mga-elevated/30 p-2 space-y-2"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="text-xs font-medium text-mga-text truncate">
                        {`Game source: ${integration.label ?? integration.integration_id}`}
                      </p>
                      {formatSourceDetail(integration) && (
                        <p className="text-[11px] text-mga-muted truncate">
                          {formatSourceDetail(integration)}
                        </p>
                      )}
                    </div>
                    <Badge
                      variant={
                        sourceBadgePresentation(integration).badgeVariant
                      }
                      className={
                        sourceBadgePresentation(integration).badgeClassName
                      }
                    >
                      {sourceBadgePresentation(integration).badge}
                    </Badge>
                  </div>
                  {integration.source_progress && (
                    <ProgressBar
                      value={
                        integration.source_progress.indeterminate ||
                        !integration.source_progress.total
                          ? undefined
                          : (integration.source_progress.current /
                              integration.source_progress.total) *
                            100
                      }
                      label={`Source: ${formatProgressLabel(integration.source_progress, "Working...")}`}
                    />
                  )}
                  {(integration.metadata_progress ||
                    integration.metadata_phase) && (
                    <ProgressBar
                      value={
                        integration.metadata_progress &&
                        !integration.metadata_progress.indeterminate &&
                        integration.metadata_progress.total
                          ? (integration.metadata_progress.current /
                              integration.metadata_progress.total) *
                            100
                          : undefined
                      }
                      label={`${formatMetadataPanelLabel(integration)}: ${formatProgressLabel(integration.metadata_progress, "Working...")}`}
                    />
                  )}
                  {integration.error &&
                    integration.status !== "completed" &&
                    integration.status !== "skipped" && (
                      <p className="text-[11px] text-red-400 truncate">
                        {integration.error}
                      </p>
                    )}
                </div>
              ))}
            </div>
          )}
          {scanEventLog.length > 0 && (
            <div
              ref={scanEventLogRef}
              onScroll={handleScanEventLogScroll}
              className="space-y-1 border-t border-mga-border/60 pt-2 max-h-56 overflow-y-auto pr-1"
            >
              {scanEventLog.map((entry) => (
                <div
                  key={entry.id}
                  className="flex items-start gap-2 text-xs text-mga-muted"
                >
                  <span className="shrink-0 font-mono text-[10px] uppercase tracking-wide text-mga-accent/80">
                    {formatLogTime(entry.ts)}
                  </span>
                  <span className="min-w-0 flex-1">{entry.text}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Scan error */}
      {scanError && (
        <div className="border border-red-500/30 rounded-mga bg-red-500/10 p-3">
          <p className="text-xs text-red-400">{scanError}</p>
        </div>
      )}

      {/* Groups */}
      {integrations.length === 0 ? (
        <div className="text-center py-12 border border-mga-border rounded-mga bg-mga-surface">
          <p className="text-mga-muted text-sm mb-3">
            No integrations configured yet.
          </p>
          <Button size="sm" onClick={() => setWizardOpen(true)}>
            <Plus size={14} />
            Add Your First Integration
          </Button>
        </div>
      ) : (
        <div className="space-y-3">
          {saveSyncMigration && (
            <div className="border border-mga-border rounded-mga bg-mga-surface p-3 space-y-2">
              <div className="flex items-center justify-between">
                <p className="text-xs font-medium text-mga-text">
                  Save Sync Migration
                </p>
                <span className="text-xs text-mga-muted uppercase tracking-wide">
                  {saveSyncMigration.status}
                </span>
              </div>
              <ProgressBar
                value={
                  saveSyncMigration.items_total > 0
                    ? (saveSyncMigration.items_completed /
                        saveSyncMigration.items_total) *
                      100
                    : undefined
                }
                label={
                  saveSyncMigration.items_total > 0
                    ? `${saveSyncMigration.items_completed}/${saveSyncMigration.items_total}`
                    : "Preparing..."
                }
              />
              <p className="text-xs text-mga-muted">
                {saveSyncMigration.slots_migrated} migrated,{" "}
                {saveSyncMigration.slots_skipped} skipped
              </p>
            </div>
          )}

          {saveSyncMessage && (
            <div className="border border-emerald-500/30 rounded-mga bg-emerald-500/10 p-3">
              <p className="text-xs text-emerald-300">{saveSyncMessage}</p>
            </div>
          )}
          {saveSyncError && (
            <div className="border border-red-500/30 rounded-mga bg-red-500/10 p-3">
              <p className="text-xs text-red-400">{saveSyncError}</p>
            </div>
          )}

          {sortedGroupKeys.map((cap) => (
            <IntegrationGroupSection
              key={cap}
              capability={cap}
              integrations={groups.get(cap)!}
              plugins={plugins}
              statusMap={statusMap}
              checkingIds={checkingIds}
              onCheck={handleCheckOne}
              onEdit={(integ) => setEditTarget(integ)}
              onDelete={(integ) => setDeleteTarget(integ)}
              // Scan props.
              stats={stats}
              scanStateByIntegrationId={
                cap === "source"
                  ? sourceScanStateByIntegrationId
                  : cap === "metadata"
                    ? metadataScanStateByIntegrationId
                    : undefined
              }
              onScan={handleScanOne}
              onScanGroup={cap === "source" ? handleScanAll : undefined}
              onRefreshMetadata={
                cap === "metadata" ? handleRefreshMetadata : undefined
              }
              scanControlsDisabled={scanInProgress}
              sourceScanActive={sourceScanActive}
              metadataRefreshActive={metadataRefreshActive}
              // Sync props.
              syncStatus={syncStatus}
              syncState={syncStateObj}
              onPush={handlePush}
              onPull={handlePull}
              onStoreKey={handleStoreKey}
              onClearKey={handleClearKey}
              activeSaveSyncIntegrationId={
                cap === "save_sync" ? activeSaveSyncIntegrationId : undefined
              }
              onSetActiveSaveSync={
                cap === "save_sync" ? handleSetActiveSaveSync : undefined
              }
              saveSyncHeaderControls={
                cap === "save_sync" ? (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setMigrationDialogOpen(true)}
                    disabled={saveSyncIntegrations.length < 2}
                    className="text-xs"
                  >
                    Migrate Saves
                  </Button>
                ) : undefined
              }
            />
          ))}
        </div>
      )}

      {/* Add Integration Wizard */}
      {wizardOpen && (
        <AddIntegrationWizard
          onClose={() => setWizardOpen(false)}
          onSaved={() => {
            setWizardOpen(false);
            queryClient.invalidateQueries({ queryKey: ["integrations"] });
          }}
        />
      )}

      {/* Edit Integration Dialog */}
      {editTarget && (
        <EditIntegrationDialog
          integration={editTarget}
          onClose={() => setEditTarget(null)}
          onSaved={() => {
            setEditTarget(null);
            queryClient.invalidateQueries({ queryKey: ["integrations"] });
          }}
        />
      )}

      {/* Delete Confirmation */}
      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="Delete Integration"
        message={`Remove "${deleteTarget?.label}"? This cannot be undone.`}
        confirmLabel="Delete"
        confirmVariant="danger"
      />

      {migrationDialogOpen && (
        <Dialog
          open={migrationDialogOpen}
          onClose={() => setMigrationDialogOpen(false)}
          title="Migrate Save Sync Data"
        >
          <div className="space-y-4">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div className="space-y-1">
                <label className="text-sm font-medium text-mga-text">
                  Source Integration
                </label>
                <select
                  value={migrationSourceId}
                  onChange={(event) => setMigrationSourceId(event.target.value)}
                  className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text"
                >
                  {saveSyncIntegrations.map((integration) => (
                    <option key={integration.id} value={integration.id}>
                      {integration.label}
                    </option>
                  ))}
                </select>
              </div>
              <div className="space-y-1">
                <label className="text-sm font-medium text-mga-text">
                  Target Integration
                </label>
                <select
                  value={migrationTargetId}
                  onChange={(event) => setMigrationTargetId(event.target.value)}
                  className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text"
                >
                  {saveSyncIntegrations.map((integration) => (
                    <option key={integration.id} value={integration.id}>
                      {integration.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="space-y-2">
              <label className="text-sm font-medium text-mga-text">Scope</label>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant={migrationScope === "all" ? "default" : "outline"}
                  size="sm"
                  onClick={() => setMigrationScope("all")}
                >
                  Entire Library
                </Button>
                <Button
                  type="button"
                  variant={migrationScope === "game" ? "default" : "outline"}
                  size="sm"
                  onClick={() => setMigrationScope("game")}
                >
                  One Canonical Game
                </Button>
              </div>
            </div>

            {migrationScope === "game" && (
              <Input
                label="Canonical Game ID"
                value={migrationCanonicalGameId}
                onChange={(event) =>
                  setMigrationCanonicalGameId(event.target.value)
                }
                placeholder="Enter the canonical game ID to migrate"
              />
            )}

            <label className="flex items-center gap-2 text-sm text-mga-text">
              <input
                type="checkbox"
                checked={migrationDeleteSource}
                onChange={(event) =>
                  setMigrationDeleteSource(event.target.checked)
                }
              />
              Delete source copies after a successful migration
            </label>

            <div className="flex justify-end gap-3 pt-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setMigrationDialogOpen(false)}
              >
                Cancel
              </Button>
              <Button
                size="sm"
                onClick={handleStartMigration}
                disabled={migrationStarting}
              >
                {migrationStarting ? "Starting..." : "Start Migration"}
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Grouping helper
// ---------------------------------------------------------------------------

function groupIntegrationsByCapability(
  integrations: Integration[],
  plugins: PluginInfo[],
): Map<string, Integration[]> {
  const pluginMap = new Map(plugins.map((p) => [p.plugin_id, p]));
  const groups = new Map<string, Integration[]>();

  for (const integ of integrations) {
    const plugin = pluginMap.get(integ.plugin_id);
    // Use the plugin's primary capability, falling back to integration_type.
    const capability =
      plugin?.capabilities?.[0] ?? integ.integration_type ?? "other";

    if (!groups.has(capability)) {
      groups.set(capability, []);
    }
    groups.get(capability)!.push(integ);
  }

  return groups;
}
