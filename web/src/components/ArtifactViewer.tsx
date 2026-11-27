import type {
  FileViewerHandle,
  ViewerEventHandler,
  ViewerOptions,
  ViewerState,
} from "@file-viewer/react";
import { registerFileViewerMermaidLoader } from "@file-viewer/renderer-text";
import {
  Cancel01Icon,
  CopyIcon,
  DownloadIcon,
  ExpandIcon,
  Maximize01Icon,
  Minimize01Icon,
  ZoomInIcon,
  ZoomOutIcon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useAtom, useAtomValue, useSetAtom } from "jotai/react";
import type { LazyExoticComponent, Ref, RefObject } from "react";
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Button, buttonVariants } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { artifactRawUrl } from "../api/client";
import {
  type ArtifactRef,
  artifactViewerAtom,
  artifactViewerThemeAtom,
  mermaidLightboxAtom,
} from "../atoms";
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
      toolbar: compact ? false : { position: "bottom-right" },
      text: { wrapLongLines: true },
    } as ViewerOptions;
  }, [preset, theme, compact]);
}

const THIN_SCROLLBAR_CSS = `
  * { scrollbar-width: thin !important; scrollbar-color: var(--muted-foreground, #888) transparent !important; }
  *::-webkit-scrollbar { width: 6px !important; height: 6px !important; }
  *::-webkit-scrollbar-track { background: transparent !important; }
  *::-webkit-scrollbar-thumb { background: var(--muted-foreground, #888) !important; border-radius: 999px !important; }
`;

function useThinScrollbars() {
  const hostRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    let observer: MutationObserver | null = null;
    const inject = (root: ParentNode) => {
      const target = root as Document | ShadowRoot;
      if (!("adoptedStyleSheets" in target) || target.querySelector?.("[data-pida-thin-scrollbar]"))
        return;
      const style = document.createElement("style");
      style.setAttribute("data-pida-thin-scrollbar", "");
      style.textContent = THIN_SCROLLBAR_CSS;
      target.appendChild(style);
    };
    const discover = () => {
      const host = hostRef.current;
      if (!host) return;
      for (const node of [host, ...Array.from(host.querySelectorAll("*"))]) {
        const shadow = (node as HTMLElement).shadowRoot;
        if (!shadow) continue;
        inject(shadow);
        if (!observer) {
          observer = new MutationObserver(() => inject(shadow));
          observer.observe(shadow, { childList: true, subtree: true });
        }
      }
    };
    const timer = window.setInterval(discover, 1200);
    discover();
    return () => {
      window.clearInterval(timer);
      observer?.disconnect();
    };
  }, []);
  return hostRef;
}

const MERMAID_DARK_VARS =
  "--pele-bg:#1e1e1e;--pele-fg:#e6e6e6;--pele-muted:#a0a0a0;" +
  "--pele-line:#e6e6e6;--pele-surface:#2a2a2a;--pele-surface-alt:#333333;" +
  "--pele-border:#555555;--pele-accent:#7aa2f7;";

/** Bind clicks on rendered Mermaid diagrams to the global lightbox. */
function useMermaidLightbox(hostRef: RefObject<HTMLDivElement | null>) {
  const setLightbox = useSetAtom(mermaidLightboxAtom);
  const theme = useAtomValue(artifactViewerThemeAtom);
  const themeRef = useRef(theme);
  useEffect(() => {
    themeRef.current = theme;
  }, [theme]);

  useEffect(() => {
    let observer: MutationObserver | null = null;
    const bound = new WeakSet<Element>();

    const showSvg = (element: Element) => {
      const svg = (
        element.matches("svg") ? element : element.querySelector("svg")
      ) as SVGElement | null;
      if (!svg) return;
      let html = svg.outerHTML;
      const dark = themeRef.current === "dark";
      if (dark && !html.includes("--pele-fg:#e6e6e6")) {
        html = /<svg\b[^>]*\bstyle="/.test(html)
          ? html.replace(/(<svg\b[^>]*\bstyle=")([^"]*)(")/, `$1$2;${MERMAID_DARK_VARS}$3`)
          : html.replace(/<svg\b/, `<svg style="${MERMAID_DARK_VARS}"`);
      }
      setLightbox({ svg: html, dark });
    };

    const attach = (root: ParentNode) => {
      root.querySelectorAll(".markdown-mermaid, .markdown-mermaid-svg").forEach((element) => {
        if (bound.has(element)) return;
        bound.add(element);
        (element as HTMLElement).style.cursor = "zoom-in";
        element.addEventListener("click", (event) => {
          event.preventDefault();
          event.stopPropagation();
          showSvg(element);
        });
      });
    };

    const discover = () => {
      const host = hostRef.current;
      if (!host) return;
      for (const node of [host, ...Array.from(host.querySelectorAll("*"))]) {
        const shadow = (node as HTMLElement).shadowRoot;
        if (!shadow) continue;
        attach(shadow);
        if (!observer) {
          observer = new MutationObserver(() => attach(shadow));
          observer.observe(shadow, { childList: true, subtree: true });
        }
      }
    };

    const timer = window.setInterval(discover, 1200);
    discover();
    return () => {
      window.clearInterval(timer);
      observer?.disconnect();
    };
  }, [hostRef, setLightbox]);
}

function FilePreview({
  ref,
  compact,
  viewerRef,
}: {
  ref: ArtifactRef;
  compact: boolean;
  viewerRef?: Ref<FileViewerHandle>;
}) {
  const { url, error } = useArtifactUrl(ref.artifactId, ref.path);
  const options = useViewerOptions(compact);
  const scrollHostRef = useThinScrollbars();
  useMermaidLightbox(scrollHostRef);
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
  const frameClass = compact
    ? "aspect-[4/3] max-h-[min(70vh,900px)] w-full overflow-hidden bg-background/40"
    : "h-full w-full min-h-0";
  if (kind === "image") {
    return (
      <div ref={scrollHostRef} className={frameClass}>
        <img src={url} alt={name} className="size-full object-contain" />
      </div>
    );
  }
  if (!options) return <p className="p-3 text-xs text-muted-foreground">Loading viewer...</p>;
  return (
    <div ref={scrollHostRef} className={frameClass}>
      <Suspense fallback={<p className="p-3 text-xs text-muted-foreground">Loading viewer...</p>}>
        <FileViewer
          ref={viewerRef}
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
    <div className="my-2 w-full overflow-hidden rounded-2xl border border-border bg-card text-sm shadow-sm">
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
  const [focused, setFocused] = useState(false);
  const viewerRef = useRef<FileViewerHandle | null>(null);
  const toast = useToast();
  const name = ref ? artifactFileName(ref) : "";
  const rawUrl = ref ? artifactRawUrl(ref.artifactId, ref.path) : "";

  useEffect(() => {
    if (!ref) setFocused(false);
  }, [ref]);

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
      <DialogContent
        showCloseButton={false}
        className={
          focused
            ? "top-0! left-0! flex! h-dvh! w-screen! max-w-none! translate-x-0! translate-y-0! flex-col! gap-0! rounded-none! p-0!"
            : "top-1/2! left-1/2! flex! h-[min(92vh,1000px)]! w-[min(1200px,96vw)]! max-w-none! -translate-x-1/2! -translate-y-1/2! flex-col! gap-0! rounded-3xl! p-0!"
        }
      >
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
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => void viewerRef.current?.zoomIn()}
                aria-label="Zoom in"
              >
                <HugeiconsIcon icon={ZoomInIcon} strokeWidth={2} />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => void viewerRef.current?.zoomOut()}
                aria-label="Zoom out"
              >
                <HugeiconsIcon icon={ZoomOutIcon} strokeWidth={2} />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => setFocused((value) => !value)}
                aria-label={focused ? "Exit full screen" : "Enter full screen"}
                aria-pressed={focused}
              >
                <HugeiconsIcon icon={focused ? Minimize01Icon : Maximize01Icon} strokeWidth={2} />
              </Button>
              <DialogClose
                render={<Button variant="ghost" size="icon-sm" aria-label="Close viewer" />}
              >
                <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
              </DialogClose>
            </div>
            <div className="min-h-0 flex-1">
              <FilePreview ref={ref} compact={false} viewerRef={viewerRef} />
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

/** Full-screen lightbox for a rendered Mermaid diagram. */
export function MermaidLightbox() {
  const [lightbox, setLightbox] = useAtom(mermaidLightboxAtom);
  const [scale, setScale] = useState(1);
  const [offset, setOffset] = useState({ x: 0, y: 0 });
  const [focused, setFocused] = useState(false);
  const dragRef = useRef<{ x: number; y: number; ox: number; oy: number } | null>(null);
  const dragHostRef = useRef<HTMLDivElement | null>(null);

  const close = useCallback(() => setLightbox(null), [setLightbox]);

  useEffect(() => {
    if (!lightbox) return;
    setScale(1);
    setOffset({ x: 0, y: 0 });
    setFocused(false);
  }, [lightbox]);

  useEffect(() => {
    if (!lightbox) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [lightbox, close]);

  useEffect(() => {
    const element = dragHostRef.current;
    if (!element || !lightbox) return;
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
      setScale((value) => Math.min(4, Math.max(0.25, value - event.deltaY * 0.001)));
    };
    element.addEventListener("wheel", onWheel, { passive: false });
    return () => element.removeEventListener("wheel", onWheel);
  }, [lightbox]);

  if (!lightbox) return null;

  return (
    <>
      <button
        type="button"
        aria-label="Close diagram"
        className="fixed inset-0 z-100 bg-black/70"
        onClick={close}
      />
      <div className="pointer-events-none fixed inset-0 z-101 flex items-center justify-center">
        <div
          className={`pointer-events-auto relative flex flex-col overflow-hidden ${
            focused ? "size-full rounded-none" : "h-[80vh] w-[80vw] rounded-lg"
          } ${lightbox.dark ? "bg-neutral-900 text-white" : "bg-white text-black"}`}
        >
          <div className="absolute left-2 top-2 z-10 flex gap-1 rounded-md bg-black/50 p-0.5">
            <Button
              variant="ghost"
              size="icon-sm"
              className="text-white/70 hover:bg-white/10 hover:text-white"
              aria-label="Zoom in"
              onClick={() => setScale((value) => Math.min(4, value + 0.25))}
            >
              <HugeiconsIcon icon={ZoomInIcon} strokeWidth={2} />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              className="text-white/70 hover:bg-white/10 hover:text-white"
              aria-label="Zoom out"
              onClick={() => setScale((value) => Math.max(0.25, value - 0.25))}
            >
              <HugeiconsIcon icon={ZoomOutIcon} strokeWidth={2} />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              className="text-white/70 hover:bg-white/10 hover:text-white"
              aria-label={focused ? "Exit focus" : "Enter focus"}
              aria-pressed={focused}
              onClick={() => setFocused((value) => !value)}
            >
              <HugeiconsIcon icon={focused ? Minimize01Icon : Maximize01Icon} strokeWidth={2} />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              className="text-white/70 hover:bg-rose-500/20 hover:text-rose-200"
              aria-label="Close diagram"
              onClick={close}
            >
              <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
            </Button>
          </div>
          {/* biome-ignore lint/a11y/noStaticElementInteractions: drag-to-pan is supplementary to the zoom controls. */}
          <div
            ref={dragHostRef}
            className="flex min-h-0 flex-1 cursor-grab items-center justify-center overflow-hidden active:cursor-grabbing"
            onMouseDown={(event) => {
              dragRef.current = { x: event.clientX, y: event.clientY, ox: offset.x, oy: offset.y };
            }}
            onMouseMove={(event) => {
              if (!dragRef.current) return;
              setOffset({
                x: dragRef.current.ox + (event.clientX - dragRef.current.x),
                y: dragRef.current.oy + (event.clientY - dragRef.current.y),
              });
            }}
            onMouseUp={() => {
              dragRef.current = null;
            }}
            onMouseLeave={() => {
              dragRef.current = null;
            }}
          >
            <div
              style={{
                transform: `translate(${offset.x}px, ${offset.y}px) scale(${scale})`,
                transformOrigin: "center center",
              }}
              // biome-ignore lint/security/noDangerouslySetInnerHtml: SVG is generated by Pele inside the viewer.
              dangerouslySetInnerHTML={{ __html: lightbox.svg }}
            />
          </div>
        </div>
      </div>
    </>
  );
}
