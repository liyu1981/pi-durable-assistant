import {
  Calendar01Icon,
  DownloadIcon,
  PlayIcon,
  RefreshIcon,
  StopIcon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useAtom, useAtomValue } from "jotai/react";
import { useCallback, useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Textarea } from "@/components/ui/textarea";
import { exportUrl, get, post } from "../api/client";
import type { Management } from "../api/types";
import { currentConversationIdAtom, managementAtom } from "../atoms";
import { formatTime, newRequestId } from "../lib/format";
import { useToast } from "../lib/useToast";
import { Field, StatusMessage, type Tone, toneVariant } from "./ui";

const REPEAT_PRESETS = ["0", "1", "2", "5", "10", "15", "30", "60", "120", "1440"];

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
  const [repeatMode, setRepeatMode] = useState<"preset" | "custom">("preset");
  const [everyPreset, setEveryPreset] = useState("0");
  const [everyCustom, setEveryCustom] = useState("5");
  const [backgroundText, setBackgroundText] = useState("");
  const every = repeatMode === "preset" ? everyPreset : everyCustom;

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
    <div className="flex flex-col gap-6">
      <Card size="sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <HugeiconsIcon icon={Calendar01Icon} strokeWidth={2} />
            Task center
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <p className="text-sm text-muted-foreground">
            {disabled
              ? "Select a conversation to manage schedules and background jobs. Memory and skills do not require one."
              : `Conversation #${currentId}`}
          </p>
          <StatusMessage>{feedback}</StatusMessage>
          <Separator />
          <div className="flex flex-wrap gap-2">
            {currentId === null ? (
              <Button variant="outline" size="sm" disabled>
                <HugeiconsIcon icon={DownloadIcon} strokeWidth={2} />
                Export Markdown
              </Button>
            ) : (
              <a
                className={buttonVariants({ variant: "outline", size: "sm" })}
                href={exportUrl(currentId)}
                download
              >
                <HugeiconsIcon icon={DownloadIcon} strokeWidth={2} />
                Export Markdown
              </a>
            )}
            <Button variant="ghost" size="sm" onClick={() => void refresh()}>
              <HugeiconsIcon icon={RefreshIcon} strokeWidth={2} />
              Refresh
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>New schedule</CardTitle>
        </CardHeader>
        <CardContent>
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
              <Input
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
                <Input
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
              <Field label="Repeat every">
                <Select
                  value={repeatMode === "preset" ? everyPreset : "custom"}
                  onValueChange={(value) => {
                    if (value === "custom") {
                      setRepeatMode("custom");
                      return;
                    }
                    setRepeatMode("preset");
                    setEveryPreset(value ?? "0");
                  }}
                >
                  <SelectTrigger disabled={disabled} className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {REPEAT_PRESETS.map((value) => (
                      <SelectItem key={value} value={value}>
                        {value === "0" ? "Once" : `${value} min`}
                      </SelectItem>
                    ))}
                    <SelectItem value="custom">Custom</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
            </div>
            {repeatMode === "custom" ? (
              <Field label="Custom interval (minutes)">
                <Input
                  name="everyMinutes"
                  type="number"
                  min="0"
                  max="525600"
                  step="any"
                  required
                  disabled={disabled}
                  value={everyCustom}
                  onChange={(event) => setEveryCustom(event.target.value)}
                />
              </Field>
            ) : null}
            <Button type="submit" disabled={disabled}>
              Create schedule
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Background subagent</CardTitle>
        </CardHeader>
        <CardContent>
          <form
            className="flex flex-col gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              void run("Create background job", async () => {
                await post(`${base}/background`, {
                  text: backgroundText,
                  requestId: newRequestId(),
                });
                setBackgroundText("");
              });
            }}
          >
            <Field label="Instructions">
              <Textarea
                name="text"
                maxLength={4000}
                required
                disabled={disabled}
                value={backgroundText}
                placeholder="Describe the work to complete independently"
                onChange={(event) => setBackgroundText(event.target.value)}
              />
            </Field>
            <Button type="submit" disabled={disabled}>
              <HugeiconsIcon icon={PlayIcon} strokeWidth={2} />
              Run in background
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Schedules</CardTitle>
        </CardHeader>
        <CardContent>
          {(management?.schedules ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">No schedules yet.</p>
          ) : (
            <ul className="flex flex-col">
              {(management?.schedules ?? []).map((item) => (
                <li key={item.id} className="border-b border-border py-3">
                  <div className="flex items-start justify-between gap-2">
                    <span className="text-sm" title={item.id}>
                      {item.text}
                    </span>
                    <Badge variant={toneVariant(statusTone(item.status))}>{item.status}</Badge>
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {item.runs} runs
                    {item.nextAt ? ` - next ${formatTime(item.nextAt)}` : ""}
                  </p>
                  <div className="mt-2 flex flex-wrap gap-2">
                    {item.status === "active" ? (
                      <Button
                        variant="outline"
                        size="xs"
                        onClick={() =>
                          void run("Pause", () => post(`${base}/schedules/${item.id}/pause`))
                        }
                      >
                        Pause
                      </Button>
                    ) : null}
                    {item.status === "paused" ? (
                      <Button
                        variant="outline"
                        size="xs"
                        onClick={() =>
                          void run("Resume", () => post(`${base}/schedules/${item.id}/resume`))
                        }
                      >
                        Resume
                      </Button>
                    ) : null}
                    {item.status === "active" || item.status === "paused" ? (
                      <Button
                        variant="destructive"
                        size="xs"
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
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Reminders</CardTitle>
        </CardHeader>
        <CardContent>
          {(management?.reminders ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">No reminders yet.</p>
          ) : (
            <ul className="flex flex-col">
              {(management?.reminders ?? []).map((item) => (
                <li
                  key={item.id}
                  className="flex flex-wrap items-center justify-between gap-2 border-b border-border py-2"
                >
                  <span className="text-sm">
                    {item.text} - {formatTime(item.at)}
                  </span>
                  <span className="flex items-center gap-2">
                    <Badge variant={toneVariant(statusTone(item.status))}>{item.status}</Badge>
                    {item.status === "scheduled" ? (
                      <Button
                        variant="destructive"
                        size="xs"
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
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Background jobs</CardTitle>
        </CardHeader>
        <CardContent>
          {(management?.background ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">No background jobs yet.</p>
          ) : (
            <ul className="flex flex-col">
              {(management?.background ?? []).map((item) => (
                <li key={item.id} className="border-b border-border py-3">
                  <div className="flex items-start justify-between gap-2">
                    <span className="text-sm" title={item.id}>
                      {item.text}
                    </span>
                    <Badge variant={toneVariant(statusTone(item.status))}>{item.status}</Badge>
                  </div>
                  {item.result ? (
                    <p className="mt-1 text-sm text-muted-foreground">
                      {item.result.slice(0, 200)}
                    </p>
                  ) : null}
                  <div className="mt-2 flex flex-wrap gap-2">
                    {item.conversationId !== null ? (
                      <Button
                        variant="link"
                        size="xs"
                        className="h-auto px-0"
                        render={<a href={`#${item.conversationId}`} />}
                      >
                        View conversation
                      </Button>
                    ) : null}
                    {item.status === "running" ? (
                      <Button
                        variant="destructive"
                        size="xs"
                        onClick={() =>
                          void run("Stop", () => post(`${base}/background/${item.id}/stop`))
                        }
                      >
                        <HugeiconsIcon icon={StopIcon} strokeWidth={2} />
                        Stop
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
