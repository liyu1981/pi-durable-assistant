import { atom } from "jotai";
import { atomWithStorage } from "jotai/utils";
import type {
  Conversation,
  KnowledgeHit,
  KnowledgePage,
  Management,
  PersonalData,
  ServerInfo,
  SseStatus,
  ViewPayload,
} from "../api/types";

export type ConversationFilter = "active" | "archived";

export const conversationsAtom = atom<Conversation[]>([]);
export const currentConversationIdAtom = atom<number | null>(null);
export const conversationFilterAtom = atom<ConversationFilter>("active");
export const pendingRemoveAtom = atom<number | null>(null);

/** Latest `view` event payload from the SSE stream. */
export const viewAtom = atom<ViewPayload | null>(null);
export const busyAtom = atom((get) => {
  const view = get(viewAtom);
  return Boolean(view?.busy || (view?.live && Object.keys(view.live).length > 0));
});
export const sseStatusAtom = atom<SseStatus>("connecting");
export const serverInfoAtom = atom<ServerInfo | null>(null);

export interface PendingAttachment {
  kind: "image" | "text";
  name: string;
  mimeType: string;
  size: number;
  data: string;
  previewUrl?: string;
}

export const attachmentsAtom = atom<PendingAttachment[]>([]);
export const draftAtom = atom("");
export const sendingAtom = atom(false);
export const steerAtom = atom(false);

/** Task-center data and UI state. */
export const managementAtom = atom<Management | null>(null);
export const personalAtom = atom<PersonalData | null>(null);
export const knowledgeResultsAtom = atom<KnowledgeHit[]>([]);
export const knowledgePageAtom = atom<KnowledgePage | null>(null);
export const leftPanelCollapsedAtom = atomWithStorage("pi.leftPanelCollapsed", false);
export const rightPanelCollapsedAtom = atomWithStorage("pi.rightPanelCollapsed", false);

export const systemSettingsOpenAtom = atom(false);

export interface ArtifactRef {
  artifactId: string;
  path: string;
}

/** The artifact currently shown in the full-screen viewer. */
export const artifactViewerAtom = atom<ArtifactRef | null>(null);
export type ArtifactViewerTheme = "light" | "dark";
const initialArtifactViewerTheme: ArtifactViewerTheme =
  typeof document !== "undefined" && document.documentElement.classList.contains("dark")
    ? "dark"
    : "light";
export const artifactViewerThemeAtom = atomWithStorage<ArtifactViewerTheme>(
  "pi.artifactViewerTheme",
  initialArtifactViewerTheme,
);

/** Panels available in the right-hand management drawer. */
export type DrawerPanel =
  | "tasks"
  | "memory"
  | "skills"
  | "tools"
  | "knowledge"
  | "status"
  | "artifacts";

export const drawerAtom = atom<{
  open: boolean;
  panel: DrawerPanel;
}>({
  open: false,
  panel: "tasks",
});

export interface Toast {
  id: number;
  message: string;
  kind: "info" | "success" | "warning" | "error";
}

export const toastsAtom = atom<Toast[]>([]);
