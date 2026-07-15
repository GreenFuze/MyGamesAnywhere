import { Clock, RefreshCw } from "lucide-react";
import type {
  LibraryScanScheduleConfig,
  LibraryScanScheduleStatus,
} from "@/api/client";
import { Button } from "@/components/ui/button";
import { useDateTimeFormat } from "@/hooks/useDateTimeFormat";

type Props = {
  status?: LibraryScanScheduleStatus;
  loading: boolean;
  saving: boolean;
  error?: string;
  onChange: (config: LibraryScanScheduleConfig) => void;
};

const intervalOptions = [5, 15, 30, 60, 180, 360, 720, 1440];

function intervalLabel(minutes: number): string {
  if (minutes < 60) return `${minutes} minutes`;
  if (minutes === 60) return "1 hour";
  if (minutes < 24 * 60) return `${minutes / 60} hours`;
  return "24 hours";
}

export function BackgroundScanScheduleCard({
  status,
  loading,
  saving,
  error,
  onChange,
}: Props) {
  const { format } = useDateTimeFormat();
  if (loading && !status) {
    return (
      <div className="rounded-mga border border-mga-border bg-mga-surface p-3 text-xs text-mga-muted">
        Loading scan schedule…
      </div>
    );
  }

  const enabled = status?.enabled ?? true;
  const intervalMinutes = status?.interval_minutes ?? 15;
  const running = status?.state === "running";
  const options = intervalOptions.includes(intervalMinutes)
    ? intervalOptions
    : [...intervalOptions, intervalMinutes].sort((a, b) => a - b);

  return (
    <div className="rounded-mga border border-mga-border bg-mga-surface p-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            {running ? (
              <RefreshCw size={14} className="animate-spin text-mga-accent" />
            ) : (
              <Clock size={14} className={enabled ? "text-emerald-400" : "text-mga-muted"} />
            )}
            <p className="text-xs font-medium text-mga-text">Automatic library scans</p>
            <span
              className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${
                running
                  ? "bg-mga-accent/15 text-mga-accent"
                  : enabled
                    ? "bg-emerald-500/15 text-emerald-300"
                    : "bg-mga-elevated text-mga-muted"
              }`}
            >
              {running ? "Running" : enabled ? "Scheduled" : "Paused"}
            </span>
          </div>
          {running && status?.active_job ? (
            <p className="mt-1 text-[11px] text-mga-accent">
              {status.active_job.current_integration_label
                ? `Scanning ${status.active_job.current_integration_label}`
                : status.active_job.current_phase || "Scan in progress"}
            </p>
          ) : enabled && status?.next_run_at ? (
            <p className="mt-1 text-[11px] text-mga-muted">
              Next scan: {format(status.next_run_at)}
            </p>
          ) : null}
          {status?.last_finished_at ? (
            <p className="mt-1 text-[11px] text-mga-muted">
              Last scan: {format(status.last_finished_at)}
              {status.last_status ? ` (${status.last_status})` : ""}
            </p>
          ) : null}
          {status?.last_error ? (
            <p className="mt-1 text-[11px] text-red-400">{status.last_error}</p>
          ) : null}
          {error ? <p className="mt-1 text-[11px] text-red-400">{error}</p> : null}
        </div>

        <div className="flex items-center gap-2">
          <select
            value={intervalMinutes}
            disabled={!enabled || saving}
            onChange={(event) =>
              onChange({ enabled, interval_minutes: Number(event.target.value) })
            }
            className="h-8 rounded-mga border border-mga-border bg-mga-elevated px-2 text-xs text-mga-text disabled:opacity-50"
            aria-label="Automatic scan interval"
          >
            {options.map((minutes) => (
              <option key={minutes} value={minutes}>
                Every {intervalLabel(minutes)}
              </option>
            ))}
          </select>
          <Button
            variant="outline"
            size="sm"
            disabled={saving}
            onClick={() => onChange({ enabled: !enabled, interval_minutes: intervalMinutes })}
          >
            {saving ? "Saving…" : enabled ? "Pause" : "Enable"}
          </Button>
        </div>
      </div>
    </div>
  );
}
