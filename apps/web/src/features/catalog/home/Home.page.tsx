import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useState, type FormEvent } from "react";
import { useCatalog, useSkillSearch } from "../../skill";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { useGenerateEntryPoint } from "../../creation";
import { useMe } from "../../../core/session/me.service";
import { MAX_COMPARE } from "../../skill";
import type { HomeSearch } from "../../../app/router";
import type { SearchCorrection, SearchFilters } from "../../../core/api/types";
import { Catalog } from "./components/Catalog";
import { CategoryNav } from "./components/CategoryNav";
import { FilterBar } from "./components/FilterBar";
import { IntentInterpretation } from "./components/IntentInterpretation";
import { NoResultsPanel } from "./components/NoResultsPanel";
import { SearchResultsList } from "./components/SearchResultsList";
import "./Home.page.css";

function parseSelection(value: string | undefined): string[] {
  return value ? value.split(",").filter(Boolean).slice(0, MAX_COMPARE) : [];
}

const SEARCH_MAX_QUERY = 2000; // one-number: searchMaxQueryRunes

const runes = (s: string) => [...s].length;

export function Home() {
  const search = useSearch({ from: "/" });
  const navigate = useNavigate({ from: "/" });
  const [draft, setDraft] = useState(search.q ?? "");
  const [trackedQ, setTrackedQ] = useState(search.q);
  if (search.q !== trackedQ) {
    setTrackedQ(search.q);
    setDraft(search.q ?? "");
  }
  const [queryError, setQueryError] = useState("");
  const selected = parseSelection(search.compare);
  const generateExposed = useGenerateEntryPoint();
  const loggedIn = !!useMe().data;

  const filters: SearchFilters = {
    script: search.script,
    validation: search.validation,
    agent: search.agent,
    tier: search.tier,
    category: search.category,
  };
  const { data, isFetching, error } = useSkillSearch(
    search.q ?? "",
    filters,
    search.q !== undefined,
    undefined,
    search.correction,
  );
  const effectiveFilters = data?.interpretation?.filters ?? filters;
  const currentCorrection = data?.interpretation && {
    intent: data.interpretation.intent,
    keywords: data.interpretation.keywords,
  };
  const browsing = search.q === undefined;
  const catalog = useCatalog(filters, browsing);

  function submitSearch(next: Partial<typeof search>) {
    void navigate({
      search: (prev) => ({ ...prev, compare: undefined, ...next }),
      replace: true,
    });
  }

  function clearFilters() {
    if (currentCorrection) {
      applyCorrection(currentCorrection, true);
      return;
    }
    void navigate({ search: { q: search.q }, replace: true });
  }

  function applyCorrection(correction: SearchCorrection, clear = false) {
    submitSearch({
      ...(clear
        ? {
            script: undefined,
            validation: undefined,
            agent: undefined,
            tier: undefined,
            category: undefined,
          }
        : effectiveFilters),
      correction: JSON.stringify(correction),
    });
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = draft.trim();
    const count = runes(trimmed);
    if (count > SEARCH_MAX_QUERY) {
      setQueryError(`搜尋文字最多 ${SEARCH_MAX_QUERY} 字，目前 ${count} 字。`);
      return;
    }
    setQueryError("");
    submitSearch({ q: trimmed || undefined, correction: undefined });
  }

  function toggleSelected(skillId: string) {
    void navigate({
      search: (prev: HomeSearch) => {
        const current = parseSelection(prev.compare);
        const next = current.includes(skillId)
          ? current.filter((id) => id !== skillId)
          : current.length >= MAX_COMPARE
            ? current
            : [...current, skillId];
        return { ...prev, compare: next.length ? next.join(",") : undefined };
      },
      replace: true,
    });
  }

  return (
    <section className="home">
      <div className="hero">
        <h1>用一句話描述你的任務</h1>
        <form onSubmit={handleSubmit}>
          <input
            type="text"
            value={draft}
            onChange={(event) => {
              setDraft(event.target.value);
              if (queryError) setQueryError("");
            }}
            placeholder="在目錄裡找一個 Skill，例如：把這份 PDF 整理成摘要"
            aria-label="任務描述"
          />
          {queryError && <p role="alert">{queryError}</p>}
          <button type="submit" className="action">
            搜尋
          </button>
          <Link className="hero-create" to="/workspace/skills" hash="create">
            自己做一個 Skill
          </Link>
        </form>
      </div>

      <CategoryNav
        filters={effectiveFilters}
        browsing={browsing}
        correction={currentCorrection ? JSON.stringify(currentCorrection) : search.correction}
      />

      <FilterBar
        filters={effectiveFilters}
        onChange={(next) =>
          submitSearch({
            ...effectiveFilters,
            ...next,
            correction: currentCorrection ? JSON.stringify(currentCorrection) : search.correction,
          })
        }
      />

      {browsing && (
        <Catalog
          query={catalog}
          selected={selected}
          onToggle={toggleSelected}
          narrowing={Object.values(filters).some((value) => value !== undefined)}
          tierFiltered={filters.tier !== undefined}
        />
      )}

      {!browsing && isFetching && <p role="status">搜尋中…</p>}
      {!browsing && <ReadFailure error={error} what="搜尋結果" />}

      {!browsing && data && (
        <>
          <p>
            查詢：<q>{data.query}</q>
          </p>

          {data.interpretation && data.interpretation.status !== "skipped" && (
            <IntentInterpretation
              key={`${data.query}:${JSON.stringify(data.interpretation)}`}
              interpretation={data.interpretation}
              onCorrect={applyCorrection}
            />
          )}

          {data.degraded && (
            <p className="notice" role="status">
              目前只用關鍵字比對搜尋，跨語言與語意相近的結果會找不到，召回率明顯較低。
            </p>
          )}
          {data.partial_index && (
            <p className="notice" role="status">
              部分 Skill 尚未建立語意索引，只能靠關鍵字命中，沒有相似度可顯示，並排在最後。
            </p>
          )}
          {data.truncated && (
            <p className="notice" role="status">
              符合的 Skill 共 {data.total} 個，這裡只列出最接近的 {data.results.length} 個。
              目前沒有翻頁；縮小任務描述或加上篩選條件會讓排序更貼近你要的。
            </p>
          )}

          {data.filtered_out && (
            <div>
              <p>有符合這個任務的 Skill，但全部被目前的篩選條件排除了。</p>
              <p>放寬或清除下方的篩選條件即可看到它們。</p>
              <button type="button" onClick={clearFilters}>
                清除所有篩選
              </button>
            </div>
          )}

          {data.no_results && (
            <NoResultsPanel
              query={data.query}
              degraded={data.degraded}
              querySuggestion={data.query_suggestion}
              loggedIn={loggedIn}
              generateExposed={generateExposed}
            />
          )}

          {data.results.length > 0 && (
            <SearchResultsList
              query={data.query}
              results={data.results}
              selected={selected}
              atLimit={selected.length >= MAX_COMPARE}
              onToggle={toggleSelected}
            />
          )}
        </>
      )}
    </section>
  );
}
