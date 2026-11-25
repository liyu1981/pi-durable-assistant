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
import { drawerAtom } from "../atoms";
import { KnowledgePanel } from "./KnowledgePanel";
import { PersonalPanel } from "./PersonalPanel";
import { StatusPanel } from "./StatusPanel";
import { TaskCenter } from "./TaskCenter";

const TABS = [
  { key: "status", label: "Status" },
  { key: "tasks", label: "Tasks" },
  { key: "personal", label: "Memory & skills" },
  { key: "knowledge", label: "Knowledge" },
] as const;

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
          <SheetDescription>Review schedules, memory, and the knowledge base.</SheetDescription>
        </SheetHeader>
        <Tabs
          value={drawer.panel}
          onValueChange={(value) =>
            setDrawer({
              open: true,
              panel: (value ?? "status") as "status" | "tasks" | "personal" | "knowledge",
            })
          }
          className="min-h-0 flex-1 gap-0"
        >
          <div className="shrink-0 border-b border-border px-4 py-3">
            <TabsList className="w-full">
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
              <TabsContent value="tasks" className="mt-0">
                <TaskCenter />
              </TabsContent>
              <TabsContent value="personal" className="mt-0">
                <PersonalPanel />
              </TabsContent>
              <TabsContent value="knowledge" className="mt-0">
                <KnowledgePanel />
              </TabsContent>
            </div>
          </ScrollArea>
        </Tabs>
      </SheetContent>
    </Sheet>
  );
}
