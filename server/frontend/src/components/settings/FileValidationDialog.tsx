import { useCallback, useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Loader2, RefreshCw, Trash2, TriangleAlert } from "lucide-react";
import {
  ApiError,
  removeMissingIntegrationRecords,
  validateIntegrationFiles,
  type FileValidationReport,
  type Integration,
} from "@/api/client";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { useToast } from "@/components/ui/toast";

interface FileValidationDialogProps {
  integration: Integration;
  open: boolean;
  onClose: () => void;
}

function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    return error.responseText?.trim() || error.message;
  }
  if (error instanceof Error) return error.message;
  return "The connection could not check its files.";
}

export function FileValidationDialog({
  integration,
  open,
  onClose,
}: FileValidationDialogProps) {
  const queryClient = useQueryClient();
  const { notify } = useToast();
  const [report, setReport] = useState<FileValidationReport | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [checking, setChecking] = useState(false);
  const [cleaning, setCleaning] = useState(false);
  const [error, setError] = useState("");

  const runValidation = useCallback(async () => {
    setChecking(true);
    setError("");
    try {
      const nextReport = await validateIntegrationFiles(integration.id);
      setReport(nextReport);
      setSelected(new Set(nextReport.missing.map((game) => game.id)));
    } catch (validationError) {
      setReport(null);
      setSelected(new Set());
      setError(errorText(validationError));
    } finally {
      setChecking(false);
    }
  }, [integration.id]);

  useEffect(() => {
    if (!open) {
      setReport(null);
      setSelected(new Set());
      setChecking(false);
      setCleaning(false);
      setError("");
      return;
    }
    void runValidation();
  }, [open, runValidation]);

  const selectedCount = selected.size;
  const selectedMissingFileCount = useMemo(
    () =>
      report?.missing
        .filter((game) => selected.has(game.id))
        .reduce((total, game) => total + game.missing_files.length, 0) ?? 0,
    [report, selected],
  );

  const toggleSelection = (sourceGameId: string) => {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(sourceGameId)) next.delete(sourceGameId);
      else next.add(sourceGameId);
      return next;
    });
  };

  const removeSelected = async () => {
    if (selectedCount === 0 || cleaning) return;
    setCleaning(true);
    setError("");
    const selectedIDs = Array.from(selected);
    try {
      const result = await removeMissingIntegrationRecords(
        integration.id,
        selectedIDs,
      );
      const removed = new Set(result.removed_source_game_ids);
      setReport((current) =>
        current
          ? {
              ...current,
              missing: current.missing.filter((game) => !removed.has(game.id)),
              missing_file_count: current.missing
                .filter((game) => !removed.has(game.id))
                .reduce((total, game) => total + game.missing_files.length, 0),
            }
          : current,
      );
      setSelected(new Set());
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["integrations"] }),
        queryClient.invalidateQueries({ queryKey: ["integration-games", integration.id] }),
        queryClient.invalidateQueries({ queryKey: ["library-stats"] }),
        queryClient.invalidateQueries({ queryKey: ["games"] }),
      ]);
      notify({
        title: `${removed.size} stale game ${removed.size === 1 ? "copy was" : "copies were"} removed`,
        description:
          "Only MGA's library records were removed. No files or saves were deleted.",
        tone: "success",
      });
    } catch (cleanupError) {
      setError(errorText(cleanupError));
    } finally {
      setCleaning(false);
    }
  };

  const close = () => {
    if (!checking && !cleaning) onClose();
  };

  return (
    <Dialog
      open={open}
      onClose={close}
      title={`Check files — ${integration.label}`}
      className="max-w-2xl"
    >
      <div className="space-y-4">
        {checking ? (
          <div className="flex items-center gap-3 rounded-mga border border-mga-border bg-mga-bg p-4 text-sm text-mga-muted">
            <Loader2 size={18} className="animate-spin text-mga-accent" />
            Checking the files available through {integration.label}…
          </div>
        ) : null}

        {error ? (
          <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-4">
            <div className="flex items-start gap-3">
              <TriangleAlert size={18} className="mt-0.5 shrink-0 text-red-300" />
              <div>
                <p className="font-medium text-red-100">Files could not be checked</p>
                <p className="mt-1 text-sm text-red-100/80">{error}</p>
                <p className="mt-2 text-xs text-red-100/70">
                  Check this connection and try again. MGA has not marked or removed any games.
                </p>
              </div>
            </div>
          </div>
        ) : null}

        {!checking && report && report.missing.length === 0 && report.failures.length === 0 ? (
          <div className="flex items-start gap-3 rounded-mga border border-emerald-500/30 bg-emerald-500/10 p-4">
            <CheckCircle2 size={18} className="mt-0.5 shrink-0 text-emerald-300" />
            <div>
              <p className="font-medium text-emerald-100">All game files were found</p>
              <p className="mt-1 text-sm text-emerald-100/75">
                Checked {report.files_checked} {report.files_checked === 1 ? "file" : "files"} across{" "}
                {report.total_checked} {report.total_checked === 1 ? "game" : "games"}.
              </p>
            </div>
          </div>
        ) : null}

        {!checking && report && report.missing.length > 0 ? (
          <>
            <div className="rounded-mga border border-amber-500/30 bg-amber-500/10 p-4">
              <p className="font-medium text-amber-100">
                {report.missing.length} {report.missing.length === 1 ? "game copy is" : "game copies are"} no longer found
              </p>
              <p className="mt-1 text-sm text-amber-100/75">
                Select stale copies to remove from MGA. This does not delete files or saves.
              </p>
              <p className="mt-2 text-xs text-amber-100/65">
                Checked {report.files_checked} files; {report.missing_file_count} could not be found through {report.integration_label}.
              </p>
            </div>

            <div className="max-h-[24rem] space-y-3 overflow-auto pr-1">
              {report.missing.map((game) => (
                <label
                  key={game.id}
                  className="block cursor-pointer rounded-mga border border-mga-border bg-mga-bg p-3 hover:border-mga-muted/70"
                >
                  <div className="flex items-start gap-3">
                    <input
                      type="checkbox"
                      checked={selected.has(game.id)}
                      onChange={() => toggleSelection(game.id)}
                      className="mt-1 h-4 w-4 accent-mga-accent"
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-start justify-between gap-3">
                        <div>
                          <p className="font-medium text-mga-text">{game.title}</p>
                          <p className="mt-0.5 text-xs text-mga-muted">
                            Found in {report.integration_label}
                          </p>
                        </div>
                        <span className="shrink-0 text-xs text-amber-300">
                          {game.missing_files.length} missing
                        </span>
                      </div>
                      <details className="mt-2 text-xs text-mga-muted">
                        <summary className="cursor-pointer hover:text-mga-text">
                          Technical details
                        </summary>
                        {game.root_path ? (
                          <p className="mt-2 break-all">Game folder: {game.root_path}</p>
                        ) : null}
                        <ul className="mt-2 space-y-1">
                          {game.missing_files.map((file) => (
                            <li
                              key={`${game.id}:${file.object_id ?? file.path}`}
                              className="break-all"
                            >
                              {file.path || `Provider item ${file.object_id}`}
                            </li>
                          ))}
                        </ul>
                      </details>
                    </div>
                  </div>
                </label>
              ))}
            </div>
          </>
        ) : null}

        {!checking && report && report.failures.length > 0 ? (
          <div className="rounded-mga border border-amber-500/30 bg-amber-500/5 p-4">
            <p className="font-medium text-amber-100">
              {report.failures.length} {report.failures.length === 1 ? "game needs" : "games need"} a rescan
            </p>
            <div className="mt-2 space-y-2 text-xs text-amber-100/75">
              {report.failures.map((failure, index) => (
                <div key={`${failure.source_game_id ?? "failure"}:${index}`}>
                  {failure.title ? <p className="font-medium text-amber-100">{failure.title}</p> : null}
                  <p>{failure.message}</p>
                </div>
              ))}
            </div>
          </div>
        ) : null}

        <div className="flex flex-wrap justify-end gap-2 border-t border-mga-border pt-4">
          <Button variant="ghost" onClick={close} disabled={checking || cleaning}>
            Close
          </Button>
          {!checking ? (
            <Button variant="outline" onClick={() => void runValidation()} disabled={cleaning}>
              <RefreshCw size={14} />
              Check again
            </Button>
          ) : null}
          {report && report.missing.length > 0 ? (
            <Button
              variant="default"
              onClick={() => void removeSelected()}
              disabled={selectedCount === 0 || cleaning}
              className="bg-red-500/20 text-red-200 hover:bg-red-500/30"
              title={
                selectedCount > 0
                  ? `Remove ${selectedCount} stale library records covering ${selectedMissingFileCount} missing files`
                  : "Select at least one stale game copy"
              }
            >
              {cleaning ? <Loader2 size={14} className="animate-spin" /> : <Trash2 size={14} />}
              {cleaning ? "Removing…" : `Remove ${selectedCount} from library`}
            </Button>
          ) : null}
        </div>
      </div>
    </Dialog>
  );
}
