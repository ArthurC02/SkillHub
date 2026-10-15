import { useEffect, useState } from "react";
import { canRetainLoadedPage } from "../../../core/api/client";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { useOwnSkillPages, ownSkillsFromPages, useSkillSearch } from "../../skill";
import { GENERATE_MAX_REFERENCES } from "../generate.model";

export function ReferencePicker({
  references,
  onToggle,
  disabled,
}: {
  references: { id: string; name: string }[];
  onToggle: (id: string, name: string) => void;
  disabled: boolean;
}) {
  const [query, setQuery] = useState("");
  const [debounced, setDebounced] = useState("");
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(query.trim()), 300);
    return () => clearTimeout(timer);
  }, [query]);

  const searching = debounced.length > 0;
  const atLimit = references.length >= GENERATE_MAX_REFERENCES;
  const selectedIds = new Set(references.map((r) => r.id));

  return (
    <div>
      <div className="field">
        <label htmlFor="generate-reference-query">搜尋要參考的小工具</label>
        <input
          id="generate-reference-query"
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="輸入名稱或關鍵字"
          disabled={disabled}
        />
      </div>
      <p className="note">
        模型會讀你選的小工具的說明檔（SKILL.md）當範例，最多 {GENERATE_MAX_REFERENCES} 個；
        產出仍是你工作區裡一個全新的小工具。 有授權暫扣或禁止再散布的目錄小工具無法被選為參考。
      </p>

      {references.length > 0 && (
        <ul className="badge-row" aria-label="已選的參考小工具">
          {references.map((r) => (
            <li key={r.id}>
              <button type="button" onClick={() => onToggle(r.id, r.name)} disabled={disabled}>
                {r.name} ✕
              </button>
            </li>
          ))}
        </ul>
      )}
      {atLimit && (
        <p className="note" id="generate-reference-limit">
          已經選滿 {GENERATE_MAX_REFERENCES} 個，取消一個才能改選別的。
        </p>
      )}

      {searching && (
        <>
          <ReferenceSearchResults
            query={debounced}
            selectedIds={selectedIds}
            atLimit={atLimit}
            disabled={disabled}
            onToggle={onToggle}
          />
          <OwnedReferenceResults
            query={debounced}
            selectedIds={selectedIds}
            atLimit={atLimit}
            disabled={disabled}
            onToggle={onToggle}
          />
        </>
      )}
    </div>
  );
}

type ReferenceResultsProps = {
  query: string;
  selectedIds: Set<string>;
  atLimit: boolean;
  disabled: boolean;
  onToggle: (id: string, name: string) => void;
};

function ReferenceSearchResults({
  query,
  selectedIds,
  atLimit,
  disabled,
  onToggle,
}: ReferenceResultsProps) {
  const search = useSkillSearch(query, {}, true, "reference");
  return (
    <>
      <h3 id="reference-search-heading" tabIndex={-1}>
        搜尋結果
      </h3>
      {search.isFetching && <p role="status">搜尋中…</p>}
      <ReadFailure
        error={search.error}
        what="參考小工具的搜尋結果"
        retry={() => void search.refetch()}
        retrying={search.isFetching}
        focusTarget="reference-search-heading"
      />
      {!search.isFetching && !search.error && search.data?.results.length === 0 && (
        <p role="status">目錄裡沒有符合的小工具。</p>
      )}
      <ul className="search-results">
        {(search.error ? [] : (search.data?.results ?? [])).map((hit) => (
          <ReferenceRow
            key={hit.skill_id}
            skillId={hit.skill_id}
            name={hit.name}
            summary={hit.summary}
            className="search-result"
            checked={selectedIds.has(hit.skill_id)}
            disabled={disabled || (!selectedIds.has(hit.skill_id) && atLimit)}
            onToggle={() => onToggle(hit.skill_id, hit.name)}
          />
        ))}
      </ul>
    </>
  );
}

function OwnedReferenceResults({
  query,
  selectedIds,
  atLimit,
  disabled,
  onToggle,
}: ReferenceResultsProps) {
  const ownSkills = useOwnSkillPages();
  const nextPageFailure = canRetainLoadedPage(ownSkills.error, ownSkills.isFetchNextPageError);
  const own = ownSkillsFromPages(
    ownSkills.error && !nextPageFailure ? undefined : ownSkills.data?.pages,
  );
  const ownMatches = (own?.skills ?? []).filter(
    (s) =>
      s.name.toLowerCase().includes(query.toLowerCase()) ||
      s.summary.toLowerCase().includes(query.toLowerCase()),
  );

  return (
    <>
      <h3 id="reference-assets-heading" tabIndex={-1}>
        資產庫中的小工具
      </h3>
      {ownSkills.isPending && <p role="status">載入資產庫中的小工具…</p>}
      <ReadFailure
        error={ownSkills.error}
        what="資產庫中的小工具"
        retry={() => void (nextPageFailure ? ownSkills.fetchNextPage() : ownSkills.refetch())}
        retrying={ownSkills.isFetching}
        focusTarget="reference-assets-heading"
      />
      {!ownSkills.isPending && !ownSkills.error && own && ownMatches.length === 0 && (
        <p role="status">
          {ownSkills.hasNextPage
            ? "已載入的小工具中沒有符合項目；還有其他小工具尚未載入。"
            : "資產庫裡沒有符合項目。"}
        </p>
      )}
      <ul className="search-results">
        {ownMatches.map((s) => (
          <ReferenceRow
            key={s.skill_id}
            skillId={s.skill_id}
            name={s.name}
            summary={s.summary}
            className="download-item"
            checked={selectedIds.has(s.skill_id)}
            disabled={disabled || (!selectedIds.has(s.skill_id) && atLimit)}
            onToggle={() => onToggle(s.skill_id, s.name)}
          />
        ))}
      </ul>
      {own && (
        <p className="note" role="status">
          已載入 {own.skills.length} 個小工具{ownSkills.hasNextPage ? "；尚有更多。" : "。"}
        </p>
      )}
      {ownSkills.hasNextPage && !ownSkills.error && (
        <button
          type="button"
          disabled={disabled || ownSkills.isFetchingNextPage}
          onClick={() => void ownSkills.fetchNextPage()}
        >
          {ownSkills.isFetchingNextPage ? "載入中…" : "載入更多小工具"}
        </button>
      )}
    </>
  );
}

function ReferenceRow({
  name,
  summary,
  className,
  checked,
  disabled,
  onToggle,
}: {
  skillId: string;
  name: string;
  summary: string;
  className: string;
  checked: boolean;
  disabled: boolean;
  onToggle: () => void;
}) {
  return (
    <li className={className}>
      <label>
        <input
          type="checkbox"
          checked={checked}
          disabled={disabled}
          aria-describedby={disabled && !checked ? "generate-reference-limit" : undefined}
          onChange={onToggle}
        />
        {name}
      </label>
      <p className="note">{summary}</p>
    </li>
  );
}
