import { useAtom } from "jotai/react";
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
  { key: "system", label: "System" },
  { key: "tasks", label: "Tasks" },
  { key: "memory", label: "Memory" },
  { key: "skills", label: "Skills" },
  { key: "tools", label: "Tools" },
  { key: "knowledge", label: "Knowledge" },
  { key: "artifacts", label: "Files" },
];

export function Drawer() {
  const [drawer, setDrawer] = useAtom(drawerAtom);

  return (
    <Sheet
      open={drawer.open}
      onOpenChange={(open) => setDrawer((previous) => ({ ...previous, open }))}
    >
      <SheetContent side="right" className="w-full gap-0 p-0 sm:max-w-xl">
        <SheetHeader className="border-b border-border">
          <SheetTitle>Management</SheetTitle>
          <SheetDescription>
            Review schedules, reminders, memory, skills, tools, and the knowledge base.
          </SheetDescription>
        </SheetHeader>
        <Tabs
          value={drawer.panel}
          onValueChange={(value) =>
            setDrawer({ open: true, panel: (value ?? "status") as DrawerPanel })
          }
          className="min-h-0 flex-1 gap-0"
        >
          <div className="shrink-0 overflow-x-auto border-b border-border px-4 py-3">
            <TabsList className="w-max min-w-full">
              {TABS.map((tab) => (
                <TabsTrigger key={tab.key} value={tab.key}>
                  {tab.label}
                </TabsTrigger>
              ))}
            </TabsList>
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
      </SheetContent>
    </Sheet>
  );
}
