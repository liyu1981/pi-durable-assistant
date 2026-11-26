import { InfoIcon, SettingsIcon } from "@hugeicons/core-free-icons";
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
    <div className="mx-auto flex max-w-xl flex-col items-center gap-5 py-4 text-center text-sm">
      <img
        src="./assets/logo-512.png"
        alt="Pi Durable Assistant"
        className="w-72 max-w-full rounded-3xl border border-border object-cover shadow-sm md:w-80"
      />
      <div className="flex flex-col gap-1">
        <p className="font-heading text-2xl font-bold tracking-tight">Pi Durable Assistant</p>
        <p className="text-xs text-muted-foreground">Version 0.1.0</p>
      </div>
      <p className="leading-relaxed text-muted-foreground">
        A self-hosted, durable personal assistant. Conversations, tools, reminders, and scheduled
        work are committed to SQLite and resume after a process restart.
      </p>
      <dl className="grid w-full grid-cols-[auto_1fr] gap-x-6 gap-y-2 rounded-2xl border border-border bg-muted/40 p-4 text-left">
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

/** Global system settings: full screen on mobile, wide side-tabbed dialog on desktop. */
export function SystemSettingsDialog() {
  const [open, setOpen] = useAtom(systemSettingsOpenAtom);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="top-0! left-0! flex! h-dvh! w-screen! max-w-none! translate-x-0! translate-y-0! flex-col! gap-0! overflow-hidden rounded-none! p-0 md:top-1/2! md:left-1/2! md:h-[min(88vh,900px)]! md:w-[min(1100px,94vw)]! md:-translate-x-1/2! md:-translate-y-1/2! md:rounded-4xl!">
        <DialogHeader className="flex shrink-0 flex-row items-center gap-3 border-b border-border px-4 py-3 md:px-6 md:py-4">
          <span className="grid size-8 place-items-center rounded-xl bg-primary text-primary-foreground">
            <HugeiconsIcon icon={SettingsIcon} strokeWidth={2} />
          </span>
          <DialogTitle className="text-lg font-semibold">Setting</DialogTitle>
          <DialogDescription className="sr-only">
            Manage the model provider and read about this installation.
          </DialogDescription>
        </DialogHeader>
        <Tabs
          defaultValue="models"
          className="flex min-h-0 flex-1 flex-col! gap-4 p-4 md:flex-row! md:gap-6 md:p-6"
        >
          <TabsList className="flex! h-auto! w-full! shrink-0 items-center overflow-x-auto rounded-2xl! p-1 md:h-fit! md:w-52! md:flex-col! md:items-stretch md:self-start md:overflow-visible">
            <TabsTrigger
              value="models"
              className="md:h-auto! md:w-full md:flex-none! md:justify-start"
            >
              <HugeiconsIcon icon={SettingsIcon} strokeWidth={2} />
              Models
            </TabsTrigger>
            <TabsTrigger
              value="about"
              className="md:h-auto! md:w-full md:flex-none! md:justify-start"
            >
              <HugeiconsIcon icon={InfoIcon} strokeWidth={2} />
              About
            </TabsTrigger>
          </TabsList>
          <TabsContent value="models" className="min-h-0 min-w-0 overflow-y-auto">
            <SettingsPanel />
          </TabsContent>
          <TabsContent value="about" className="min-h-0 min-w-0 overflow-y-auto">
            <AboutPanel />
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
