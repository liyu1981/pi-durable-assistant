import { CheckmarkCircle02Icon, SettingsIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useSetAtom } from "jotai/react";
import { useCallback, useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  activateLLMProfile,
  createLLMProfile,
  deleteLLMProfile,
  fetchLLMModels,
  getLLMProfiles,
  testLLMSettings,
  updateLLMProfile,
} from "../api/client";
import type { LLMSettingsInput } from "../api/types";
import { activeLLMProfileIdAtom, llmProfilesAtom, serverInfoAtom } from "../atoms";
import { useToast } from "../lib/useToast";
import { Field, LoadingState, StatusMessage } from "./ui";

const APIS = [
  { value: "openai-completions", label: "OpenAI completions" },
  { value: "openai-responses", label: "OpenAI responses" },
  { value: "anthropic-messages", label: "Anthropic messages" },
] as const;

interface FormState {
  name: string;
  api: string;
  baseURL: string;
  model: string;
  contextWindow: string;
  maxTokens: string;
  apiKey: string;
}

function toForm(profile: {
  name: string;
  api: string;
  baseURL: string;
  model: string;
  contextWindow: number;
  maxTokens: number;
}): FormState {
  return {
    name: profile.name,
    api: profile.api,
    baseURL: profile.baseURL,
    model: profile.model,
    contextWindow: String(profile.contextWindow),
    maxTokens: String(profile.maxTokens),
    apiKey: "",
  };
}

function validateBaseURL(form: FormState): string | null {
  if (!form.baseURL.trim()) return "Base URL is required.";
  if (!/^https?:\/\//.test(form.baseURL.trim())) {
    return "Base URL must start with http:// or https://.";
  }
  return null;
}

function validate(form: FormState): string | null {
  const baseError = validateBaseURL(form);
  if (baseError) return baseError;
  if (!form.name.trim()) return "Profile name is required.";
  if (!form.model.trim()) return "Model is required.";
  const windowSize = Number(form.contextWindow);
  const maxTokens = Number(form.maxTokens);
  if (!Number.isInteger(windowSize) || windowSize < 1000) {
    return "Context window must be an integer of at least 1000.";
  }
  if (!Number.isInteger(maxTokens) || maxTokens < 1 || maxTokens > windowSize) {
    return "Max tokens must be between 1 and the context window.";
  }
  return null;
}

/** Named LLM provider profiles, persisted in SQLite and applied without restart. */
export function SettingsPanel() {
  const setServerInfo = useSetAtom(serverInfoAtom);
  const setProfiles = useSetAtom(llmProfilesAtom);
  const setActiveId = useSetAtom(activeLLMProfileIdAtom);
  const toast = useToast();
  const [profiles, setLocalProfiles] = useState<
    Awaited<ReturnType<typeof getLLMProfiles>>["profiles"]
  >([]);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [form, setForm] = useState<FormState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [models, setModels] = useState<string[]>([]);
  const [busy, setBusy] = useState<"test" | "save" | "models" | null>(null);

  const syncProfiles = useCallback(
    async (data: Awaited<ReturnType<typeof getLLMProfiles>>) => {
      setLocalProfiles(data.profiles);
      setProfiles(data.profiles);
      setActiveId(data.activeId);
      const selected = data.profiles.find((item) => item.id === data.activeId) ?? data.profiles[0];
      setSelectedId(selected?.id ?? null);
      setForm(selected ? toForm(selected) : null);
    },
    [setActiveId, setProfiles],
  );

  const refresh = useCallback(async () => {
    try {
      await syncProfiles(await getLLMProfiles());
      setError(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, [syncProfiles]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const selectProfile = (id: string) => {
    const profile = profiles.find((item) => item.id === Number(id));
    if (!profile) return;
    setSelectedId(profile.id);
    setForm(toForm(profile));
    setModels([]);
    setNotice(null);
  };

  const update = useCallback((key: keyof FormState, value: string) => {
    setForm((previous) => (previous ? { ...previous, [key]: value } : previous));
    setNotice(null);
    if (key === "api" || key === "baseURL" || key === "apiKey") setModels([]);
  }, []);

  const input = useCallback((): LLMSettingsInput | null => {
    if (!form) return null;
    const value: LLMSettingsInput = {
      api: form.api,
      baseURL: form.baseURL.trim(),
      model: form.model.trim(),
      contextWindow: Number(form.contextWindow),
      maxTokens: Number(form.maxTokens),
    };
    if (form.apiKey.trim()) value.apiKey = form.apiKey.trim();
    return value;
  }, [form]);

  const fetchModels = async () => {
    if (!form) {
      setError("Settings are still loading.");
      return;
    }
    const baseError = validateBaseURL(form);
    if (baseError) {
      setError(baseError);
      return;
    }
    setError(null);
    setNotice(null);
    setBusy("models");
    try {
      const candidate: { api: string; baseURL: string; apiKey?: string } = {
        api: form.api,
        baseURL: form.baseURL.trim(),
      };
      if (form.apiKey.trim()) candidate.apiKey = form.apiKey.trim();
      const result = await fetchLLMModels(candidate);
      setModels(result.models);
      setNotice(
        result.models.length > 0
          ? `Found ${result.models.length} model(s) in ${result.latencyMs} ms. Pick one below.`
          : `Connected in ${result.latencyMs} ms, but the provider returned no models.`,
      );
    } catch (cause) {
      setModels([]);
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(null);
    }
  };

  const test = async () => {
    const value = input();
    const invalid = value ? validate(form as FormState) : "Settings are still loading.";
    if (invalid || !value) {
      setError(invalid);
      return;
    }
    setError(null);
    setNotice(null);
    setBusy("test");
    try {
      const result = await testLLMSettings(value);
      setModels(result.models);
      setNotice(`Connected in ${result.latencyMs} ms.`);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(null);
    }
  };

  const save = async () => {
    const value = input();
    const invalid = value ? validate(form as FormState) : "Settings are still loading.";
    if (invalid || !value || selectedId === null) {
      setError(invalid ?? "No profile selected.");
      return;
    }
    setError(null);
    setNotice(null);
    setBusy("save");
    try {
      const result = await updateLLMProfile(selectedId, {
        name: (form as FormState).name,
        ...value,
      });
      setForm((previous) => (previous ? { ...previous, apiKey: "" } : previous));
      if (result.profile.active) {
        setServerInfo((current) =>
          current ? { ...current, model: result.profile.model, api: result.profile.api } : current,
        );
      }
      await refresh();
      toast(`${result.profile.name} saved.`, "success");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(null);
    }
  };

  const create = async () => {
    const value = input();
    const invalid = value ? validate(form as FormState) : "Settings are still loading.";
    if (invalid || !value) {
      setError(invalid);
      return;
    }
    setError(null);
    setBusy("save");
    try {
      const result = await createLLMProfile({ name: (form as FormState).name, ...value });
      await refresh();
      setSelectedId(result.profile.id);
      setForm(toForm(result.profile));
      toast(`${result.profile.name} created.`, "success");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(null);
    }
  };

  const remove = async () => {
    if (selectedId === null) return;
    setError(null);
    setBusy("save");
    try {
      await deleteLLMProfile(selectedId);
      await refresh();
      toast("Profile removed.", "success");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(null);
    }
  };

  const activate = async () => {
    if (selectedId === null) return;
    setError(null);
    setBusy("save");
    try {
      const result = await activateLLMProfile(selectedId);
      await refresh();
      setServerInfo((current) =>
        current ? { ...current, model: result.profile.model, api: result.profile.api } : current,
      );
      toast(`${result.profile.name} is now active.`, "success");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(null);
    }
  };

  return (
    <Card size="sm" className="w-full">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <HugeiconsIcon icon={SettingsIcon} strokeWidth={2} />
          Model providers
        </CardTitle>
        <CardDescription>
          Save multiple provider profiles in SQLite. Switch the active profile from the composer.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {!form ? (
          error ? (
            <StatusMessage>{error}</StatusMessage>
          ) : (
            <LoadingState label="Loading model settings" />
          )
        ) : (
          <>
            <div className="grid gap-2">
              <Label>Profile</Label>
              <Select
                value={String(selectedId)}
                onValueChange={(value) => value && selectProfile(value)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {profiles.map((profile) => (
                    <SelectItem key={profile.id} value={String(profile.id)}>
                      {profile.name}
                      {profile.active ? " (active)" : ""}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Field label="Profile name">
              <Input value={form.name} onChange={(event) => update("name", event.target.value)} />
            </Field>
            <div className="grid gap-2">
              <Label>Protocol</Label>
              <Select value={form.api} onValueChange={(value) => value && update("api", value)}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {APIS.map((api) => (
                    <SelectItem key={api.value} value={api.value}>
                      {api.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Field label="Base URL">
              <Input
                value={form.baseURL}
                placeholder="https://api.openai.com/v1"
                onChange={(event) => update("baseURL", event.target.value)}
                spellCheck={false}
              />
            </Field>
            <Field label="API key (optional)">
              <Input
                type="password"
                value={form.apiKey}
                placeholder="Leave blank to keep the stored key"
                autoComplete="off"
                onChange={(event) => update("apiKey", event.target.value)}
              />
            </Field>
            <Field label="Model">
              <div className="flex flex-col gap-2">
                <Combobox
                  items={models}
                  value={form.model}
                  onValueChange={(value) => value && update("model", value)}
                  inputValue={form.model}
                  onInputValueChange={(value) => update("model", value)}
                >
                  <ComboboxInput
                    className="w-full"
                    placeholder="Type a model or choose from the list"
                    spellCheck={false}
                  />
                  <ComboboxContent>
                    <ComboboxList>
                      {models.map((model) => (
                        <ComboboxItem key={model} value={model}>
                          {model}
                        </ComboboxItem>
                      ))}
                      <ComboboxEmpty>No models match your search.</ComboboxEmpty>
                    </ComboboxList>
                  </ComboboxContent>
                </Combobox>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="self-start"
                  onClick={() => void fetchModels()}
                  disabled={busy !== null}
                >
                  {busy === "models" ? "Fetching..." : "Fetch models"}
                </Button>
              </div>
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field label="Context window">
                <Input
                  type="number"
                  min={1000}
                  step={1000}
                  value={form.contextWindow}
                  onChange={(event) => update("contextWindow", event.target.value)}
                />
              </Field>
              <Field label="Max tokens">
                <Input
                  type="number"
                  min={1}
                  step={1}
                  value={form.maxTokens}
                  onChange={(event) => update("maxTokens", event.target.value)}
                />
              </Field>
            </div>
            <StatusMessage>{error}</StatusMessage>
            {notice ? (
              <p className="flex items-center gap-2 text-sm text-foreground">
                <HugeiconsIcon icon={CheckmarkCircle02Icon} strokeWidth={2} />
                {notice}
              </p>
            ) : null}
            <div className="flex flex-wrap gap-2">
              <Button onClick={() => void save()} disabled={busy !== null}>
                {busy === "save" ? "Saving..." : "Save profile"}
              </Button>
              <Button variant="outline" onClick={() => void create()} disabled={busy !== null}>
                New profile
              </Button>
              <Button variant="outline" onClick={() => void activate()} disabled={busy !== null}>
                Use in chat
              </Button>
              <Button variant="outline" onClick={() => void test()} disabled={busy !== null}>
                {busy === "test" ? "Testing..." : "Test connection"}
              </Button>
              {profiles.length > 1 ? (
                <Button variant="ghost" onClick={() => void remove()} disabled={busy !== null}>
                  Delete profile
                </Button>
              ) : null}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
