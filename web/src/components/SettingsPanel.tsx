import { CheckmarkCircle02Icon, SettingsIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useAtomValue, useSetAtom } from "jotai/react";
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
import { fetchLLMModels, getLLMSettings, saveLLMSettings, testLLMSettings } from "../api/client";
import type { LLMSettings, LLMSettingsInput } from "../api/types";
import { serverInfoAtom } from "../atoms";
import { useToast } from "../lib/useToast";
import { Field, LoadingState, StatusMessage } from "./ui";

const APIS = [
  { value: "openai-completions", label: "OpenAI completions" },
  { value: "openai-responses", label: "OpenAI responses" },
  { value: "anthropic-messages", label: "Anthropic messages" },
] as const;

interface FormState {
  api: string;
  baseURL: string;
  model: string;
  contextWindow: string;
  maxTokens: string;
  apiKey: string;
}

function toForm(settings: LLMSettings): FormState {
  return {
    api: settings.api,
    baseURL: settings.baseURL,
    model: settings.model,
    contextWindow: String(settings.contextWindow),
    maxTokens: String(settings.maxTokens),
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

/** Model provider settings, persisted in SQLite and applied without a restart. */
export function SettingsPanel() {
  const setServerInfo = useSetAtom(serverInfoAtom);
  const info = useAtomValue(serverInfoAtom);
  const toast = useToast();
  const [loaded, setLoaded] = useState<LLMSettings | null>(null);
  const [form, setForm] = useState<FormState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [models, setModels] = useState<string[]>([]);
  const [busy, setBusy] = useState<"test" | "save" | "models" | null>(null);

  useEffect(() => {
    let cancelled = false;
    getLLMSettings()
      .then((settings) => {
        if (cancelled) return;
        setLoaded(settings);
        setForm(toForm(settings));
      })
      .catch((cause: unknown) => {
        if (!cancelled) setError(cause instanceof Error ? cause.message : String(cause));
      });
    return () => {
      cancelled = true;
    };
  }, []);

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
    // A blank field means "keep the stored key"; an explicit clear has its own
    // button so a save of other fields cannot accidentally remove a secret.
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
      const input: { api: string; baseURL: string; apiKey?: string } = {
        api: form.api,
        baseURL: form.baseURL.trim(),
      };
      if (form.apiKey.trim()) input.apiKey = form.apiKey.trim();
      const result = await fetchLLMModels(input);
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
      const names = result.models.slice(0, 3).join(", ");
      setNotice(`Connected in ${result.latencyMs} ms${names ? `. Models: ${names}` : "."}`);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(null);
    }
  };

  const save = async () => {
    const value = input();
    const invalid = value ? validate(form as FormState) : "Settings are still loading.";
    if (invalid || !value) {
      setError(invalid);
      return;
    }
    setError(null);
    setNotice(null);
    setBusy("save");
    try {
      const saved = await saveLLMSettings(value);
      setLoaded(saved);
      setForm((previous) => (previous ? { ...previous, apiKey: "" } : previous));
      if (info) {
        setServerInfo({ ...info, model: saved.model, api: saved.api });
      }
      toast(`Model settings saved. Active model: ${saved.model}.`, "success");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(null);
    }
  };

  const clearKey = async () => {
    const value = input();
    if (!value) return;
    value.apiKey = "";
    setError(null);
    setBusy("save");
    try {
      const saved = await saveLLMSettings(value);
      setLoaded(saved);
      setForm((previous) => (previous ? { ...previous, apiKey: "" } : previous));
      toast("Stored API key removed.", "success");
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
          Model provider
        </CardTitle>
        <CardDescription>
          Saved in SQLite and applied immediately. The API key is never sent back to the browser.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {!form || !loaded ? (
          error ? (
            <StatusMessage>{error}</StatusMessage>
          ) : (
            <LoadingState label="Loading model settings" />
          )
        ) : (
          <>
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
            <Field
              label={loaded.apiKeySet ? "API key (a key is already stored)" : "API key (optional)"}
            >
              <Input
                type="password"
                value={form.apiKey}
                placeholder={
                  loaded.apiKeySet ? "Leave blank to keep the stored key" : "No key stored"
                }
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
                {busy === "save" ? "Saving..." : "Save settings"}
              </Button>
              <Button variant="outline" onClick={() => void test()} disabled={busy !== null}>
                {busy === "test" ? "Testing..." : "Test connection"}
              </Button>
              {loaded.apiKeySet ? (
                <Button variant="ghost" onClick={() => void clearKey()} disabled={busy !== null}>
                  Remove stored key
                </Button>
              ) : null}
            </div>
            <p className="text-xs text-muted-foreground">
              The model change applies to new messages in existing conversations. Environment
              variables are only used the first time this database is opened.
            </p>
          </>
        )}
      </CardContent>
    </Card>
  );
}
