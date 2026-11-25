import { ArrowRight01Icon, BookOpen01Icon, SearchIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useAtom } from "jotai/react";
import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { get } from "../api/client";
import type { KnowledgePage, KnowledgeSearchResult, PersonalData } from "../api/types";
import { knowledgePageAtom, knowledgeResultsAtom, personalAtom } from "../atoms";
import { useToast } from "../lib/useToast";
import { Field, SwitchField } from "./ui";

export function KnowledgePanel() {
  const [results, setResults] = useAtom(knowledgeResultsAtom);
  const [page, setPage] = useAtom(knowledgePageAtom);
  const [personal, setPersonal] = useAtom(personalAtom);
  const toast = useToast();
  const [query, setQuery] = useState("");
  const [history, setHistory] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (personal !== null) return;
    let active = true;
    void (async () => {
      try {
        const data = await get<PersonalData>("/api/personal");
        if (active) setPersonal(data);
      } catch {
        // The status line falls back to "not configured".
      }
    })();
    return () => {
      active = false;
    };
  }, [personal, setPersonal]);

  const search = async () => {
    if (!query.trim()) return;
    setBusy(true);
    try {
      const params = new URLSearchParams({ q: query.trim() });
      if (history) params.set("history", "1");
      const found = await get<KnowledgeSearchResult>(`/api/knowledge/search?${params.toString()}`);
      setResults(found.hits);
      setPage(null);
    } catch (error) {
      toast(`Search failed: ${error instanceof Error ? error.message : String(error)}`, "warning");
    } finally {
      setBusy(false);
    }
  };

  const readPage = async (path: string, line: number) => {
    setBusy(true);
    try {
      const params = new URLSearchParams({ path, line: String(line) });
      setPage(await get<KnowledgePage>(`/api/knowledge/read?${params.toString()}`));
    } catch (error) {
      toast(`Read failed: ${error instanceof Error ? error.message : String(error)}`, "warning");
    } finally {
      setBusy(false);
    }
  };

  const knowledge = personal?.knowledge;

  return (
    <div className="flex flex-col gap-6">
      <Card size="sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <HugeiconsIcon icon={SearchIcon} strokeWidth={2} />
            Knowledge search
          </CardTitle>
        </CardHeader>
        <CardContent>
          <form
            className="flex flex-col gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              void search();
            }}
          >
            <p className="text-sm text-muted-foreground">
              {knowledge?.enabled
                ? `Read-only scope: ${(knowledge.allow ?? []).join(", ") || "all paths"}`
                : `Knowledge base unavailable: ${knowledge?.reason ?? "not configured"}`}
            </p>
            <Field label="Search term (1-200 characters)">
              <Input
                name="q"
                maxLength={200}
                required
                value={query}
                placeholder="Enter keywords"
                onChange={(event) => setQuery(event.target.value)}
              />
            </Field>
            <SwitchField
              label="Include archived notes"
              checked={history}
              onCheckedChange={setHistory}
            />
            <Button type="submit" disabled={busy || !query.trim()}>
              <HugeiconsIcon icon={SearchIcon} strokeWidth={2} />
              {busy ? "Working..." : "Search"}
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Results</CardTitle>
        </CardHeader>
        <CardContent>
          {results.length === 0 ? (
            <p className="text-sm text-muted-foreground">Enter a keyword to start searching.</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {results.map((hit) => (
                <li key={`${hit.path}-${hit.line}`}>
                  <Button
                    variant="outline"
                    className="h-auto w-full justify-start whitespace-normal py-3 text-left"
                    onClick={() => void readPage(hit.path, hit.line)}
                  >
                    <span className="flex w-full flex-col gap-2">
                      <span className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                        {hit.path}:
                        {hit.line === hit.endLine ? hit.line : `${hit.line}-${hit.endLine}`}
                        <Badge variant="outline">{hit.status}</Badge>
                      </span>
                      <span className="whitespace-pre-wrap break-words text-xs">{hit.excerpt}</span>
                    </span>
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      {page ? (
        <Card size="sm">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <HugeiconsIcon icon={BookOpen01Icon} strokeWidth={2} />
              Note content
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <p className="text-xs text-muted-foreground">
              {page.path}:{page.startLine}-{page.endLine} [{page.status}] {page.revision}
              {page.truncated ? " (truncated)" : ""}
            </p>
            <ScrollArea className="h-64 rounded-xl border border-border bg-muted/30 p-3">
              <pre className="whitespace-pre-wrap break-words font-mono text-xs">{page.text}</pre>
            </ScrollArea>
            <Button
              variant="outline"
              size="sm"
              onClick={() => void readPage(page.path, page.endLine + 1)}
            >
              Read next section
              <HugeiconsIcon icon={ArrowRight01Icon} strokeWidth={2} />
            </Button>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}
