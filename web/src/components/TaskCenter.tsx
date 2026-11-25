import { useAtom, useAtomValue } from "jotai/react";
import { useCallback, useEffect, useState } from "react";
import { exportUrl, get, post } from "../api/client";
import type { Management } from "../api/types";
import { currentConversationIdAtom, managementAtom } from "../atoms";
import { formatTime, newRequestId } from "../lib/format";
import { useToast } from "../lib/useToast";
import { Badge, Button, Field, SectionTitle, StatusMessage } from "./ui";

type Tone = "neutral" | "accent" | "success" | "warning" | "danger";

function statusTone(status: string): Tone {
  if (status === "active") return "success";
  if (status === "paused" || status === "scheduled") return "warning";
  if (status === "failed" || status === "error" || status === "cancelled") return "danger";
  if (status === "running") return "accent";
  return "neutral";
}

export function TaskCenter() {
  const currentId = useAtomValue(currentConversationIdAtom);
  const [management, setManagement] = useAtom(managementAtom);
  const toast = useToast();
  const [feedback, setFeedback] = useState("");
  const [scheduleText, setScheduleText] = useState("");
  const [minutes, setMinutes] = useState("2");
  const [every, setEvery] = useState("0");
  const [backgroundText, setBackgroundText] = useState("");

  const refresh = useCallback(async () => {
    if (currentId === null) {
      setManagement(null);
      return;
    }
    try {
      setManagement(await get<Management>(`/api/conversations/${currentId}/management`));
    } catch (error) {
      toast(
        `Failed to load tasks: ${error instanceof Error ? error.message : String(error)}`,
        "warning",
      );
    }
  }, [currentId, setManagement, toast]);

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

  const run = async (label: string, action: () => Promise<unknown>) => {
    setFeedback("Working...");
    try {
      await action();
      setFeedback(`${label} complete`);
      await refresh();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setFeedback(`${label} failed: ${message}`);
      toast(`${label} failed: ${message}`, "warning");
    }
  };

  const base = currentId === null ? "" : `/api/conversations/${currentId}`;
  const disabled = currentId === null;

  return (
    <>
      <section>
        <SectionTitle>Task center</SectionTitle>
        <p className="text-sm text-primary/70">
          {disabled
            ? "Select a conversation to manage schedules and background jobs. Memory and skills do not require one."
            : `Conversation #${currentId}`}
        </p>
        <div className="mt-2">
          <StatusMessage>{feedback}</StatusMessage>
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          {currentId === null ? (
            <Button disabled>Export Markdown</Button>
          ) : (
            <a className="btn" href={exportUrl(currentId)} download>
              Export Markdown
            </a>
          )}
          <Button variant="quiet" onClick={() => void refresh()}>
            Refresh
          </Button>
        </div>
      </section>

      <section>
        <SectionTitle>New schedule</SectionTitle>
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            void run("Create schedule", async () => {
              await post(`${base}/schedules`, {
                text: scheduleText,
                minutesFromNow: Number(minutes),
                everyMinutes: Number(every),
                requestId: newRequestId(),
              });
              setScheduleText("");
            });
          }}
        >
          <Field label="Task text">
            <input
              className="field"
              name="text"
              maxLength={1000}
              required
              disabled={disabled}
              value={scheduleText}
              placeholder="e.g. Check for project updates"
              onChange={(event) => setScheduleText(event.target.value)}
            />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Start in (minutes)">
              <input
                className="field"
                name="minutesFromNow"
                type="number"
                min="0.25"
                max="525600"
                step="any"
                required
                disabled={disabled}
                value={minutes}
                onChange={(event) => setMinutes(event.target.value)}
              />
            </Field>
            <Field label="Repeat every (0 = once)">
              <input
                className="field"
                name="everyMinutes"
                type="number"
                min="0"
                max="525600"
                step="any"
                required
                disabled={disabled}
                value={every}
                onChange={(event) => setEvery(event.target.value)}
              />
            </Field>
          </div>
          <Button type="submit" variant="primary" disabled={disabled}>
            Create schedule
          </Button>
        </form>
      </section>

      <section>
        <SectionTitle>Background subagent</SectionTitle>
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            void run("Create background job", async () => {
              await post(`${base}/background`, { text: backgroundText, requestId: newRequestId() });
              setBackgroundText("");
            });
          }}
        >
          <Field label="Instructions">
            <textarea
              className="field min-h-20 resize-y"
              name="text"
              maxLength={4000}
              required
              disabled={disabled}
              value={backgroundText}
              placeholder="Describe the work to complete independently"
              onChange={(event) => setBackgroundText(event.target.value)}
            />
          </Field>
          <Button type="submit" variant="primary" disabled={disabled}>
            Run in background
          </Button>
        </form>
      </section>

      <section>
        <SectionTitle>Schedules</SectionTitle>
        {(management?.schedules ?? []).length === 0 ? (
          <p className="text-sm text-primary/70">No schedules yet.</p>
        ) : (
          <ul className="flex flex-col">
            {(management?.schedules ?? []).map((item) => (
              <li key={item.id} className="border-b border-primary/15 py-3">
                <div className="flex items-start justify-between gap-2">
                  <span className="text-sm" title={item.id}>
                    {item.text}
                  </span>
                  <Badge tone={statusTone(item.status)}>{item.status}</Badge>
                </div>
                <p className="label mt-1 text-primary/70">
                  {item.runs} runs
                  {item.nextAt ? ` - next ${formatTime(item.nextAt)}` : ""}
                </p>
                <div className="mt-2 flex flex-wrap gap-2">
                  {item.status === "active" ? (
                    <Button
                      variant="quiet"
                      onClick={() =>
                        void run("Pause", () => post(`${base}/schedules/${item.id}/pause`))
                      }
                    >
                      Pause
                    </Button>
                  ) : null}
                  {item.status === "paused" ? (
                    <Button
                      variant="quiet"
                      onClick={() =>
                        void run("Resume", () => post(`${base}/schedules/${item.id}/resume`))
                      }
                    >
                      Resume
                    </Button>
                  ) : null}
                  {item.status === "active" || item.status === "paused" ? (
                    <Button
                      variant="danger"
                      onClick={() =>
                        void run("Cancel", () => post(`${base}/schedules/${item.id}/cancel`))
                      }
                    >
                      Cancel
                    </Button>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <SectionTitle>Reminders</SectionTitle>
        {(management?.reminders ?? []).length === 0 ? (
          <p className="text-sm text-primary/70">No reminders yet.</p>
        ) : (
          <ul className="flex flex-col">
            {(management?.reminders ?? []).map((item) => (
              <li
                key={item.id}
                className="flex flex-wrap items-center justify-between gap-2 border-b border-primary/15 py-2"
              >
                <span className="text-sm">
                  {item.text} - {formatTime(item.at)}
                </span>
                <span className="flex items-center gap-2">
                  <Badge tone={statusTone(item.status)}>{item.status}</Badge>
                  {item.status === "scheduled" ? (
                    <Button
                      variant="danger"
                      onClick={() =>
                        void run("Cancel reminder", () =>
                          post(`${base}/reminders/${item.id}/cancel`),
                        )
                      }
                    >
                      Cancel
                    </Button>
                  ) : null}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <SectionTitle>Background jobs</SectionTitle>
        {(management?.background ?? []).length === 0 ? (
          <p className="text-sm text-primary/70">No background jobs yet.</p>
        ) : (
          <ul className="flex flex-col">
            {(management?.background ?? []).map((item) => (
              <li key={item.id} className="border-b border-primary/15 py-3">
                <div className="flex items-start justify-between gap-2">
                  <span className="text-sm" title={item.id}>
                    {item.text}
                  </span>
                  <Badge tone={statusTone(item.status)}>{item.status}</Badge>
                </div>
                {item.result ? (
                  <p className="mt-1 text-sm text-primary/70">{item.result.slice(0, 200)}</p>
                ) : null}
                <div className="mt-2 flex flex-wrap gap-2">
                  {item.conversationId !== null ? (
                    <a className="btn" href={`#${item.conversationId}`}>
                      View conversation
                    </a>
                  ) : null}
                  {item.status === "running" ? (
                    <Button
                      variant="danger"
                      onClick={() =>
                        void run("Stop", () => post(`${base}/background/${item.id}/stop`))
                      }
                    >
                      Stop
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
