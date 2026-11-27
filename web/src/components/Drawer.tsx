import { ArrowLeft01Icon, ArrowRight01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useAtom } from "jotai/react";
import { useRef } from "react";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { type DrawerPanel, drawerAtom } from "../atoms";
import { ArtifactsPanel } from "./ArtifactsPanel";
import { KnowledgePanel } from "./KnowledgePanel";
import { MemoryPanel } from "./MemoryPanel";
import { SkillsPanel } from "./SkillsPanel";
import { StatusPanel } from "./StatusPanel";
import { SystemPromptPanel } from "./SystemPromptPanel";
import { TaskCenter } from "./TaskCenter";
import { ToolsPanel } from "./ToolsPanel";

const TABS: { key: DrawerPanel; label: string }[] = [
  { key: "status", label: "Status" },
  { key: "artifacts", label: "Files" },
  { key: "tasks", label: "Tasks" },
  { key: "memory", label: "Memory" },
  { key: "knowledge", label: "Knowledge" },
  { key: "skills", label: "Skills" },
  { key: "tools", label: "Tools" },
  { key: "system", label: "System" },
];

/** Shared management tabs, used by the desktop right panel and mobile drawer. */
export function ManagementTabs({
  value,
  onValueChange,
  className,
}: {
  value: DrawerPanel;
  onValueChange: (panel: DrawerPanel) => void;
  className?: string;
}) {
  const listRef = useRef<HTMLDivElement | null>(null);
  const scroll = (direction: "left" | "right") => {
    listRef.current?.scrollBy({ left: direction === "left" ? -200 : 200, behavior: "smooth" });
  };
  return (
    <Tabs
      value={value}
      onValueChange={(next) => onValueChange((next ?? "status") as DrawerPanel)}
      className={className ?? "min-h-0 flex-1 gap-0"}
    >
      <div className="flex shrink-0 items-center gap-1 border-b border-border px-2 py-3">
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label="Scroll tabs left"
          onClick={() => scroll("left")}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
        </Button>
        <div
          ref={listRef}
          className="min-w-0 flex-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
        >
          <TabsList className="w-max min-w-full">
            {TABS.map((tab) => (
              <TabsTrigger key={tab.key} value={tab.key}>
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label="Scroll tabs right"
          onClick={() => scroll("right")}
        >
          <HugeiconsIcon icon={ArrowRight01Icon} strokeWidth={2} />
        </Button>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="flex flex-col gap-8 px-4 py-6">
          <TabsContent value="status" className="mt-0">
            <StatusPanel className="w-full border-l-0 bg-transparent px-0" />
          </TabsContent>
          <TabsContent value="system" className="mt-0">
            <SystemPromptPanel />
          </TabsContent>
          <TabsContent value="tasks" className="mt-0">
            <TaskCenter />
          </TabsContent>
          <TabsContent value="memory" className="mt-0">
            <MemoryPanel />
          </TabsContent>
          <TabsContent value="skills" className="mt-0">
            <SkillsPanel />
          </TabsContent>
          <TabsContent value="tools" className="mt-0">
            <ToolsPanel />
          </TabsContent>
          <TabsContent value="knowledge" className="mt-0">
            <KnowledgePanel />
          </TabsContent>
          <TabsContent value="artifacts" className="mt-0">
            <ArtifactsPanel />
          </TabsContent>
        </div>
      </ScrollArea>
    </Tabs>
  );
}

/** Mobile-only sheet; Desktop uses the always-visible right panel instead. */
export function Drawer() {
  const [drawer, setDrawer] = useAtom(drawerAtom);

  return (
    <Sheet
      open={drawer.open}
      onOpenChange={(open) => setDrawer((previous) => ({ ...previous, open }))}
    >
      <SheetContent side="right" className="w-full gap-0 p-0 sm:max-w-xl lg:hidden">
        <SheetHeader className="border-b border-border">
          <SheetTitle>Management</SheetTitle>
          <SheetDescription>
            Review schedules, reminders, memory, skills, tools, files, and the system prompt.
          </SheetDescription>
        </SheetHeader>
        <ManagementTabs
          value={drawer.panel}
          onValueChange={(panel) => setDrawer({ open: true, panel })}
        />
      </SheetContent>
    </Sheet>
  );
}
