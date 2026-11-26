import { AtomIcon, InfoIcon, SettingsIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useAtom } from "jotai/react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { systemSettingsOpenAtom } from "../atoms";
import { SettingsPanel } from "./SettingsPanel";

function AboutPanel() {
  return (
    <div className="flex flex-col gap-4 text-sm">
      <div className="flex items-center gap-3">
        <span className="grid size-10 place-items-center rounded-2xl bg-primary text-primary-foreground">
          <HugeiconsIcon icon={AtomIcon} strokeWidth={2} />
        </span>
        <div>
          <p className="font-heading text-lg font-bold tracking-tight">Pi Durable Assistant</p>
          <p className="text-xs text-muted-foreground">Version 0.1.0</p>
        </div>
      </div>
      <p className="leading-relaxed text-muted-foreground">
        A self-hosted, durable personal assistant. Conversations, tools, reminders, and scheduled
        work are committed to SQLite and resume after a process restart.
      </p>
      <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 rounded-2xl border border-border bg-muted/40 p-4">
        <dt className="text-muted-foreground">Backend</dt>
        <dd>Go 1.26 with PiG Durable 1.7</dd>
        <dt className="text-muted-foreground">Frontend</dt>
        <dd>React 19, TypeScript 7, Vite 8, Tailwind 4, shadcn/ui Luma</dd>
        <dt className="text-muted-foreground">State</dt>
        <dd>Jotai with localStorage persistence for panel layout</dd>
        <dt className="text-muted-foreground">License</dt>
        <dd>MIT</dd>
      </dl>
      <p className="text-xs text-muted-foreground">
        Not affiliated with Pi, PiG, or the shadcn/ui project.
      </p>
    </div>
  );
}

/** Global system settings, opened from the header with a single link. */
export function SystemSettingsDialog() {
  const [open, setOpen] = useAtom(systemSettingsOpenAtom);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="max-w-3xl gap-0 p-0">
        <DialogHeader className="border-b border-border px-6 py-4">
          <DialogTitle className="flex items-center gap-2">
            <HugeiconsIcon icon={SettingsIcon} strokeWidth={2} />
            System settings
          </DialogTitle>
          <DialogDescription>
            Manage the model provider and read about this installation.
          </DialogDescription>
        </DialogHeader>
        <Tabs orientation="vertical" defaultValue="models" className="min-h-96 p-4">
          <TabsList
            variant="line"
            className="w-44 shrink-0 items-start rounded-2xl border-r border-border pr-2"
          >
            <TabsTrigger value="models">
              <HugeiconsIcon icon={SettingsIcon} strokeWidth={2} />
              Models
            </TabsTrigger>
            <TabsTrigger value="about">
              <HugeiconsIcon icon={InfoIcon} strokeWidth={2} />
              About
            </TabsTrigger>
          </TabsList>
          <TabsContent value="models" className="min-w-0 overflow-y-auto pl-6 pr-1">
            <SettingsPanel />
          </TabsContent>
          <TabsContent value="about" className="min-w-0 overflow-y-auto pl-6 pr-1">
            <AboutPanel />
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
