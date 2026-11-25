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
    return { error: "暂不支持 SVG，请上传图片或文本类文件" };
  }
  const isGif = mime === "image/gif" || ext === "gif";
  const isImage = IMAGE_MIMES.has(mime) || IMAGE_EXTENSIONS.has(ext);
  const isText = mime.startsWith("text/") || TEXT_EXTENSIONS.has(ext);
  if (!isImage && !isText) return { error: `「${name}」不是支持的图片或文本文件` };

  if (isImage) {
    if (isGif) {
      if (file.size > MAX_IMAGE) return { error: `GIF「${name}」超过 5MB` };
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
        image.onerror = () => reject(new Error(`图片「${name}」加载失败`));
        image.src = objectUrl;
      });
    } catch (error) {
      URL.revokeObjectURL(objectUrl);
      return { error: error instanceof Error ? error.message : "图片加载失败" };
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
        return { error: "浏览器不支持图片压缩" };
      }
      context.fillStyle = "#ffffff";
      context.fillRect(0, 0, width, height);
      context.drawImage(image, 0, 0, width, height);
      URL.revokeObjectURL(objectUrl);
      const dataUrl = canvas.toDataURL("image/jpeg", 0.85);
      const base64 = dataUrl.split(",")[1] ?? "";
      const size = Math.round((base64.length * 3) / 4);
      if (size > MAX_IMAGE) return { error: `图片压缩后仍超过 5MB` };
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
      return { error: `图片「${name}」超过 5MB` };
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

  if (file.size > MAX_TEXT) return { error: `文本文件「${name}」超过 200KB` };
  const buffer = await file.arrayBuffer();
  let text: string;
  try {
    text = new TextDecoder("utf-8", { fatal: true }).decode(buffer);
  } catch {
    return { error: `「${name}」不是 UTF-8 文本` };
  }
  if (text.includes("\u0000")) return { error: `「${name}」不是文本文件` };
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
        toast("一次最多上传 5 个附件", "warning");
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
            toast("附件总大小不能超过 10MB", "warning");
            continue;
          }
          if (result.attachment) {
            total += result.attachment.size;
            count += 1;
            setAttachments((previous) => [...previous, result.attachment as PendingAttachment]);
          }
        } catch (error) {
          toast(error instanceof Error ? error.message : "处理文件失败", "warning");
        }
      }
    },
    [attachments, setAttachments, toast],
  );

  useEffect(() => {
    const column = document.querySelector<HTMLElement>(".chat-column");
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
      toast(`发送失败：${error instanceof Error ? error.message : String(error)}`, "error");
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
      toast(`停止失败：${error instanceof Error ? error.message : String(error)}`, "warning");
    }
  };

  const removeAttachment = (index: number) => {
    const attachment = attachments[index];
    if (attachment?.previewUrl?.startsWith("blob:")) URL.revokeObjectURL(attachment.previewUrl);
    setAttachments((previous) => previous.filter((_, at) => at !== index));
  };

  const canSend = (draft.trim().length > 0 || attachments.length > 0) && !sending;

  return (
    <footer className="composer">
      {dragging ? <div className="drop-overlay">拖放文件到此处上传（最多 5 个）</div> : null}
      <div className="composer-controls">
        <div className="row-between">
          {busy ? (
            <label className="label">
              <input
                type="checkbox"
                checked={steer}
                onChange={(event) => setSteer(event.target.checked)}
              />
              插话（steer）
            </label>
          ) : (
            <span className="label">Enter 发送 · Shift+Enter 换行</span>
          )}
          {busy ? (
            <button type="button" className="btn btn-danger" onClick={() => void stop()}>
              ⏹ 停止
            </button>
          ) : null}
        </div>
        {(view?.inbox ?? 0) > 0 ? <span className="label">排队中：{view?.inbox} 条</span> : null}
      </div>

      {attachments.length > 0 ? (
        <div className="pending-attachments">
          {attachments.map((attachment, index) => (
            <span className="pending-chip" key={keyFor(`pending-${attachment.name}`)}>
              {attachment.previewUrl ? (
                <img src={attachment.previewUrl} alt="" />
              ) : (
                <span aria-hidden="true">📄</span>
              )}
              <span title={attachment.name}>{attachment.name}</span>
              <button
                type="button"
                className="btn btn-quiet"
                aria-label={`移除 ${attachment.name}`}
                onClick={() => removeAttachment(index)}
              >
                ×
              </button>
            </span>
          ))}
        </div>
      ) : null}

      <div className="composer-box">
        <input
          ref={fileRef}
          className="hidden-input"
          type="file"
          multiple
          accept={ACCEPT}
          tabIndex={-1}
          onChange={(event) => {
            if (event.target.files?.length) void addFiles(event.target.files);
            event.target.value = "";
          }}
        />
        <button
          type="button"
          className="btn btn-quiet"
          aria-label="添加附件"
          onClick={() => fileRef.current?.click()}
        >
          📎
        </button>
        <textarea
          ref={textareaRef}
          aria-label="输入消息"
          placeholder="输入消息…（Enter 发送，Shift+Enter 换行）"
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
        />
        <button
          type="button"
          className="btn btn-primary"
          disabled={!canSend}
          onClick={() => void submit()}
        >
          {sending ? "发送中…" : "发送"}
        </button>
      </div>
    </footer>
  );
}
