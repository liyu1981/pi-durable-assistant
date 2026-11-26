import { Cancel01Icon, CheckmarkCircle02Icon, MagicWand01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { Textarea } from "@/components/ui/textarea";
import { post } from "../api/client";
import { newRequestId } from "../lib/format";
import { STATUS_LABEL, statusTone, usePersonal } from "../lib/usePersonal";
import { Field, StatusMessage, toneVariant } from "./ui";

export function SkillsPanel() {
  const { personal, feedback, run } = usePersonal();
  const [skillName, setSkillName] = useState("");
  const [skillDescription, setSkillDescription] = useState("");
  const [skillBody, setSkillBody] = useState("");

  return (
    <div className="flex flex-col gap-6">
      <StatusMessage>{feedback}</StatusMessage>

      <Card size="sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <HugeiconsIcon icon={MagicWand01Icon} strokeWidth={2} />
            Skills
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <p className="text-sm text-muted-foreground">
            Only the name is placed in the prompt; the assistant reads the full body when needed.
          </p>
          <form
            className="flex flex-col gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              void run(
                "Save skill",
                async () => {
                  await post("/api/personal/skills", {
                    name: skillName,
                    description: skillDescription,
                    body: skillBody,
                    requestId: newRequestId(),
                  });
                },
                () => {
                  setSkillName("");
                  setSkillDescription("");
                  setSkillBody("");
                },
              );
            }}
          >
            <Field label="Name">
              <Input
                name="name"
                maxLength={40}
                required
                value={skillName}
                placeholder="e.g. Release checklist"
                onChange={(event) => setSkillName(event.target.value)}
              />
            </Field>
            <Field label="When to use it">
              <Input
                name="description"
                maxLength={180}
                required
                value={skillDescription}
                onChange={(event) => setSkillDescription(event.target.value)}
              />
            </Field>
            <Field label="Steps">
              <Textarea
                name="body"
                maxLength={4000}
                required
                value={skillBody}
                onChange={(event) => setSkillBody(event.target.value)}
              />
            </Field>
            <Button type="submit">
              <HugeiconsIcon icon={MagicWand01Icon} strokeWidth={2} />
              Save skill
            </Button>
          </form>
          <Separator />
          {(personal?.skills ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">No skills yet.</p>
          ) : (
            <ul className="flex flex-col">
              {(personal?.skills ?? []).map((item) => (
                <li key={item.id} className="border-b border-border py-3">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <p className="text-sm">
                        {item.name}: {item.description}
                      </p>
                      <details className="mt-1">
                        <summary className="cursor-pointer text-xs text-muted-foreground">
                          View steps
                        </summary>
                        <pre className="mt-2 whitespace-pre-wrap break-words border-l-2 border-primary/60 pl-2 font-mono text-xs">
                          {item.body}
                        </pre>
                      </details>
                    </div>
                    <Badge variant={toneVariant(statusTone(item.status))}>
                      {STATUS_LABEL[item.status] ?? item.status}
                    </Badge>
                  </div>
                  <div className="mt-2 flex flex-wrap gap-2">
                    {item.status === "pending" ? (
                      <Button
                        variant="outline"
                        size="xs"
                        onClick={() =>
                          void run("Approve", () =>
                            post(`/api/personal/skills/${item.id}/approve`, {}),
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
                            post(`/api/personal/skills/${item.id}/reject`, {}),
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
