import type { ViewerEventHandler, ViewerOptions, ViewerState } from "@file-viewer/react";
import { registerFileViewerMermaidLoader } from "@file-viewer/renderer-text";
import { CopyIcon, DownloadIcon, ExpandIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useAtom, useSetAtom } from "jotai/react";
import type { LazyExoticComponent } from "react";
import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { Button, buttonVariants } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { artifactRawUrl } from "../api/client";
import { type ArtifactRef, artifactViewerAtom, artifactViewerThemeAtom } from "../atoms";
import {
  artifactFileName,
  artifactKind,
  formatBytes,
  loadArtifactUrl,
  parseArtifactRef,
} from "../lib/artifact";
import { useToast } from "../lib/useToast";

registerFileViewerMermaidLoader(() => import("../lib/fileViewerPeleMermaid"));

// Heavy renderer chunks load only when a file preview is first shown.
const FileViewer: LazyExoticComponent<typeof import("@file-viewer/react").FileViewer> = lazy(() =>
  import("@file-viewer/react").then((module) => ({ default: module.FileViewer })),
);

function useArtifactUrl(artifactId: string, filePath: string) {
  const [url, setUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    let cancelled = false;
    setUrl(null);
    setError(null);
    loadArtifactUrl({ artifactId, path: filePath })
      .then((value) => {
        if (!cancelled) setUrl(value);
      })
      .catch((cause: unknown) => {
        if (!cancelled) setError(cause instanceof Error ? cause.message : String(cause));
      });
    return () => {
      cancelled = true;
    };
  }, [artifactId, filePath]);
  return { url, error };
}

function useViewerOptions(compact: boolean): ViewerOptions | null {
  const [theme] = useAtom(artifactViewerThemeAtom);
  const [preset, setPreset] = useState<unknown>(null);
  useEffect(() => {
    let cancelled = false;
    import("@file-viewer/preset-standard").then((module) => {
      if (!cancelled) setPreset(module.default);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return useMemo(() => {
    if (!preset) return null;
    return {
      preset,
      rendererMode: "replace",
      theme,
      toolbar: { position: compact ? "top-right" : "bottom-right" },
    } as ViewerOptions;
  }, [preset, theme, compact]);
}

function FilePreview({ ref, compact }: { ref: ArtifactRef; compact: boolean }) {
  const { url, error } = useArtifactUrl(ref.artifactId, ref.path);
  const options = useViewerOptions(compact);
  const setTheme = useSetAtom(artifactViewerThemeAtom);
  const [renderError, setRenderError] = useState<string | null>(null);
  const name = artifactFileName(ref);
  const kind = artifactKind(name);

  const onEvent = useCallback<ViewerEventHandler>(
    (event) => {
      if (event.type === "theme-change") setTheme(event.payload);
    },
    [setTheme],
  );
  const onStateChange = useCallback((state: ViewerState) => {
    setRenderError(
      state.error
        ? state.error instanceof Error
          ? state.error.message
          : String(state.error)
        : null,
    );
  }, []);

  if (error || renderError) {
    return <p className="p-3 text-xs text-destructive">{error ?? renderError}</p>;
  }
  if (!url) return <p className="p-3 text-xs text-muted-foreground">Loading file...</p>;
  if (kind === "image") {
    return <img src={url} alt={name} className="max-h-64 w-full object-contain" />;
  }
  if (!options) return <p className="p-3 text-xs text-muted-foreground">Loading viewer...</p>;
  return (
    <div className={compact ? "h-64 w-full" : "h-full w-full min-h-0"}>
      <Suspense fallback={<p className="p-3 text-xs text-muted-foreground">Loading viewer...</p>}>
        <FileViewer
          url={url}
          filename={name}
          options={options}
          onEvent={onEvent}
          onStateChange={onStateChange}
        />
      </Suspense>
    </div>
  );
}

/** Inline preview card produced by `[name](artifact://...)` Markdown links. */
export function ArtifactEmbed({
  ref: artifactRef,
  sizeBytes,
}: {
  ref: ArtifactRef;
  sizeBytes?: number;
}) {
  const setViewer = useSetAtom(artifactViewerAtom);
  const toast = useToast();
  const name = artifactFileName(artifactRef);
  const rawUrl = artifactRawUrl(artifactRef.artifactId, artifactRef.path);

  const copyLink = async () => {
    try {
      await navigator.clipboard.writeText(`${window.location.origin}${rawUrl}`);
      toast("Artifact link copied.", "success");
    } catch {
      toast("Could not copy the artifact link.", "warning");
    }
  };

  return (
    <div className="my-2 w-full max-w-2xl overflow-hidden rounded-2xl border border-border bg-card text-sm shadow-sm">
      <div className="flex items-center justify-between gap-2 border-b border-border px-3 py-2">
        <div className="min-w-0">
          <p className="truncate font-medium text-foreground" title={artifactRef.path}>
            {name}
          </p>
          <p className="truncate text-xs text-muted-foreground">
            {artifactRef.artifactId}
            {sizeBytes === undefined ? "" : ` · ${formatBytes(sizeBytes)}`}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <a
            href={rawUrl}
            download={name}
            aria-label={`Download ${name}`}
            className={buttonVariants({ variant: "ghost", size: "icon-sm" })}
          >
            <HugeiconsIcon icon={DownloadIcon} strokeWidth={2} />
          </a>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => void copyLink()}
            aria-label="Copy artifact link"
          >
            <HugeiconsIcon icon={CopyIcon} strokeWidth={2} />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => setViewer({ ...artifactRef })}
            aria-label={`Open ${name} full screen`}
          >
            <HugeiconsIcon icon={ExpandIcon} strokeWidth={2} />
          </Button>
        </div>
      </div>
      <div className="bg-background/40">
        <FilePreview ref={artifactRef} compact />
      </div>
    </div>
  );
}

/** Inline image produced by `![alt](artifact://...)` Markdown links. */
export function ArtifactInlineImage({ ref: artifactRef, alt }: { ref: ArtifactRef; alt: string }) {
  const setViewer = useSetAtom(artifactViewerAtom);
  const { url, error } = useArtifactUrl(artifactRef.artifactId, artifactRef.path);
  if (error) return <span className="text-xs text-destructive">{error}</span>;
  if (!url) return <span className="text-xs text-muted-foreground">Loading image...</span>;
  return (
    <button
      type="button"
      onClick={() => setViewer({ ...artifactRef })}
      aria-label={`Open ${alt}`}
      className="cursor-zoom-in"
    >
      <img
        src={url}
        alt={alt}
        className="max-h-80 max-w-full rounded-xl border border-border object-contain"
      />
    </button>
  );
}

/** Plain link for bare artifact:// URLs. Opens the viewer without a card. */
export function ArtifactPlainLink({
  ref: artifactRef,
  children,
}: {
  ref: ArtifactRef;
  children: string;
}) {
  const setViewer = useSetAtom(artifactViewerAtom);
  return (
    <button
      type="button"
      onClick={() => setViewer({ ...artifactRef })}
      className="text-primary underline underline-offset-2"
    >
      {children}
    </button>
  );
}

/** Single global full-screen viewer, driven by artifactViewerAtom. */
export function ArtifactFocusDialog() {
  const [ref, setRef] = useAtom(artifactViewerAtom);
  const toast = useToast();
  const name = ref ? artifactFileName(ref) : "";
  const rawUrl = ref ? artifactRawUrl(ref.artifactId, ref.path) : "";

  const copyLink = async () => {
    if (!ref) return;
    try {
      await navigator.clipboard.writeText(`${window.location.origin}${rawUrl}`);
      toast("Artifact link copied.", "success");
    } catch {
      toast("Could not copy the artifact link.", "warning");
    }
  };

  return (
    <Dialog open={ref !== null} onOpenChange={(open) => !open && setRef(null)}>
      <DialogContent className="top-0! left-0! flex! h-dvh! w-screen! max-w-none! translate-x-0! translate-y-0! flex-col! gap-0! rounded-none! p-0! sm:top-1/2! sm:left-1/2! sm:h-[min(92vh,1000px)]! sm:w-[min(1200px,96vw)]! sm:-translate-x-1/2! sm:-translate-y-1/2! sm:rounded-3xl!">
        <DialogTitle className="sr-only">Artifact viewer</DialogTitle>
        <DialogDescription className="sr-only">
          Preview of an artifact file created by the assistant.
        </DialogDescription>
        {ref ? (
          <>
            <div className="flex shrink-0 items-center gap-2 border-b border-border px-4 py-2">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{name}</p>
                <p className="truncate text-xs text-muted-foreground">
                  {ref.artifactId}/{ref.path}
                </p>
              </div>
              <a
                href={rawUrl}
                download={name}
                aria-label={`Download ${name}`}
                className={buttonVariants({ variant: "ghost", size: "icon-sm" })}
              >
                <HugeiconsIcon icon={DownloadIcon} strokeWidth={2} />
              </a>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => void copyLink()}
                aria-label="Copy artifact link"
              >
                <HugeiconsIcon icon={CopyIcon} strokeWidth={2} />
              </Button>
            </div>
            <div className="min-h-0 flex-1">
              <FilePreview ref={ref} compact={false} />
            </div>
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

/** Validates a parsed artifact reference before passing it to the viewer. */
export function validArtifactLink(value: string): ArtifactRef | null {
  return parseArtifactRef(value);
}
