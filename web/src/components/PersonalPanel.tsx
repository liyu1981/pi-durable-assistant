import {
  BrainIcon,
  Cancel01Icon,
  CheckmarkCircle02Icon,
  SparklesIcon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useAtom } from "jotai/react";
import { useCallback, useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { Textarea } from "@/components/ui/textarea";
import { get, post } from "../api/client";
import type { PersonalData } from "../api/types";
import { personalAtom } from "../atoms";
import { newRequestId } from "../lib/format";
import { useToast } from "../lib/useToast";
import { Field, StatusMessage, type Tone, toneVariant } from "./ui";

const STATUS_LABEL: Record<string, string> = {
  pending: "Pending",
  approved: "Active",
  rejected: "Rejected",
};

function statusTone(status: string): Tone {
  if (status === "pending") return "warning";
  if (status === "approved") return "success";
  return "danger";
}

export function PersonalPanel() {
  const [personal, setPersonal] = useAtom(personalAtom);
  const toast = useToast();
  const [feedback, setFeedback] = useState("");
  const [memoryText, setMemoryText] = useState("");
  const [skillName, setSkillName] = useState("");
  const [skillDescription, setSkillDescription] = useState("");
  const [skillBody, setSkillBody] = useState("");

  const refresh = useCallback(async () => {
    try {
      setPersonal(await get<PersonalData>("/api/personal"));
    } catch (error) {
      toast(
        `Failed to load memory and skills: ${error instanceof Error ? error.message : String(error)}`,
        "warning",
      );
    }
  }, [setPersonal, toast]);

  useEffect(() => {
    let active = true;
    const tick = () => {
      if (active) void refresh();
    };
    tick();
    const timer = setInterval(tick, 5000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [refresh]);

  const run = async (label: string, action: () => Promise<unknown>, reset?: () => void) => {
    setFeedback("Working...");
    try {
      await action();
      reset?.();
      setFeedback(`${label} complete`);
      await refresh();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setFeedback(`${label} failed: ${message}`);
      toast(`${label} failed: ${message}`, "warning");
    }
  };

  const knowledge = personal?.knowledge;

  return (
    <div className="flex flex-col gap-6">
      <Card size="sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <HugeiconsIcon icon={BrainIcon} strokeWidth={2} />
            Memory &amp; skills
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          <p className="text-sm text-muted-foreground">
            {knowledge?.enabled
              ? `Knowledge base is read-only: ${(knowledge.allow ?? []).join(", ") || "all paths"}`
              : `Knowledge base unavailable: ${knowledge?.reason ?? "not configured"}`}
          </p>
          <StatusMessage>{feedback}</StatusMessage>
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Long-term memory</CardTitle>
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

      <Card size="sm">
        <CardHeader>
          <CardTitle>Skills</CardTitle>
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
            <Button type="submit">Save skill</Button>
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
