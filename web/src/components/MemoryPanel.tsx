import {
  BrainIcon,
  Cancel01Icon,
  CheckmarkCircle02Icon,
  SparklesIcon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { post } from "../api/client";
import { newRequestId } from "../lib/format";
import { STATUS_LABEL, statusTone, usePersonal } from "../lib/usePersonal";
import { Field, StatusMessage, toneVariant } from "./ui";

export function MemoryPanel() {
  const { personal, feedback, run } = usePersonal();
  const [memoryText, setMemoryText] = useState("");

  return (
    <div className="flex flex-col gap-6">
      <StatusMessage>{feedback}</StatusMessage>

      <Card size="sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <HugeiconsIcon icon={BrainIcon} strokeWidth={2} />
            Long-term memory
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <p className="text-sm text-muted-foreground">
            Memories you write take effect immediately. Assistant proposals need approval.
          </p>
          <form
            className="flex flex-col gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              void run(
                "Save memory",
                async () => {
                  await post("/api/personal/memory", {
                    text: memoryText,
                    requestId: newRequestId(),
                  });
                },
                () => setMemoryText(""),
              );
            }}
          >
            <Field label="Fact to remember">
              <Input
                name="text"
                maxLength={280}
                required
                value={memoryText}
                placeholder="e.g. Reply with short bullet lists"
                onChange={(event) => setMemoryText(event.target.value)}
              />
            </Field>
            <Button type="submit">
              <HugeiconsIcon icon={SparklesIcon} strokeWidth={2} />
              Remember
            </Button>
          </form>
          <Separator />
          {(personal?.memory ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">No cross-session memories yet.</p>
          ) : (
            <ul className="flex flex-col">
              {(personal?.memory ?? []).map((item) => (
                <li key={item.id} className="border-b border-border py-3">
                  <div className="flex items-start justify-between gap-2">
                    <span className="text-sm" title={item.id}>
                      {item.text}
                    </span>
                    <Badge variant={toneVariant(statusTone(item.status))}>
                      {STATUS_LABEL[item.status] ?? item.status}
                    </Badge>
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {item.source === "user" ? "Written by you" : "Proposed by the assistant"}
                  </p>
                  <div className="mt-2 flex flex-wrap gap-2">
                    {item.status === "pending" ? (
                      <Button
                        variant="outline"
                        size="xs"
                        onClick={() =>
                          void run("Approve", () =>
                            post(`/api/personal/memory/${item.id}/approve`, {}),
                          )
                        }
                      >
                        <HugeiconsIcon icon={CheckmarkCircle02Icon} strokeWidth={2} />
                        Approve
                      </Button>
                    ) : null}
                    {item.status === "pending" || item.status === "approved" ? (
                      <Button
                        variant="destructive"
                        size="xs"
                        onClick={() =>
                          void run("Reject", () =>
                            post(`/api/personal/memory/${item.id}/reject`, {}),
                          )
                        }
                      >
                        <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
                        {item.status === "approved" ? "Revoke" : "Reject"}
                      </Button>
                    ) : null}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
