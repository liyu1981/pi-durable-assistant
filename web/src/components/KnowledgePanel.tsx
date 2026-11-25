import { useAtom } from "jotai/react";
import { useEffect, useState } from "react";
import { get } from "../api/client";
import type { KnowledgePage, KnowledgeSearchResult, PersonalData } from "../api/types";
import { knowledgePageAtom, knowledgeResultsAtom, personalAtom } from "../atoms";
import { useToast } from "../lib/useToast";
import { Badge, Button, CheckboxField, Field, SectionTitle } from "./ui";

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
    <>
      <section>
        <SectionTitle>Knowledge search</SectionTitle>
        <p className="text-sm text-primary/70">
          {knowledge?.enabled
            ? `Read-only scope: ${(knowledge.allow ?? []).join(", ") || "all paths"}`
            : `Knowledge base unavailable: ${knowledge?.reason ?? "not configured"}`}
        </p>
        <form
          className="mt-3 flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            void search();
          }}
        >
          <Field label="Search term (1-200 characters)">
            <input
              className="field"
              name="q"
              maxLength={200}
              required
              value={query}
              placeholder="Enter keywords"
              onChange={(event) => setQuery(event.target.value)}
            />
          </Field>
          <CheckboxField label="Include archived notes" checked={history} onChange={setHistory} />
          <Button type="submit" variant="primary" disabled={busy || !query.trim()}>
            {busy ? "Working..." : "Search"}
          </Button>
        </form>
      </section>

      <section>
        <SectionTitle>Results</SectionTitle>
        {results.length === 0 ? (
          <p className="text-sm text-primary/70">Enter a keyword to start searching.</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {results.map((hit) => (
              <li key={`${hit.path}-${hit.line}`}>
                <button
                  type="button"
                  className="w-full rounded-sm border border-primary/20 px-3 py-2 text-left hover:border-secondary"
                  onClick={() => void readPage(hit.path, hit.line)}
                >
                  <span className="label flex flex-wrap items-center gap-2 text-primary/70">
                    {hit.path}:{hit.line === hit.endLine ? hit.line : `${hit.line}-${hit.endLine}`}
                    <Badge tone="neutral">{hit.status}</Badge>
                  </span>
                  <span className="mt-2 block whitespace-pre-wrap break-words text-xs">
                    {hit.excerpt}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      {page ? (
        <section>
          <SectionTitle>Note content</SectionTitle>
          <p className="label text-primary/70">
            {page.path}:{page.startLine}-{page.endLine} [{page.status}] {page.revision}
            {page.truncated ? " (truncated)" : ""}
          </p>
          <pre className="mt-2 max-h-72 overflow-auto whitespace-pre-wrap break-words border-l-2 border-secondary pl-3 text-xs">
            {page.text}
          </pre>
          <Button
            className="mt-3"
            variant="quiet"
            onClick={() => void readPage(page.path, page.endLine + 1)}
          >
            Read next section
          </Button>
        </section>
      ) : null}
    </>
  );
}
