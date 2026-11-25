import { useAtom, useAtomValue } from "jotai/react";
import { useCallback, useEffect, useRef, useState } from "react";
import { post, sendMessage } from "../api/client";
import {
  attachmentsAtom,
  busyAtom,
  currentConversationIdAtom,
  draftAtom,
  type PendingAttachment,
  sendingAtom,
  steerAtom,
  viewAtom,
} from "../atoms";
import { makeKeyFactory, newRequestId } from "../lib/format";
import { useToast } from "../lib/useToast";
import { Button, CheckboxField } from "./ui";

const TEXT_EXTENSIONS = new Set([
  "txt",
  "md",
  "markdown",
  "csv",
  "tsv",
  "json",
  "jsonl",
  "yaml",
  "yml",
  "xml",
  "html",
  "htm",
  "css",
  "js",
  "mjs",
  "cjs",
  "ts",
  "tsx",
  "jsx",
  "py",
  "go",
  "rs",
  "java",
  "kt",
  "swift",
  "c",
  "h",
  "cpp",
  "hpp",
  "cs",
  "rb",
  "php",
  "sh",
  "zsh",
  "bash",
  "sql",
  "toml",
  "ini",
  "conf",
  "log",
  "srt",
  "vtt",
]);
const IMAGE_MIMES = new Set(["image/png", "image/jpeg", "image/webp", "image/gif"]);
const IMAGE_EXTENSIONS = new Set(["png", "jpg", "jpeg", "webp", "gif"]);
const MAX_FILES = 5;
const MAX_IMAGE = 5 * 1024 * 1024;
const MAX_TEXT = 200 * 1024;
const MAX_TOTAL = 10 * 1024 * 1024;

const ACCEPT = [
  "image/png",
  "image/jpeg",
  "image/webp",
  "image/gif",
  "text/*",
  ...Array.from(TEXT_EXTENSIONS, (ext) => `.${ext}`),
].join(",");

function cleanName(raw: string): string {
  const segment = raw.split(/[\\/]/).pop() ?? "";
  const safe = Array.from(segment)
    .filter((character) => {
      const code = character.codePointAt(0) ?? 0;
      return code > 0x1f && code !== 0x7f && !'"<>'.includes(character);
    })
    .join("");
  return safe.trim().slice(0, 100) || "attachment";
}

function extension(name: string): string {
  return name.includes(".") ? (name.split(".").pop() ?? "").toLowerCase() : "";
}

function toBase64(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.length; i += 8192) {
    binary += String.fromCharCode(...bytes.subarray(i, Math.min(i + 8192, bytes.length)));
  }
  return btoa(binary);
}

async function processFile(
  file: File,
): Promise<{ attachment?: PendingAttachment; error?: string }> {
  const name = cleanName(file.name);
  const ext = extension(name);
  const mime = file.type.toLowerCase().split(";")[0].trim();
  if (mime === "image/svg+xml" || ext === "svg") {
    return { error: "SVG files are not supported. Upload an image or a text file." };
  }
  const isGif = mime === "image/gif" || ext === "gif";
  const isImage = IMAGE_MIMES.has(mime) || IMAGE_EXTENSIONS.has(ext);
  const isText = mime.startsWith("text/") || TEXT_EXTENSIONS.has(ext);
  if (!isImage && !isText) return { error: `"${name}" is not a supported image or text file` };

  if (isImage) {
    if (isGif) {
      if (file.size > MAX_IMAGE) return { error: `GIF "${name}" exceeds 5 MB` };
      return {
        attachment: {
          kind: "image",
          name,
          mimeType: "image/gif",
          size: file.size,
          data: toBase64(await file.arrayBuffer()),
          previewUrl: URL.createObjectURL(file),
        },
      };
    }
    const objectUrl = URL.createObjectURL(file);
    let image: HTMLImageElement;
    try {
      image = new Image();
      await new Promise<void>((resolve, reject) => {
        image.onload = () => resolve();
        image.onerror = () => reject(new Error(`Image "${name}" could not be loaded`));
        image.src = objectUrl;
      });
    } catch (error) {
      URL.revokeObjectURL(objectUrl);
      return { error: error instanceof Error ? error.message : "Image could not be loaded" };
    }
    const maxDimension = Math.max(image.naturalWidth, image.naturalHeight);
    if (maxDimension > 2048 || file.size > 1.5 * 1024 * 1024) {
      const scale = maxDimension > 2048 ? 2048 / maxDimension : 1;
      const width = Math.round(image.naturalWidth * scale);
      const height = Math.round(image.naturalHeight * scale);
      const canvas = document.createElement("canvas");
      canvas.width = width;
      canvas.height = height;
      const context = canvas.getContext("2d");
      if (!context) {
        URL.revokeObjectURL(objectUrl);
        return { error: "This browser does not support image compression" };
      }
      const surface = getComputedStyle(document.documentElement)
        .getPropertyValue("--color-surface")
        .trim();
      context.fillStyle = surface || "white";
      context.fillRect(0, 0, width, height);
      context.drawImage(image, 0, 0, width, height);
      URL.revokeObjectURL(objectUrl);
      const dataUrl = canvas.toDataURL("image/jpeg", 0.85);
      const base64 = dataUrl.split(",")[1] ?? "";
      const size = Math.round((base64.length * 3) / 4);
      if (size > MAX_IMAGE) return { error: "Image is still over 5 MB after compression" };
      return {
        attachment: {
          kind: "image",
          name: `${name.replace(/\.[^.]+$/, "")}.jpg`,
          mimeType: "image/jpeg",
          size,
          data: base64,
          previewUrl: dataUrl,
        },
      };
    }
    if (file.size > MAX_IMAGE) {
      URL.revokeObjectURL(objectUrl);
      return { error: `Image "${name}" exceeds 5 MB` };
    }
    return {
      attachment: {
        kind: "image",
        name,
        mimeType:
          mime || (ext === "png" ? "image/png" : ext === "webp" ? "image/webp" : "image/jpeg"),
        size: file.size,
        data: toBase64(await file.arrayBuffer()),
        previewUrl: objectUrl,
      },
    };
  }

  if (file.size > MAX_TEXT) return { error: `Text file "${name}" exceeds 200 KB` };
  const buffer = await file.arrayBuffer();
  let text: string;
  try {
    text = new TextDecoder("utf-8", { fatal: true }).decode(buffer);
  } catch {
    return { error: `"${name}" is not UTF-8 text` };
  }
  if (text.includes("\u0000")) return { error: `"${name}" is not a text file` };
  return {
    attachment: {
      kind: "text",
      name,
      mimeType: mime || "text/plain",
      size: file.size,
      data: toBase64(buffer),
    },
  };
}

interface ComposerProps {
  onEnsureConversation: () => Promise<number | null>;
  onSent: () => void;
}

export function Composer({ onEnsureConversation, onSent }: ComposerProps) {
  const [draft, setDraft] = useAtom(draftAtom);
  const [attachments, setAttachments] = useAtom(attachmentsAtom);
  const [sending, setSending] = useAtom(sendingAtom);
  const [steer, setSteer] = useAtom(steerAtom);
  const currentId = useAtomValue(currentConversationIdAtom);
  const view = useAtomValue(viewAtom);
  const busy = useAtomValue(busyAtom);
  const toast = useToast();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const composing = useRef(false);
  const dragDepth = useRef(0);
  const keyFor = makeKeyFactory();
  const [dragging, setDragging] = useState(false);

  const addFiles = useCallback(
    async (files: FileList | File[]) => {
      const incoming = Array.from(files);
      if (incoming.length === 0) return;
      if (attachments.length + incoming.length > MAX_FILES) {
        toast("At most 5 attachments per message", "warning");
        return;
      }
      let total = attachments.reduce((sum, item) => sum + item.size, 0);
      let count = attachments.length;
      for (const file of incoming) {
        if (count >= MAX_FILES) break;
        try {
          const result = await processFile(file);
          if (result.error) {
            toast(result.error, "warning");
            continue;
          }
          if (result.attachment && total + result.attachment.size > MAX_TOTAL) {
            if (result.attachment.previewUrl?.startsWith("blob:")) {
              URL.revokeObjectURL(result.attachment.previewUrl);
            }
            toast("Total attachment size must stay under 10 MB", "warning");
            continue;
          }
          if (result.attachment) {
            total += result.attachment.size;
            count += 1;
            setAttachments((previous) => [...previous, result.attachment as PendingAttachment]);
          }
        } catch (error) {
          toast(error instanceof Error ? error.message : "Failed to process the file", "warning");
        }
      }
    },
    [attachments, setAttachments, toast],
  );

  useEffect(() => {
    const column = document.querySelector<HTMLElement>("[data-chat-column]");
    if (!column) return;
    const hasFiles = (event: DragEvent) =>
      Array.from(event.dataTransfer?.types ?? []).includes("Files");
    const enter = (event: DragEvent) => {
      if (!hasFiles(event)) return;
      event.preventDefault();
      dragDepth.current += 1;
      setDragging(true);
    };
    const over = (event: DragEvent) => {
      if (!hasFiles(event)) return;
      event.preventDefault();
      if (event.dataTransfer) event.dataTransfer.dropEffect = "copy";
    };
    const leave = (event: DragEvent) => {
      event.preventDefault();
      dragDepth.current -= 1;
      if (dragDepth.current <= 0) {
        dragDepth.current = 0;
        setDragging(false);
      }
    };
    const drop = (event: DragEvent) => {
      event.preventDefault();
      dragDepth.current = 0;
      setDragging(false);
      if (event.dataTransfer?.files.length) void addFiles(event.dataTransfer.files);
    };
    column.addEventListener("dragenter", enter);
    column.addEventListener("dragover", over);
    column.addEventListener("dragleave", leave);
    column.addEventListener("drop", drop);
    return () => {
      column.removeEventListener("dragenter", enter);
      column.removeEventListener("dragover", over);
      column.removeEventListener("dragleave", leave);
      column.removeEventListener("drop", drop);
    };
  }, [addFiles]);

  useEffect(() => {
    const element = textareaRef.current;
    if (!element) return;
    element.style.height = "auto";
    const minimum = draft.split("\n").length * 24;
    element.style.height = `${Math.min(Math.max(element.scrollHeight, minimum), 140)}px`;
  }, [draft]);

  const submit = async () => {
    if (sending) return;
    const text = draft.trim();
    if (!text && attachments.length === 0) return;
    const id = currentId ?? (await onEnsureConversation());
    if (id === null) return;
    setSending(true);
    try {
      await sendMessage(id, {
        text,
        requestId: newRequestId(),
        mode: steer ? "steer" : "followUp",
        attachments: attachments.map((item) => ({
          name: item.name,
          mimeType: item.mimeType,
          data: item.data,
        })),
      });
      setDraft("");
      for (const attachment of attachments) {
        if (attachment.previewUrl?.startsWith("blob:")) URL.revokeObjectURL(attachment.previewUrl);
      }
      setAttachments([]);
      onSent();
    } catch (error) {
      toast(`Failed to send: ${error instanceof Error ? error.message : String(error)}`, "error");
    } finally {
      setSending(false);
      textareaRef.current?.focus();
    }
  };

  const stop = async () => {
    if (currentId === null) return;
    try {
      await post(`/api/conversations/${currentId}/abort`);
    } catch (error) {
      toast(`Failed to stop: ${error instanceof Error ? error.message : String(error)}`, "warning");
    }
  };

  const removeAttachment = (index: number) => {
    const attachment = attachments[index];
    if (attachment?.previewUrl?.startsWith("blob:")) URL.revokeObjectURL(attachment.previewUrl);
    setAttachments((previous) => previous.filter((_, at) => at !== index));
  };

  const canSend = (draft.trim().length > 0 || attachments.length > 0) && !sending;

  return (
    <footer className="border-t border-primary bg-surface px-4 py-3">
      {dragging ? (
        <div
          className="pointer-events-none absolute inset-0 z-30 grid place-items-center bg-surface/95 text-lg font-bold text-secondary"
          aria-hidden="true"
        >
          <span className="rounded-md border-2 border-dashed border-secondary px-8 py-6">
            Drop files here to attach (up to 5)
          </span>
        </div>
      ) : null}

      <div className="mx-auto flex w-full max-w-3xl flex-col gap-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          {busy ? (
            <CheckboxField label="Steer" checked={steer} onChange={setSteer} />
          ) : (
            <span className="label text-primary/70">
              Enter to send · Shift+Enter for a new line
            </span>
          )}
          <div className="flex items-center gap-3">
            {(view?.inbox ?? 0) > 0 ? (
              <span className="label text-primary/70">Queued: {view?.inbox}</span>
            ) : null}
            {busy ? (
              <Button variant="danger" onClick={() => void stop()}>
                Stop
              </Button>
            ) : null}
          </div>
        </div>

        {attachments.length > 0 ? (
          <ul className="flex flex-wrap gap-2" aria-label="Attachments">
            {attachments.map((attachment, index) => {
              const key = keyFor(attachment.name);
              return (
                <li
                  key={key}
                  className="flex items-center gap-2 rounded-sm border border-primary/20 px-2 py-1"
                >
                  {attachment.previewUrl ? (
                    <img
                      src={attachment.previewUrl}
                      alt=""
                      className="h-9 w-9 shrink-0 rounded-sm border border-primary/20 object-cover"
                    />
                  ) : (
                    <span className="label text-secondary">File</span>
                  )}
                  <span className="max-w-40 truncate text-xs" title={attachment.name}>
                    {attachment.name}
                  </span>
                  <button
                    type="button"
                    className="btn btn-quiet min-h-6 px-1"
                    aria-label={`Remove ${attachment.name}`}
                    onClick={() => removeAttachment(index)}
                  >
                    ×
                  </button>
                </li>
              );
            })}
          </ul>
        ) : null}

        <div className="flex items-end gap-2 rounded-md border border-primary bg-surface p-2 focus-within:border-secondary">
          <input
            ref={fileRef}
            className="absolute h-px w-px opacity-0"
            type="file"
            multiple
            accept={ACCEPT}
            tabIndex={-1}
            aria-hidden="true"
            onChange={(event) => {
              if (event.target.files?.length) void addFiles(event.target.files);
              event.target.value = "";
            }}
          />
          <Button
            variant="quiet"
            aria-label="Add attachment"
            onClick={() => fileRef.current?.click()}
          >
            Attach
          </Button>
          <textarea
            ref={textareaRef}
            aria-label="Message"
            placeholder="Write a message... (Enter to send, Shift+Enter for a new line)"
            rows={1}
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            onCompositionStart={() => {
              composing.current = true;
            }}
            onCompositionEnd={() => {
              composing.current = false;
            }}
            onPaste={(event) => {
              if (event.clipboardData.files.length > 0) {
                event.preventDefault();
                void addFiles(event.clipboardData.files);
              }
            }}
            onKeyDown={(event) => {
              if (
                event.key === "Enter" &&
                !event.shiftKey &&
                !composing.current &&
                !event.nativeEvent.isComposing
              ) {
                event.preventDefault();
                void submit();
              }
            }}
            className="max-h-35 min-h-11 flex-1 resize-none border-0 bg-transparent leading-6 outline-none"
          />
          <Button variant="primary" disabled={!canSend} onClick={() => void submit()}>
            {sending ? "Sending..." : "Send"}
          </Button>
        </div>
      </div>
    </footer>
  );
}
