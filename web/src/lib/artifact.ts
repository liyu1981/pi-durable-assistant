import { artifactRawUrl } from "../api/client";
import type { ArtifactRef } from "../atoms";

/** Parse an artifact://<id>/<path> reference from Markdown. */
export function parseArtifactRef(value: string): ArtifactRef | null {
  const match = /^artifact:\/\/([A-Za-z0-9_-]+)\/(\S+)$/.exec(value.trim());
  if (!match) return null;
  let filePath: string;
  try {
    filePath = decodeURIComponent(match[2]);
  } catch {
    filePath = match[2];
  }
  if (!filePath || filePath.startsWith("/") || filePath.split("/").includes("..")) return null;
  return { artifactId: match[1], path: filePath };
}

export function artifactFileName(ref: ArtifactRef): string {
  const parts = ref.path.split("/");
  return parts[parts.length - 1] || "artifact";
}

export function artifactKind(name: string): "image" | "text" | "pdf" | "document" | "other" {
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  if (["png", "jpg", "jpeg", "gif", "webp", "svg", "avif"].includes(ext)) return "image";
  if (ext === "pdf") return "pdf";
  if (
    [
      "md",
      "markdown",
      "txt",
      "csv",
      "json",
      "yaml",
      "yml",
      "log",
      "js",
      "mjs",
      "ts",
      "tsx",
      "jsx",
      "go",
      "py",
      "css",
      "html",
      "xml",
    ].includes(ext)
  ) {
    return "text";
  }
  if (["doc", "docx", "ppt", "pptx", "xls", "xlsx", "odt", "rtf", "epub"].includes(ext)) {
    return "document";
  }
  return "other";
}

export function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / (1024 * 1024)).toFixed(1)} MB`;
}

const objectUrls = new Map<string, string>();

export function cachedArtifactUrl(ref: ArtifactRef): string | null {
  return objectUrls.get(`${ref.artifactId}/${ref.path}`) ?? null;
}

/** Fetch artifact bytes once per reference and expose a viewer-friendly URL. */
export async function loadArtifactUrl(ref: ArtifactRef): Promise<string> {
  const key = `${ref.artifactId}/${ref.path}`;
  const cached = objectUrls.get(key);
  if (cached) return cached;
  const response = await fetch(artifactRawUrl(ref.artifactId, ref.path));
  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new Error(body?.error ?? `Artifact request failed (${response.status})`);
  }
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  objectUrls.set(key, url);
  return url;
}
