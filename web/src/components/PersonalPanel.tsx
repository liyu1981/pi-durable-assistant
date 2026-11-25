import { useAtom } from "jotai/react";
import { useCallback, useEffect, useState } from "react";
import { get, post } from "../api/client";
import type { PersonalData } from "../api/types";
import { personalAtom } from "../atoms";
import { newRequestId } from "../lib/format";
import { useToast } from "../lib/useToast";
import { Badge, Button, Field, SectionTitle, StatusMessage } from "./ui";

const STATUS_LABEL: Record<string, string> = {
  pending: "Pending",
  approved: "Active",
  rejected: "Rejected",
};

function statusTone(status: string): "warning" | "success" | "danger" {
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
    <>
      <section>
        <SectionTitle>Memory &amp; skills</SectionTitle>
        <p className="text-sm text-primary/70">
          {knowledge?.enabled
            ? `Knowledge base is read-only: ${(knowledge.allow ?? []).join(", ") || "all paths"}`
            : `Knowledge base unavailable: ${knowledge?.reason ?? "not configured"}`}
        </p>
        <div className="mt-2">
          <StatusMessage>{feedback}</StatusMessage>
        </div>
      </section>

      <section>
        <SectionTitle>Long-term memory</SectionTitle>
        <p className="text-sm text-primary/70">
          Memories you write take effect immediately. Assistant proposals need approval.
        </p>
        <form
          className="mt-3 flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            void run(
              "Save memory",
              async () => {
                await post("/api/personal/memory", { text: memoryText, requestId: newRequestId() });
              },
              () => setMemoryText(""),
            );
          }}
        >
          <Field label="Fact to remember">
            <input
              className="field"
              name="text"
              maxLength={280}
              required
              value={memoryText}
              placeholder="e.g. Reply with short bullet lists"
              onChange={(event) => setMemoryText(event.target.value)}
            />
          </Field>
          <Button type="submit" variant="primary">
            Remember
          </Button>
        </form>
        {(personal?.memory ?? []).length === 0 ? (
          <p className="mt-3 text-sm text-primary/70">No cross-session memories yet.</p>
        ) : (
          <ul className="mt-3 flex flex-col">
            {(personal?.memory ?? []).map((item) => (
              <li key={item.id} className="border-b border-primary/15 py-3">
                <div className="flex items-start justify-between gap-2">
                  <span className="text-sm" title={item.id}>
                    {item.text}
                  </span>
                  <Badge tone={statusTone(item.status)}>
                    {STATUS_LABEL[item.status] ?? item.status}
                  </Badge>
                </div>
                <p className="label mt-1 text-primary/70">
                  {item.source === "user" ? "Written by you" : "Proposed by the assistant"}
                </p>
                <div className="mt-2 flex flex-wrap gap-2">
                  {item.status === "pending" ? (
                    <Button
                      variant="quiet"
                      onClick={() =>
                        void run("Approve", () =>
                          post(`/api/personal/memory/${item.id}/approve`, {}),
                        )
                      }
                    >
                      Approve
                    </Button>
                  ) : null}
                  {item.status === "pending" || item.status === "approved" ? (
                    <Button
                      variant="danger"
                      onClick={() =>
                        void run("Reject", () => post(`/api/personal/memory/${item.id}/reject`, {}))
                      }
                    >
                      {item.status === "approved" ? "Revoke" : "Reject"}
                    </Button>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <SectionTitle>Skills</SectionTitle>
        <p className="text-sm text-primary/70">
          Only the name is placed in the prompt; the assistant reads the full body when needed.
        </p>
        <form
          className="mt-3 flex flex-col gap-3"
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
            <input
              className="field"
              name="name"
              maxLength={40}
              required
              value={skillName}
              onChange={(event) => setSkillName(event.target.value)}
            />
          </Field>
          <Field label="When to use it">
            <input
              className="field"
              name="description"
              maxLength={180}
              required
              value={skillDescription}
              onChange={(event) => setSkillDescription(event.target.value)}
            />
          </Field>
          <Field label="Steps">
            <textarea
              className="field min-h-20 resize-y"
              name="body"
              maxLength={4000}
              required
              value={skillBody}
              onChange={(event) => setSkillBody(event.target.value)}
            />
          </Field>
          <Button type="submit" variant="primary">
            Save skill
          </Button>
        </form>
        {(personal?.skills ?? []).length === 0 ? (
          <p className="mt-3 text-sm text-primary/70">No skills yet.</p>
        ) : (
          <ul className="mt-3 flex flex-col">
            {(personal?.skills ?? []).map((item) => (
              <li key={item.id} className="border-b border-primary/15 py-3">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <p className="text-sm">
                      {item.name}: {item.description}
                    </p>
                    <details className="mt-1">
                      <summary className="label cursor-pointer text-secondary">View steps</summary>
                      <pre className="mt-2 whitespace-pre-wrap break-words border-l-2 border-secondary pl-2 text-xs">
                        {item.body}
                      </pre>
                    </details>
                  </div>
                  <Badge tone={statusTone(item.status)}>
                    {STATUS_LABEL[item.status] ?? item.status}
                  </Badge>
                </div>
                <div className="mt-2 flex flex-wrap gap-2">
                  {item.status === "pending" ? (
                    <Button
                      variant="quiet"
                      onClick={() =>
                        void run("Approve", () =>
                          post(`/api/personal/skills/${item.id}/approve`, {}),
                        )
                      }
                    >
                      Approve
                    </Button>
                  ) : null}
                  {item.status === "pending" || item.status === "approved" ? (
                    <Button
                      variant="danger"
                      onClick={() =>
                        void run("Reject", () => post(`/api/personal/skills/${item.id}/reject`, {}))
                      }
                    >
                      {item.status === "approved" ? "Revoke" : "Reject"}
                    </Button>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>
    </>
  );
}
