import { File01Icon, PinIcon, RefreshIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useSetAtom } from "jotai/react";
import { useCallback, useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  decideArtifactConfirmation,
  getArtifactConfirmations,
  getArtifacts,
  listArtifactFiles,
  pinArtifact,
  unpinArtifact,
} from "../api/client";
import type { Artifact, ArtifactConfirmation, ArtifactFileInfo } from "../api/types";
import { artifactViewerAtom } from "../atoms";
import { formatBytes } from "../lib/artifact";
import { useToast } from "../lib/useToast";
import { LoadingState, StatusMessage } from "./ui";

/** Lists artifact folders, their files and pending pin requests. */
export function ArtifactsPanel() {
  const toast = useToast();
  const setViewer = useSetAtom(artifactViewerAtom);
  const [artifacts, setArtifacts] = useState<Artifact[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [files, setFiles] = useState<ArtifactFileInfo[]>([]);
  const [confirmations, setConfirmations] = useState<ArtifactConfirmation[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const [list, pending] = await Promise.all([getArtifacts(), getArtifactConfirmations()]);
      setArtifacts(list.artifacts);
      setConfirmations(pending.confirmations.filter((item) => item.status === "pending"));
      setSelected((current) => current ?? list.artifacts[0]?.id ?? null);
      setError(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!selected) {
      setFiles([]);
      return;
    }
    let cancelled = false;
    listArtifactFiles(selected)
      .then((result) => {
        if (!cancelled) setFiles(result.files);
      })
      .catch((cause: unknown) => {
        if (!cancelled) setError(cause instanceof Error ? cause.message : String(cause));
      });
    return () => {
      cancelled = true;
    };
  }, [selected]);

  const changePin = async (artifact: Artifact) => {
    try {
      const result = artifact.pinned
        ? await unpinArtifact(artifact.id)
        : await pinArtifact(artifact.id);
      setArtifacts((current) =>
        current.map((item) => (item.id === artifact.id ? result.artifact : item)),
      );
      toast(artifact.pinned ? "Artifact unpinned." : "Artifact pinned.", "success");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  };

  const decide = async (id: string, decision: "approve" | "reject") => {
    try {
      await decideArtifactConfirmation(id, decision);
      await refresh();
      toast(
        decision === "approve" ? "Confirmation approved." : "Confirmation rejected.",
        "success",
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  };

  if (loading) return <LoadingState label="Loading artifacts" />;

  return (
    <div className="flex flex-col gap-6 text-sm">
      <div className="flex items-center justify-between gap-2">
        <div>
          <h3 className="font-heading text-lg font-semibold">Artifacts</h3>
          <p className="text-xs text-muted-foreground">
            Files created by the assistant. Pinned folders never expire.
          </p>
        </div>
        <Button
          variant="outline"
          size="icon-sm"
          onClick={() => void refresh()}
          aria-label="Refresh artifacts"
        >
          <HugeiconsIcon icon={RefreshIcon} strokeWidth={2} />
        </Button>
      </div>
      <StatusMessage>{error}</StatusMessage>

      {confirmations.length > 0 ? (
        <section className="flex flex-col gap-2">
          <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Pending requests
          </h4>
          {confirmations.map((item) => (
            <div key={item.id} className="flex flex-col gap-2 rounded-2xl border border-border p-3">
              <p className="text-xs text-muted-foreground">
                Agent requested <strong>{item.action}</strong> for {item.artifactId}
              </p>
              {item.context ? <p className="text-sm">{item.context}</p> : null}
              <div className="flex gap-2">
                <Button size="sm" onClick={() => void decide(item.id, "approve")}>
                  Approve
                </Button>
                <Button variant="outline" size="sm" onClick={() => void decide(item.id, "reject")}>
                  Reject
                </Button>
              </div>
            </div>
          ))}
        </section>
      ) : null}

      <section className="flex flex-col gap-2">
        <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Folders
        </h4>
        {artifacts.length === 0 ? (
          <p className="text-xs text-muted-foreground">No artifacts yet.</p>
        ) : (
          artifacts.map((artifact) => (
            <button
              key={artifact.id}
              type="button"
              onClick={() => setSelected(artifact.id)}
              className={`flex items-center gap-2 rounded-2xl border p-3 text-left transition ${
                selected === artifact.id
                  ? "border-primary bg-primary/5"
                  : "border-border hover:bg-muted"
              }`}
            >
              <HugeiconsIcon
                icon={File01Icon}
                strokeWidth={2}
                className="size-4 shrink-0 text-muted-foreground"
              />
              <span className="min-w-0 flex-1">
                <span className="block truncate font-mono text-xs">{artifact.id}</span>
                <span className="block text-xs text-muted-foreground">
                  {formatBytes(artifact.sizeBytes)} ·{" "}
                  {new Date(artifact.updatedAt).toLocaleString()}
                </span>
              </span>
              {artifact.pinned ? <Badge variant="secondary">Pinned</Badge> : null}
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={artifact.pinned ? "Unpin artifact" : "Pin artifact"}
                onClick={(event) => {
                  event.stopPropagation();
                  void changePin(artifact);
                }}
              >
                <HugeiconsIcon icon={PinIcon} strokeWidth={2} />
              </Button>
            </button>
          ))
        )}
      </section>

      {selected ? (
        <section className="flex flex-col gap-2">
          <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Files
          </h4>
          {files.length === 0 ? (
            <p className="text-xs text-muted-foreground">This artifact is empty.</p>
          ) : (
            files.map((file) => (
              <button
                key={file.path}
                type="button"
                onClick={() => setViewer({ artifactId: selected, path: file.path })}
                className="flex items-center gap-2 rounded-2xl border border-border px-3 py-2 text-left hover:bg-muted"
              >
                <HugeiconsIcon
                  icon={File01Icon}
                  strokeWidth={2}
                  className="size-4 shrink-0 text-muted-foreground"
                />
                <span className="min-w-0 flex-1 truncate font-mono text-xs">{file.path}</span>
                <span className="text-xs text-muted-foreground">{formatBytes(file.sizeBytes)}</span>
              </button>
            ))
          )}
        </section>
      ) : null}
    </div>
  );
}
