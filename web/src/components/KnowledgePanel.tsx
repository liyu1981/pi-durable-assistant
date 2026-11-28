import {
  ArrowLeft02Icon,
  BookOpen01Icon,
  Cancel01Icon,
  Delete02Icon,
  Download04Icon,
  FolderAddIcon,
  FolderOpenIcon,
  RefreshIcon,
  Upload04Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Textarea } from "@/components/ui/textarea";
import {
  createKnowledgeVault,
  deleteKnowledgeFile,
  deleteKnowledgeVault,
  knowledgeRawUrl,
  listKnowledgeFiles,
  listKnowledgeVaults,
  readKnowledgeFile,
  updateKnowledgeVault,
  uploadKnowledgeFile,
} from "../api/client";
import type { KnowledgeFileContent, KnowledgeFileInfo, KnowledgeVault } from "../api/types";
import { formatBytes } from "../lib/artifact";
import { useToast } from "../lib/useToast";
import { ConfirmDialog, EmptyState, Field, LoadingState, StatusMessage } from "./ui";

/** Reads a browser File as base64, which the API stores byte for byte. */
function readFileAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error(`Could not read ${file.name}`));
    reader.onload = () => {
      const result = reader.result;
      if (typeof result !== "string") {
        reject(new Error(`Could not read ${file.name}`));
        return;
      }
      const comma = result.indexOf(",");
      resolve(comma === -1 ? "" : result.slice(comma + 1));
    };
    reader.readAsDataURL(file);
  });
}

function errorText(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause);
}

/** One vault's files, with upload, preview and delete controls. */
function VaultFiles({
  vault,
  onChanged,
  onBack,
}: {
  vault: KnowledgeVault;
  onChanged: () => void;
  onBack: () => void;
}) {
  const toast = useToast();
  const [files, setFiles] = useState<KnowledgeFileInfo[]>([]);
  const [preview, setPreview] = useState<KnowledgeFileContent | null>(null);
  const [pendingDelete, setPendingDelete] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);

  const refresh = useCallback(async () => {
    try {
      const result = await listKnowledgeFiles(vault.id);
      setFiles(result.files);
      setError(null);
    } catch (cause) {
      setError(errorText(cause));
    }
  }, [vault.id]);

  useEffect(() => {
    void refresh();
    setPreview(null);
  }, [refresh]);

  const upload = async (selected: FileList | null) => {
    if (!selected || selected.length === 0) return;
    setBusy(true);
    try {
      for (const file of Array.from(selected)) {
        await uploadKnowledgeFile(vault.id, file.name, await readFileAsBase64(file));
      }
      await refresh();
      onChanged();
      toast(
        selected.length === 1
          ? `Uploaded ${selected[0].name}.`
          : `Uploaded ${selected.length} files.`,
        "success",
      );
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setBusy(false);
      if (inputRef.current) inputRef.current.value = "";
    }
  };

  const open = async (file: KnowledgeFileInfo) => {
    setBusy(true);
    try {
      setPreview(await readKnowledgeFile(vault.id, file.path));
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (path: string) => {
    setBusy(true);
    try {
      await deleteKnowledgeFile(vault.id, path);
      if (preview?.path === path) setPreview(null);
      await refresh();
      onChanged();
      toast("File deleted.", "success");
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setBusy(false);
      setPendingDelete(null);
    }
  };

  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="icon-sm" aria-label="Back to vaults" onClick={onBack}>
          <HugeiconsIcon icon={ArrowLeft02Icon} strokeWidth={2} />
        </Button>
        <div className="min-w-0 flex-1">
          <h4 className="truncate font-heading text-base font-semibold">{vault.name}</h4>
          <p className="truncate text-xs text-muted-foreground">
            {vault.fileCount} file{vault.fileCount === 1 ? "" : "s"} ·{" "}
            {formatBytes(vault.sizeBytes)}
            {vault.description ? ` · ${vault.description}` : ""}
          </p>
        </div>
        <Button
          size="sm"
          disabled={busy}
          onClick={() => inputRef.current?.click()}
          title="Upload files into this vault"
        >
          <HugeiconsIcon icon={Upload04Icon} strokeWidth={2} />
          Upload
        </Button>
        <input
          ref={inputRef}
          type="file"
          multiple
          className="hidden"
          onChange={(event) => void upload(event.target.files)}
        />
      </div>
      <StatusMessage>{error}</StatusMessage>

      {files.length === 0 ? (
        <p className="rounded-2xl border border-dashed border-border p-4 text-xs text-muted-foreground">
          This vault is empty. Upload any file type with the button above.
        </p>
      ) : (
        <ul className="flex flex-col gap-2">
          {files.map((file) => (
            <li
              key={file.path}
              className="flex items-center gap-2 rounded-2xl border border-border px-3 py-2"
            >
              <HugeiconsIcon
                icon={BookOpen01Icon}
                strokeWidth={2}
                className="size-4 shrink-0 text-muted-foreground"
              />
              <button
                type="button"
                className="min-w-0 flex-1 truncate text-left font-mono text-xs hover:underline"
                title={`Preview ${file.path}`}
                onClick={() => void open(file)}
              >
                {file.path}
              </button>
              <span className="shrink-0 text-xs text-muted-foreground">
                {formatBytes(file.sizeBytes)}
              </span>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={`Delete ${file.path}`}
                title="Delete file"
                disabled={busy}
                onClick={() => setPendingDelete(file.path)}
              >
                <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
              </Button>
            </li>
          ))}
        </ul>
      )}

      {preview ? (
        <Card size="sm">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-sm">
              <span className="min-w-0 flex-1 truncate font-mono">{preview.path}</span>
              <Badge variant="outline">{preview.isText ? "text" : preview.mimeType}</Badge>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label="Close preview"
                onClick={() => setPreview(null)}
              >
                <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
              </Button>
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            {preview.isText ? (
              <ScrollArea className="h-64 rounded-xl border border-border bg-muted/30 p-3">
                <pre className="whitespace-pre-wrap break-words font-mono text-xs">
                  {preview.content ?? ""}
                </pre>
              </ScrollArea>
            ) : (
              <div className="flex flex-col gap-2">
                <p className="text-xs text-muted-foreground">
                  Binary file ({preview.mimeType}, {formatBytes(preview.sizeBytes)}). Open it in a
                  new tab or download it.
                </p>
                <div className="flex flex-wrap gap-2">
                  <a
                    href={knowledgeRawUrl(vault.id, preview.path)}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-1 text-xs underline"
                  >
                    <HugeiconsIcon icon={FolderOpenIcon} strokeWidth={2} className="size-4" />
                    Open
                  </a>
                  <a
                    href={knowledgeRawUrl(vault.id, preview.path)}
                    download
                    className="inline-flex items-center gap-1 text-xs underline"
                  >
                    <HugeiconsIcon icon={Download04Icon} strokeWidth={2} className="size-4" />
                    Download
                  </a>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      ) : null}

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(next) => {
          if (!next) setPendingDelete(null);
        }}
        title="Delete file?"
        description={`${pendingDelete ?? ""} is removed from this vault permanently.`}
        confirmLabel="Delete"
        destructive
        onConfirm={() => {
          if (pendingDelete !== null) void remove(pendingDelete);
        }}
      />
    </section>
  );
}

/** Knowledge vaults: create, describe, upload, browse and delete. */
export function KnowledgePanel() {
  const toast = useToast();
  const [vaults, setVaults] = useState<KnowledgeVault[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<KnowledgeVault | null>(null);
  const [editing, setEditing] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const result = await listKnowledgeVaults();
      setVaults(result.vaults);
      setSelectedId((current) =>
        current && result.vaults.some((vault) => vault.id === current)
          ? current
          : (result.vaults[0]?.id ?? null),
      );
      setError(null);
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const selected = vaults.find((vault) => vault.id === selectedId) ?? null;

  const create = async () => {
    if (!name.trim()) return;
    setBusy(true);
    try {
      const result = await createKnowledgeVault(name, description);
      setName("");
      setDescription("");
      await refresh();
      setSelectedId(result.vault.id);
      toast("Vault created.", "success");
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setBusy(false);
    }
  };

  const saveDetails = async () => {
    if (!selected || !name.trim()) return;
    setBusy(true);
    try {
      await updateKnowledgeVault(selected.id, name, description);
      setEditing(false);
      await refresh();
      toast("Vault updated.", "success");
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (vault: KnowledgeVault) => {
    setBusy(true);
    try {
      await deleteKnowledgeVault(vault.id);
      if (selectedId === vault.id) setSelectedId(null);
      await refresh();
      toast("Vault deleted.", "success");
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setBusy(false);
      setPendingDelete(null);
    }
  };

  if (loading) return <LoadingState label="Loading knowledge vaults" />;

  return (
    <div className="flex flex-col gap-6 text-sm">
      <div className="flex items-center justify-between gap-2">
        <div>
          <h3 className="font-heading text-lg font-semibold">Knowledge vaults</h3>
          <p className="text-xs text-muted-foreground">
            Uploaded reference files the assistant can list and read. Create one vault per topic and
            describe what belongs in it.
          </p>
        </div>
        <Button
          variant="outline"
          size="icon-sm"
          aria-label="Refresh vaults"
          onClick={() => void refresh()}
        >
          <HugeiconsIcon icon={RefreshIcon} strokeWidth={2} />
        </Button>
      </div>
      <StatusMessage>{error}</StatusMessage>

      <Card size="sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-sm">
            <HugeiconsIcon icon={FolderAddIcon} strokeWidth={2} />
            New vault
          </CardTitle>
        </CardHeader>
        <CardContent>
          <form
            className="flex flex-col gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              void create();
            }}
          >
            <Field label="Name">
              <Input
                value={name}
                maxLength={120}
                required
                placeholder="Research papers"
                onChange={(event) => setName(event.target.value)}
              />
            </Field>
            <Field label="Description">
              <Textarea
                value={description}
                maxLength={2000}
                rows={2}
                placeholder="What this vault contains, so the assistant knows when to use it"
                onChange={(event) => setDescription(event.target.value)}
              />
            </Field>
            <Button type="submit" disabled={busy || !name.trim()}>
              {busy ? "Working..." : "Create vault"}
            </Button>
          </form>
        </CardContent>
      </Card>

      {vaults.length === 0 ? (
        <EmptyState title="No vaults yet" hint="Create a vault above, then upload files into it." />
      ) : (
        <section className="flex flex-col gap-2">
          <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Vaults
          </h4>
          {vaults.map((vault) => (
            <div
              key={vault.id}
              className={`flex items-start gap-2 rounded-2xl border p-3 transition ${
                selectedId === vault.id ? "border-primary bg-primary/5" : "border-border"
              }`}
            >
              <button
                type="button"
                className="min-w-0 flex-1 text-left"
                onClick={() => setSelectedId(vault.id)}
                title={vault.description || vault.name}
              >
                <span className="flex items-center gap-2">
                  <span className="truncate font-medium">{vault.name}</span>
                  <Badge variant="secondary">
                    {vault.fileCount} file{vault.fileCount === 1 ? "" : "s"}
                  </Badge>
                </span>
                <span className="mt-1 block truncate text-xs text-muted-foreground">
                  {vault.description || "No description yet"}
                </span>
                <span className="mt-1 block font-mono text-[10px] text-muted-foreground">
                  {vault.id} · {formatBytes(vault.sizeBytes)}
                </span>
              </button>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={`Delete vault ${vault.name}`}
                title="Delete vault"
                disabled={busy}
                onClick={() => setPendingDelete(vault)}
              >
                <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
              </Button>
            </div>
          ))}
        </section>
      )}

      {selected ? (
        <>
          {editing ? (
            <Card size="sm">
              <CardHeader>
                <CardTitle className="text-sm">Edit vault</CardTitle>
              </CardHeader>
              <CardContent>
                <form
                  className="flex flex-col gap-3"
                  onSubmit={(event) => {
                    event.preventDefault();
                    void saveDetails();
                  }}
                >
                  <Field label="Name">
                    <Input
                      value={name}
                      maxLength={120}
                      required
                      onChange={(event) => setName(event.target.value)}
                    />
                  </Field>
                  <Field label="Description">
                    <Textarea
                      value={description}
                      maxLength={2000}
                      rows={2}
                      onChange={(event) => setDescription(event.target.value)}
                    />
                  </Field>
                  <div className="flex gap-2">
                    <Button type="submit" size="sm" disabled={busy || !name.trim()}>
                      Save
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setEditing(false)}
                    >
                      Cancel
                    </Button>
                  </div>
                </form>
              </CardContent>
            </Card>
          ) : (
            <Button
              variant="outline"
              size="sm"
              className="self-start"
              onClick={() => {
                setName(selected.name);
                setDescription(selected.description);
                setEditing(true);
              }}
            >
              Edit name and description
            </Button>
          )}
          <VaultFiles
            vault={selected}
            onChanged={() => void refresh()}
            onBack={() => setSelectedId(null)}
          />
        </>
      ) : null}

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(next) => {
          if (!next) setPendingDelete(null);
        }}
        title="Delete vault?"
        description={`${pendingDelete?.name ?? ""} and all of its files are deleted permanently.`}
        confirmLabel="Delete"
        destructive
        onConfirm={() => {
          if (pendingDelete !== null) void remove(pendingDelete);
        }}
      />
    </div>
  );
}
