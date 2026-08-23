import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useState, type FormEvent } from "react";
import { useSkillSearch } from "../api/skills";
import { useGenerateEntryPoint } from "../api/generate";
import { GenerateSkill } from "../components/GenerateSkill";
import { LabelledBadge } from "../components/LabelledBadge";
import { RiskSummary } from "../components/RiskIndicator";
import { MAX_COMPARE } from "./Compare";
import type { PublicSearchResult, SearchFilters } from "../api/types";

/**
 * DISC-001/002/003/004/005: public intent search. Anonymous — GET
 * /api/skills/search is mounted without RequireSession, so nothing on this page
 * needs a session.
 *
 * The empty / low-confidence / degraded states come from the server's four
 * separate flags and are shown separately, because they mean different things:
 * `no_results` = nothing was close enough, `filtered_out` = there were matches
 * but the filters removed them, `degraded` = we could not look properly,
 * `partial_index` = part of the catalog is not searchable yet.
 *
 * Query and filters live in the URL (see router.tsx), so a filtered result page
 * can be shared and survives a reload.
 */
export function Home() {
  const search = useSearch({ from: "/" });
  const navigate = useNavigate({ from: "/" });
  const [draft, setDraft] = useState(search.q ?? "");
  const [selected, setSelected] = useState<string[]>([]);
  const generateExposed = useGenerateEntryPoint();

  const filters: SearchFilters = {
    script: search.script,
    validation: search.validation,
    agent: search.agent,
  };
  // `undefined` = nothing submitted yet, so no request. `""` is a blank submit,
  // which the server answers with no_results plus the suggestion copy (DISC-005);
  // duplicating that copy here is exactly what the acceptance criteria stopped
  // asking for.
  const { data, isFetching, isError } = useSkillSearch(
    search.q ?? "",
    filters,
    search.q !== undefined,
  );

  /**
   * Any change to the question — new query or new filter — makes the old
   * selection meaningless: the ids may not be on the page any more, and a
   * hidden selection would compare skills the user can no longer see.
   */
  function submitSearch(next: Partial<typeof search>) {
    setSelected([]);
    void navigate({ search: (prev) => ({ ...prev, ...next }), replace: true });
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    submitSearch({ q: draft.trim() });
  }

  function toggleSelected(skillId: string) {
    setSelected((current) =>
      current.includes(skillId)
        ? current.filter((id) => id !== skillId)
        : current.length >= MAX_COMPARE
          ? current
          : [...current, skillId],
    );
  }

  return (
    // `home` is a styling hook, not a variant: this is the only route whose h1
    // is a hero rather than a document title, and the only one whose form is the
    // page's primary control (see index.css `.home h1` / `.home form`).
    <section className="home">
      <h1>用一句話描述你的任務</h1>
      <form onSubmit={handleSubmit}>
        <input
          type="text"
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="例如：把這份 PDF 整理成摘要"
          aria-label="任務描述"
        />
        <button type="submit">搜尋</button>
      </form>

      <FilterBar
        filters={filters}
        onChange={(next) => submitSearch(next)}
        disabled={search.q === undefined}
      />

      {isFetching && <p role="status">搜尋中…</p>}
      {isError && <p role="alert">搜尋失敗，請稍後再試。</p>}

      {data && (
        <>
          {/* DISC-001: the original query is kept and echoed, not rewritten away. */}
          <p>
            查詢：<q>{data.query}</q>
          </p>

          {/* Non-blocking notices: results (if any) are still shown below. */}
          {data.degraded && (
            <p className="notice" role="status">
              目前只用關鍵字比對搜尋，跨語言與語意相近的結果會找不到，召回率明顯較低。
              {data.degraded_reason && <span className="note">（{data.degraded_reason}）</span>}
            </p>
          )}
          {data.partial_index && (
            <p className="notice" role="status">
              部分 Skill 尚未建立語意索引，只能靠關鍵字命中，沒有相似度可顯示，並排在最後。
            </p>
          )}
          {/*
            ADR-042 決策 3 / 設計 §4.3: 這個上限一直都在（預設 20），而第 21 筆
            對這一頁而言不存在——**被截斷的清單必須說出自己被截斷了**，否則它讀
            起來就是完整答案。與上面兩則刻意分開：那兩則說的是「我們看得夠不夠
            清楚」，這一則說的是「找到的東西有多少在這一頁上」。
          */}
          {data.truncated && (
            <p className="notice" role="status">
              符合的 Skill 超過 {data.limit} 個，這裡只列出最接近的 {data.limit} 個。
              目前沒有翻頁；縮小任務描述或加上篩選條件會讓排序更貼近你要的。
            </p>
          )}

          {/*
            DISC-003: the two empty states are different problems with different
            fixes, so they never share copy. Widening a filter and rewording a
            task are opposite advice, and giving the wrong one sends the user
            looking in a place where the answer is not.
          */}
          {data.filtered_out && (
            <div>
              <p>有符合這個任務的 Skill，但全部被目前的篩選條件排除了。</p>
              <p>放寬或清除下方的篩選條件即可看到它們。</p>
              <button
                type="button"
                onClick={() => submitSearch({ script: undefined, validation: undefined })}
              >
                清除所有篩選
              </button>
            </div>
          )}

          {data.no_results && (
            <div>
              <p>沒有夠接近的 Skill。</p>
              {/* DISC-005: the suggestion is the server's, not a hardcoded string. */}
              {data.query_suggestion && <p>{data.query_suggestion}</p>}
              {/*
                GEN-004's entry point, and only here — never in the
                `filtered_out` branch above (widening a filter and describing a
                task are opposite advice) and never beside the search box
                (ADR-046 決策 7: 先搜尋、搜不到再生成 is a product opinion, and
                an entry point of equal weight says the opposite).

                `generateExposed` is ADR-052's flag, read from /me. Off — which
                is the default and the state every beta deployment is in until
                01 §11.2's first funnel segment has a reading — and none of this
                renders.
              */}
              {generateExposed && <GenerateSkill initialTask={data.query} />}
            </div>
          )}

          {data.results.length > 0 && (
            <>
              <RankingExplainer degraded={data.degraded} partialIndex={data.partial_index} />
              <CompareBar selected={selected} />
              {/*
                The count is the live region, not the list: a list of result
                cards under aria-live makes a screen reader re-read every card
                in full on each search, which is louder than saying nothing.
              */}
              <p role="status" className="note">
                找到 {data.results.length} 個 Skill。
              </p>
              <ul className="search-results" aria-label="搜尋結果">
                {data.results.map((hit) => (
                  <SearchResultRow
                    key={hit.skill_id}
                    hit={hit}
                    checked={selected.includes(hit.skill_id)}
                    atLimit={selected.length >= MAX_COMPARE}
                    onToggle={toggleSelected}
                  />
                ))}
              </ul>
            </>
          )}
        </>
      )}
    </section>
  );
}

/**
 * DISC-003 (spec 02:DISC-002「使用者可依類別、來源層級、Agent、是否包含 Script、
 * 是否需要 MCP 與驗證狀態篩選」).
 *
 * Three of the six dimensions have per-row data in this build and are live
 * controls. The other three are rendered as disabled controls carrying the
 * reason, rather than being hidden or — far worse — offered as controls that
 * accept a value and narrow nothing. The server rejects those three with 400 for
 * the same reason, so a hand-edited URL cannot get an unfiltered page that looks
 * filtered.
 *
 * The wording of each reason is the honest one, not a "coming soon": MCP has no
 * source of truth anywhere in the pipeline, and 類別/來源層級 are curation data
 * the platform never stored.
 *
 * Agent 相容 became live with the M2 baseline measurements (0022): 45 skills,
 * one sandbox Run each. Only its runtime axis is a filter — every measured skill
 * came back `activated`, so a capability filter would separate nothing, which is
 * exactly why 來源層級 is still disabled.
 */
const UNAVAILABLE_FILTERS: Array<{ key: string; label: string; reason: string }> = [
  {
    key: "category",
    label: "類別",
    reason: "類別目前只存在於策展清單，沒有存進平台，匯入流程也不收這個欄位，因此無法篩選。",
  },
  {
    key: "tier",
    label: "來源層級",
    reason: "目前目錄內每一個 Skill 都是「已索引」層級，人工精選審查尚未開始，篩了也分不出東西。",
  },
  {
    key: "mcp",
    label: "需要 MCP",
    reason: "靜態掃描與 SKILL.md 都沒有記錄是否需要 MCP，平台沒有這項資料可以篩。",
  },
];

function FilterBar({
  filters,
  onChange,
  disabled,
}: {
  filters: SearchFilters;
  onChange: (next: SearchFilters) => void;
  disabled: boolean;
}) {
  /*
    §2.4「停用要說原因」: these three are live filters, dead only until there is a
    result set to narrow, and until this batch nothing said so — while the note
    at the bottom of the bar explains that the *other* three are the unavailable
    ones, which reads as a promise that these work. The reason gets the same
    shape as the unavailable dimensions below (visible .note + aria-describedby,
    never a `title`): a tooltip does not exist on touch, and QA-009 already
    established that the reason is the honest part of the feature, not a
    footnote. One node, referenced by all three — three copies of one sentence
    is noise, and aria-describedby may be pointed at a shared id.
  */
  const whyDisabled = disabled ? "filter-why-query" : undefined;

  return (
    <div className="filter-bar" role="group" aria-label="篩選條件">
      <label>
        是否包含 Script
        <select
          value={filters.script ?? ""}
          disabled={disabled}
          aria-describedby={whyDisabled}
          onChange={(event) =>
            onChange({ script: (event.target.value || undefined) as SearchFilters["script"] })
          }
        >
          <option value="">不限</option>
          <option value="yes">包含 Script</option>
          <option value="no">不含 Script</option>
        </select>
      </label>

      <label>
        驗證狀態
        <select
          value={filters.validation ?? ""}
          disabled={disabled}
          aria-describedby={whyDisabled}
          onChange={(event) =>
            onChange({
              validation: (event.target.value || undefined) as SearchFilters["validation"],
            })
          }
        >
          <option value="">不限</option>
          <option value="passed">規格驗證已通過</option>
          <option value="unverified">尚未驗證</option>
        </select>
      </label>

      {/*
        The runtime axis of DISC-002's Agent dimension. The wording says what the
        value means rather than repeating the value: 「模型轉譯」 is the one a
        reader cannot guess, and it is the one that decides whether the Skill's
        own script is what runs.
      */}
      <label>
        Agent 相容（實測）
        <select
          value={filters.agent ?? ""}
          disabled={disabled}
          aria-describedby={whyDisabled}
          onChange={(event) =>
            onChange({ agent: (event.target.value || undefined) as SearchFilters["agent"] })
          }
        >
          <option value="">不限</option>
          <option value="native">腳本可直接執行</option>
          <option value="transpiled">腳本不會執行，由模型轉譯</option>
          <option value="failed">腳本無法執行且試跑失敗</option>
          <option value="unverified">尚未試跑</option>
        </select>
      </label>

      {disabled && (
        <p className="note" id="filter-why-query">
          先描述一次任務，才會有結果可以篩：上面三項在第一次搜尋之後才能使用。
        </p>
      )}

      {UNAVAILABLE_FILTERS.map(({ key, label, reason }) => (
        <label key={key} title={reason}>
          {label}
          <select disabled aria-describedby={`filter-why-${key}`}>
            <option>無法篩選</option>
          </select>
          <span id={`filter-why-${key}`} className="note">
            {reason}
          </span>
        </label>
      ))}

      {/*
        DISC-003 honesty note, sitting with the controls rather than in a help
        page: a filter bar with four dead controls has to say why on the spot,
        or it reads as a broken UI instead of an absent capability.
      */}
      <p className="note">
        篩選條件只會用平台真的有的資料。上面標為「無法篩選」的三項，是因為平台目前沒有這些資料，
        不是因為所有 Skill 都不符合。
      </p>
    </div>
  );
}

/**
 * DISC-004 (spec 02:DISC-002 「排序依據需可被簡要說明」): what actually decides
 * the order, in plain language.
 *
 * Every claim below is the measured behaviour of the pipeline this page calls,
 * not an ideal: ranking is vector distance alone and the lexical leg only
 * widens the candidate set (db/queries/search.sql `PublicHybridSearchSkills`,
 * after golden-query-set.md §10.7 measured equal-weight RRF costing 15 of 48
 * queries their Top-1). The two exceptions are the two states the response
 * already flags, and they are listed whether or not they apply right now —
 * the rule is what is being explained — with the live one marked.
 */
function RankingExplainer({
  degraded,
  partialIndex,
}: {
  degraded: boolean;
  partialIndex: boolean;
}) {
  return (
    <details className="ranking-explainer">
      <summary>排序依據（為什麼是這個順序？）</summary>
      <ul>
        <li>
          <strong>排的是語意相似度。</strong>
          系統把你輸入的描述和每個 Skill 的索引文字都轉成向量，兩者越接近就排越前面。每個結果標示的
          「相似度」就是這個分數，範圍 0 到 1，越大越接近。
        </li>
        <li>
          <strong>關鍵字命中只用來多找候選，不會改變名次。</strong>
          靠關鍵字被找出來的 Skill，一樣用它自己的語意相似度排序，不會因為字面命中而往前擠。
        </li>
        <li>
          {/*
            ponytail: 0.25 is transcribed from catalog.MaxCosineDistance (0.75
            cosine distance) and the contract does not expose it, so the page
            cannot stay in sync with it. §2.2 says do not print a value this
            surface does not own — the honest half-measure until the search
            response carries it is to attribute it: the sentence now says whose
            setting it is, so a reader who finds a 0.3 cut-off in the server has
            been told which of the two is authoritative. Move it onto the search
            response the next time the value changes; it is stated once here and
            referred to as 「那個門檻」 below rather than transcribed twice.
          */}
          <strong>相似度低於 0.25 的一律不顯示。</strong>
          這個 0.25 是平台目前的設定值，不是介面契約的一部分，調整了這一頁不會自己跟著改。全部都低於
          門檻時會直接說「沒有夠接近的 Skill」，而不是硬給一頁不相關的結果。這個門檻是實測
          出來的：在評測語料上，12 條離題查詢全部被正確拒答，同時 48 條正常查詢一條也沒漏掉。
        </li>
        <li>
          <strong>不看 Star 數、下載數或更新時間。</strong>
          排序完全不使用人氣或新舊，只有相關度。
        </li>
        <li>
          {/*
            DISC-003: filtering is not ranking. Saying so here is what stops a
            user reading a short filtered page as "the search got worse".
          */}
          <strong>篩選條件不影響名次。</strong>
          篩選只是把不符合條件的結果整個拿掉，剩下的順序和沒篩之前完全一樣。
        </li>
        <li>
          <strong>例外一：只能用關鍵字比對時。</strong>
          語意服務不可用的話，整頁改用關鍵字分數排序。那個分數不是相似度、沒有固定範圍，上面那個
          門檻也不生效；這時不顯示相似度，改在每個結果旁註明原因。篩選條件在這個狀態下照樣生效。
          {degraded && <span className="badge">目前這次搜尋就是這個狀態</span>}
        </li>
        <li>
          <strong>例外二：還沒建立語意索引的 Skill。</strong>
          這些 Skill
          算不出相似度，只能靠關鍵字被找到，固定排在最後，門檻也沒有判斷過它們；畫面上不顯示
          相似度，改註明原因。
          {partialIndex && <span className="badge">這頁就有這種結果</span>}
        </li>
      </ul>
    </details>
  );
}

/**
 * DISC-009 entry point: 2–3 selected candidates go to the comparison page.
 *
 * §2.4 again, on the other disabled control on this page: at MAX_COMPARE every
 * remaining checkbox goes dead, and the hint below used to say 「勾選 2 至 3
 * 個」 whether or not three were already picked — so the state that disables
 * them was the one state the copy never mentioned. The limit line is separate
 * from the two-candidate hint because at three the link branch renders and the
 * hint does not, which is exactly when the reason is needed.
 */
function CompareBar({ selected }: { selected: string[] }) {
  return (
    <div className="compare-bar">
      {selected.length >= 2 ? (
        <Link to="/compare" search={{ ids: selected.join(",") }}>
          並排比較這 {selected.length} 個 Skill
        </Link>
      ) : (
        <p className="note">勾選 2 至 {MAX_COMPARE} 個 Skill，即可並排比較它們的靜態資料。</p>
      )}
      {selected.length >= MAX_COMPARE && (
        <p className="note" id="compare-limit">
          已經選滿 {MAX_COMPARE}{" "}
          個，並排比較最多就是這麼多；其餘的勾選框會停用，取消一個才能改選別的。
        </p>
      )}
    </div>
  );
}

/**
 * 02:DISC-002 requires every result row to show name, plain summary, source
 * tier, agent compatibility, dependencies, a risk hint and the last
 * verification time. The API carries all seven; this renders the five that are
 * not the name and summary.
 *
 * Nothing here is inferred. An unscanned row says its scan status is unknown
 * rather than showing no flags, an empty dependency list says "not extracted"
 * rather than "none", and the two sandbox compatibility axes are marked
 * 尚未試跑 rather than left out — an absent row reads as "fine" (NFR-001).
 */

function ResultFacets({ hit }: { hit: PublicSearchResult }) {
  const untested =
    hit.compatibility.capability.value === "unverified" &&
    hit.compatibility.runtime.value === "unverified";

  return (
    <dl className="result-facets">
      <dt>來源層級</dt>
      <dd>
        <LabelledBadge kind="tier" value={hit.tier} />
      </dd>

      {/*
        Both server notes below were `title=` only. Three problems, one fix:
        obligation §1.1 wants the evidence *and its qualifier* visible, `title`
        does not exist on touch at all, and /compare renders these same two
        server strings as visible text — two surfaces stating one fact
        differently is 02:NFR-001. They are the server's copy either way
        (§4.4: the wording is the server's, the front end keeps no enum table).
      */}
      <dt>相容狀態</dt>
      <dd>
        規格驗證：{hit.compatibility.spec_validation.label}
        {/* DISC-002: 沒有驗證證據的 Skill 必須明確標記「尚未試跑」. */}
        {untested && <span className="badge badge-untested">尚未試跑</span>}
        <span className="note">{hit.compatibility.note}</span>
      </dd>

      <dt>依賴</dt>
      <dd>
        {hit.dependencies.length > 0 ? (
          hit.dependencies.join("、")
        ) : (
          <span className="note">未擷取到依賴資訊（不等於沒有依賴）</span>
        )}
      </dd>

      <dt>風險提示</dt>
      <dd>
        <RiskSummary risk={hit.risk} />
      </dd>

      <dt>最近驗證時間</dt>
      <dd>
        {hit.verified_at ? (
          hit.verified_at.slice(0, 10)
        ) : (
          <span className="note">尚未匯入內容</span>
        )}
      </dd>
    </dl>
  );
}

function SearchResultRow({
  hit,
  checked,
  atLimit,
  onToggle,
}: {
  hit: PublicSearchResult;
  checked: boolean;
  atLimit: boolean;
  onToggle: (skillId: string) => void;
}) {
  return (
    <li className="search-result">
      <label className="compare-pick">
        <input
          type="checkbox"
          checked={checked}
          disabled={!checked && atLimit}
          /* The limit line in CompareBar exists exactly when this is disabled. */
          aria-describedby={!checked && atLimit ? "compare-limit" : undefined}
          onChange={() => onToggle(hit.skill_id)}
        />
        加入比較
      </label>
      <Link to="/skills/$skillId" params={{ skillId: hit.skill_id }}>
        {hit.name}
      </Link>
      {/*
        ADR-013 again, and the important half of it. This row already badged
        `match_reason` as 「AI 產生」 three lines below while printing the model's
        rewrite of the summary in the same <p> the author's own text occupies —
        the footnote was marked and the sentence a reader decides on was not.
        Same badge, same title convention, so the two cannot drift apart.
      */}
      <p>
        {hit.summary}{" "}
        {hit.summary_source === "model" && (
          <span
            className="badge badge-source-model"
            title="這段摘要由模型改寫，不是套件作者寫的；你的 Agent 讀的是套件自己的 description"
          >
            AI 改寫
          </span>
        )}
        {hit.summary_source === "package" && (
          <span className="badge badge-source-package" title="套件自己的 frontmatter description">
            作者原文
          </span>
        )}
        {/*
          The contract requires the field, so this branch is a server that
          failed to answer — and the one thing it must not do is answer for it.
          Defaulting to 作者原文 would put the author's name on the model's
          sentence, which is the ADR-013 failure this whole change is about,
          reintroduced as a fallback.
        */}
        {hit.summary_source !== "model" && hit.summary_source !== "package" && (
          <span className="badge badge-source-unknown" title="伺服器沒有回報這段摘要的來源">
            來源未標示
          </span>
        )}
      </p>
      {hit.match_reason && (
        <p className="match-reason">
          符合原因：{hit.match_reason}
          {/*
            ADR-013: model-generated copy has to be visibly marked as such, so
            a reader can tell an LLM's explanation from a mechanical one derived
            from keyword overlap.
          */}
          {hit.match_reason_source === "model" && (
            <span className="badge badge-source-model" title="這段說明由模型產生，未經人工核對">
              AI 產生
            </span>
          )}
          {/* `badge-source-template` has no CSS rule, but disc.test.tsx counts
              this selector to assert template copy does not borrow the model
              marker, and SkillDetail.tsx uses the same class. Test hook, kept. */}
          {hit.match_reason_source === "template" && (
            <span className="badge badge-source-template" title="依查詢與文件的關鍵字重疊組出">
              規則產生
            </span>
          )}
        </p>
      )}
      <ResultFacets hit={hit} />
      {/*
        DISC-004: every candidate shows the score the order was built from.
        `rank` is null exactly when there is no similarity to show — the whole
        page came from the lexical leg, or this row has no embedding yet — and
        the server's `rank_note` says which. The unbounded lexical score is not
        substituted for it: a live FTS-only answer measured 1.4, and printing
        that under a 「相似度」 label would state a number it does not mean.
      */}
      <p className="rank">
        {hit.rank === null
          ? (hit.rank_note ?? "未計算語意相似度。")
          : `相似度 ${hit.rank.toFixed(2)}`}
      </p>
    </li>
  );
}
