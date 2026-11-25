import { useAtom } from "jotai/react";
import { useEffect, useState } from "react";
import { get } from "../api/client";
import type { KnowledgePage, KnowledgeSearchResult, PersonalData } from "../api/types";
import { knowledgePageAtom, knowledgeResultsAtom, personalAtom } from "../atoms";
import { useToast } from "../lib/useToast";

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
      toast(`检索失败：${error instanceof Error ? error.message : String(error)}`, "warning");
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
      toast(`读取失败：${error instanceof Error ? error.message : String(error)}`, "warning");
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <section>
        <div className="card-title">知识检索</div>
        <p className="empty-note">
          {personal?.knowledge?.enabled
            ? `只读范围：${(personal.knowledge.allow ?? []).join("、") || "全部"}`
            : `知识库未启用：${personal?.knowledge?.reason ?? "未配置"}`}
        </p>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void search();
          }}
        >
          <label>
            检索词（1～200 字）
            <input
              name="q"
              maxLength={200}
              required
              value={query}
              placeholder="输入关键词"
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>
          <label className="label">
            <input
              type="checkbox"
              checked={history}
              onChange={(event) => setHistory(event.target.checked)}
            />
            包含归档笔记
          </label>
          <button type="submit" className="btn btn-primary" disabled={busy || !query.trim()}>
            搜索
          </button>
        </form>
      </section>

      <section>
        <div className="card-title">检索结果</div>
        {results.length === 0 ? (
          <p className="empty-note">输入关键词开始检索。</p>
        ) : (
          results.map((hit) => (
            <button
              type="button"
              className="example-card"
              key={`${hit.path}-${hit.line}`}
              onClick={() => void readPage(hit.path, hit.line)}
            >
              <span>
                {hit.path}:{hit.line === hit.endLine ? hit.line : `${hit.line}-${hit.endLine}`} [
                {hit.status}]
              </span>
              <pre>{hit.excerpt}</pre>
            </button>
          ))
        )}
      </section>

      {page ? (
        <section>
          <div className="card-title">笔记内容</div>
          <p className="label">
            {page.path}:{page.startLine}-{page.endLine} [{page.status}] {page.revision}
            {page.truncated ? "（已截断）" : ""}
          </p>
          <pre>{page.text}</pre>
          <button
            type="button"
            className="btn"
            onClick={() => void readPage(page.path, page.endLine + 1)}
          >
            读取后续内容
          </button>
        </section>
      ) : null}
    </>
  );
}
