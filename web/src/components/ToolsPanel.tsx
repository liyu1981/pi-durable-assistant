import { ToolboxIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useCallback, useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { get } from "../api/client";
import type { ToolInfo, ToolsData } from "../api/types";
import { useToast } from "../lib/useToast";
import { LoadingState, StatusMessage } from "./ui";

/** Tools whose schema has argument properties are inspectable. */
function parameterNames(tool: ToolInfo): string[] {
  const properties = tool.parameters?.properties;
  if (properties && typeof properties === "object" && !Array.isArray(properties)) {
    return Object.keys(properties as Record<string, unknown>);
  }
  return [];
}

export function ToolsPanel() {
  const toast = useToast();
  const [tools, setTools] = useState<ToolInfo[] | null>(null);
  const [feedback, setFeedback] = useState("");

  const refresh = useCallback(async () => {
    setFeedback("Working...");
    try {
      const data = await get<ToolsData>("/api/tools");
      setTools(data.tools);
      setFeedback("");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setFeedback(`Failed to load tools: ${message}`);
      toast(`Failed to load tools: ${message}`, "warning");
    }
  }, [toast]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return (
    <div className="flex flex-col gap-6">
      <StatusMessage>{feedback}</StatusMessage>

      <Card size="sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <HugeiconsIcon icon={ToolboxIcon} strokeWidth={2} />
            Agent tools
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <p className="text-sm text-muted-foreground">
            Every tool below is available to the assistant in new turns. Open a tool to inspect its
            arguments.
          </p>
          {tools === null ? (
            <LoadingState label="Loading tools..." />
          ) : tools.length === 0 ? (
            <p className="text-sm text-muted-foreground">No tools registered.</p>
          ) : (
            <ul className="flex flex-col">
              {tools.map((tool) => {
                const parameters = parameterNames(tool);
                return (
                  <li
                    key={`${tool.extension}:${tool.name}`}
                    className="border-b border-border py-3"
                  >
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className="font-mono text-sm font-semibold">{tool.name}</p>
                        <p className="mt-1 text-sm text-muted-foreground">{tool.description}</p>
                        {parameters.length > 0 ? (
                          <details className="mt-1">
                            <summary className="cursor-pointer text-xs text-muted-foreground">
                              Arguments: {parameters.join(", ")}
                            </summary>
                            <pre className="mt-2 whitespace-pre-wrap break-words border-l-2 border-primary/60 pl-2 font-mono text-xs">
                              {JSON.stringify(tool.parameters, null, 2)}
                            </pre>
                          </details>
                        ) : (
                          <p className="mt-1 text-xs text-muted-foreground">No arguments</p>
                        )}
                      </div>
                      <Badge variant="outline">{tool.extension}</Badge>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
          <Button variant="outline" size="sm" onClick={() => void refresh()}>
            Refresh
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
