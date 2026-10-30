import { useNavigate, useSearch } from "@tanstack/react-router";
import { useLayoutEffect, useState, type FormEvent } from "react";
import {
  useCatalog,
  useCorrectedSkillSearch,
  useMatchReasons,
  withMatchReasons,
} from "../../skill";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Loading } from "../../../shared/ui/Loading";
import { useGenerateEntryPoint } from "../../creation";
import { useMe } from "../../../core/session/me.service";
import type { HomeSearch } from "../../../app/router";
import type { SearchCorrection, SearchFilters } from "../../../core/api/types";
import { Catalog } from "./components/Catalog";
import { CategoryNav } from "./components/CategoryNav";
import { FilterBar } from "./components/FilterBar";
import { SearchHeroForm } from "./components/SearchHeroForm";
import { SearchResultsSection } from "./components/SearchResultsSection";
import { CompareBar } from "./components/CompareBar";
import {
  clearedFilterFields,
  filtersFromSearch,
  nextSelection,
  parseSelection,
  linkedQueryError,
  queryLengthError,
} from "./Home.model";
import "./Home.page.css";

function updatedSearch(
  previous: HomeSearch,
  next: Partial<HomeSearch>,
  preserveComparison: boolean,
) {
  return {
    ...previous,
    compare: preserveComparison ? previous.compare : undefined,
    ...next,
  };
}

function useSearchDraft(query: string | undefined) {
  const [draft, setDraft] = useState(query ?? "");
  const [trackedQuery, setTrackedQuery] = useState(query);
  if (query !== trackedQuery) {
    setTrackedQuery(query);
    setDraft(query ?? "");
  }
  return [draft, setDraft] as const;
}

function revealFocusedComparison() {
  document
    .querySelector('.home .search-results input[type="checkbox"]:focus')
    ?.scrollIntoView({ block: "nearest" });
}

export function Home() {
  const search = useSearch({ from: "/" });
  const navigate = useNavigate({ from: "/" });
  const [draft, setDraft] = useSearchDraft(search.q);
  const [queryError, setQueryError] = useState("");
  const selected = parseSelection(search.compare);
  const generateExposed = useGenerateEntryPoint();
  const loggedIn = !!useMe().data;

  const filters: SearchFilters = filtersFromSearch(search);
  const linkedError = linkedQueryError(search.q);
  const result = useCorrectedSkillSearch(
    search.q ?? "",
    filters,
    search.q !== undefined && !linkedError,
    search.correction,
  );
  const data = result.error || search.q === undefined ? undefined : result.data;
  const reasons = useMatchReasons(data);
  useLayoutEffect(revealFocusedComparison, [search.compare, reasons.data]);
  const effectiveFilters = data?.interpretation?.filters ?? filters;
  const currentCorrection = data?.interpretation && {
    intent: data.interpretation.intent,
    keywords: data.interpretation.keywords,
  };
  const browsing = search.q === undefined;
  const catalog = useCatalog(filters, browsing);

  function submitSearch(next: Partial<HomeSearch>, preserveComparison = false) {
    void navigate({
      search: (prev: HomeSearch) => updatedSearch(prev, next, preserveComparison),
      replace: true,
    });
  }

  function clearFilters() {
    if (currentCorrection) return applyCorrection(currentCorrection, true);
    void navigate({
      search: (prev: HomeSearch) => ({ q: search.q, compare: prev.compare }),
      replace: true,
    });
  }

  function applyCorrection(correction: SearchCorrection, clear = false) {
    submitSearch(
      {
        ...(clear ? clearedFilterFields() : effectiveFilters),
        correction: JSON.stringify(correction),
      },
      true,
    );
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = draft.trim();
    const lengthError = queryLengthError(trimmed);
    setQueryError(lengthError ?? "");
    if (lengthError) return;
    submitSearch({ q: trimmed || undefined, correction: undefined });
  }

  function toggleSelected(skillId: string) {
    void navigate({
      search: (prev: HomeSearch) => {
        const next = nextSelection(parseSelection(prev.compare), skillId);
        return { ...prev, compare: next.length ? next.join(",") : undefined };
      },
      replace: true,
      resetScroll: false,
    });
  }

  const catalogView = (
    <Catalog query={catalog} selected={selected} onToggle={toggleSelected} filters={filters} />
  );
  return (
    <section className={browsing ? "home" : "home home-searching"}>
      <SearchHeroForm
        compact={!browsing}
        draft={draft}
        queryError={queryError || linkedError}
        onDraftChange={(value) => {
          setDraft(value);
          if (queryError) setQueryError("");
        }}
        onSubmit={handleSubmit}
      />

      {!browsing && <SearchReadState query={result} />}
      {browsing && catalog.error && catalogView}
      <CompareBar selected={selected} />

      <CategoryNav
        onCountsChange={revealFocusedComparison}
        filters={effectiveFilters}
        browsing={browsing}
        correction={currentCorrection ? JSON.stringify(currentCorrection) : search.correction}
      />

      <FilterBar
        filters={effectiveFilters}
        onChange={(next) =>
          submitSearch(
            {
              ...effectiveFilters,
              ...next,
              correction: currentCorrection ? JSON.stringify(currentCorrection) : search.correction,
            },
            true,
          )
        }
      />

      {browsing && !catalog.error && catalogView}

      {!browsing && data && (
        <SearchResultsSection
          data={withMatchReasons(data, reasons.data)}
          confirmed={result.fetchStatus === "idle"}
          selected={selected}
          loggedIn={loggedIn}
          generateExposed={generateExposed}
          onCorrect={applyCorrection}
          onClearFilters={clearFilters}
          onToggle={toggleSelected}
        />
      )}
    </section>
  );
}

function SearchReadState({ query }: { query: ReturnType<typeof useCorrectedSkillSearch> }) {
  return (
    <>
      {query.isFetching && <p role="status">搜尋中…</p>}
      {query.fetchStatus === "paused" && <Loading what="搜尋結果" paused />}
      <ReadFailure
        error={query.error}
        what="搜尋結果"
        retry={() => void query.refetch()}
        retrying={query.fetchStatus !== "idle"}
        focusTarget="catalog-search-heading"
      />
    </>
  );
}
