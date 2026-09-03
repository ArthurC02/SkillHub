import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useEffect, useState, type FormEvent } from "react";
import { useCatalog, useSkillSearch } from "../api/skills";
import { ReadFailure } from "../components/LoginRequired";
import { useGenerateEntryPoint } from "../api/generate";
import { useMe } from "../api/me";
import { GenerateSkill } from "../components/GenerateSkill";
import { Loading } from "../components/Loading";
import { LabelledBadge } from "../components/LabelledBadge";
import { RiskSummary } from "../components/RiskIndicator";
import { SignInAction } from "../components/SignIn";
import { Timestamp } from "../components/Timestamp";
import { MAX_COMPARE } from "./Compare";
import type { HomeSearch } from "../router";
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
/** 網址上的候選勾選，修剪到 DISC-009 的上限。空與缺席是同一個答案。 */
function parseSelection(value: string | undefined): string[] {
  return value ? value.split(",").filter(Boolean).slice(0, MAX_COMPARE) : [];
}

export function Home() {
  const search = useSearch({ from: "/" });
  const navigate = useNavigate({ from: "/" });
  const [draft, setDraft] = useState(search.q ?? "");
  useEffect(() => setDraft(search.q ?? ""), [search.q]);
  /**
   * DISC-009 的候選勾選，來源是網址而不是元件狀態。
   *
   * `compareRoute` 的註解自己寫著「the selection lives in the URL so a comparison
   * is linkable and survives a reload」——那個裁定在 `/compare` 上成立，在產生它的
   * 這一步上以前不成立：`useState` 撐不過一次導覽，所以「比較 → 上一頁 → 換掉一筆」
   * 每走一次都要重新勾兩個。修剪在這裡而不是在 `validateSearch`，與 `/compare`
   * 同一條規則（手改的網址落在一份合法的選擇上，不是錯誤頁）。
   */
  const selected = parseSelection(search.compare);
  const generateExposed = useGenerateEntryPoint();
  // DISC-001 serves this page to anyone; the exits below must not assume a session.
  const loggedIn = !!useMe().data;

  const filters: SearchFilters = {
    script: search.script,
    validation: search.validation,
    agent: search.agent,
    tier: search.tier,
  };
  // `undefined` = nothing submitted yet, so no request. `""` is a blank submit,
  // which the server answers with no_results plus the suggestion copy (DISC-005);
  // duplicating that copy here is exactly what the acceptance criteria stopped
  // asking for.
  const { data, isFetching, error } = useSkillSearch(
    search.q ?? "",
    filters,
    search.q !== undefined,
  );
  /*
    02:DISC-006. Until this, a first visit was a heading, an empty box and nothing
    else: the page asked every reader to describe a task before it would show
    them anything, so someone who did not already know what was in the catalogue
    had to guess a sentence to find out. 義務 1.2「快到第一個判斷」 cannot be met
    by a screen whose first judgement is behind a correct guess, and 資訊架構
    IA-5 records the same hole from the other end — when a search finds nothing,
    「what IS here」 had no address.

    Exclusive with the search read and never in flight beside it, so neither can
    be seen answering for the other. 資訊架構 R1「一個位址回答一個問題」 still
    holds: both states answer 「which skills should I look at」, one before the
    reader has narrowed it and one after.
  */
  const browsing = search.q === undefined;
  const catalog = useCatalog(filters, browsing);

  /**
   * Any change to the question — new query or new filter — makes the old
   * selection meaningless: the ids may not be on the page any more, and a
   * hidden selection would compare skills the user can no longer see.
   *
   * `compare: undefined` 是**明寫**的，因為這個函式淺層合併：選擇現在住在網址上，
   * 而合併會把它帶過新的查詢——那正是上面那段話說的、以前由一個 `useEffect` 負責
   * 擋掉的東西。同一個理由，同一行程式碼裡，而不是在別處的一個副作用裡。
   */
  function submitSearch(next: Partial<typeof search>) {
    void navigate({
      search: (prev) => ({ ...prev, compare: undefined, ...next }),
      replace: true,
    });
  }

  /**
   * Keeps the question, drops every filter — by replacing the search rather
   * than merging over it.
   *
   * Written as a replacement because the enumerated version was wrong once
   * already: it listed `script` and `validation`, submitSearch shallow-merges,
   * and so `agent` stayed on the URL while the button announced a clearing it
   * had not performed. Naming the filters here means the fourth DISC-003
   * dimension reintroduces the same defect on the day it is added, and the test
   * that enumerates three keys would not see it either (adversarial review,
   * 2026-08-24).
   */
  function clearFilters() {
    void navigate({ search: { q: search.q }, replace: true });
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    // `|| undefined` 是「回到目錄」那條路。`browsing` 的判準是 `q === undefined`，
    // 而空字串是字串，所以把搜尋框清空再按搜尋——也就是「那你就把有的都給我看」
    // 這個最自然的手勢——以前送出的是 `q=""`，伺服器對空查詢走 no_results，畫面
    // 回「沒有夠接近的 Skill」。目錄明明就在同一個位址上，而這個手勢到不了它。
    submitSearch({ q: draft.trim() || undefined });
  }

  /**
   * `replace: true`：勾一個候選不是一段歷史。二十次勾選會讓上一頁變成一條回不去的
   * 隧道，而 DISC-009 要的是「比完可以回去換一筆」，那條路要留給導覽本身。
   * 讀的是 `prev` 而不是上面算好的 `selected`：連續兩次點擊之間 navigate 是非同步的。
   */
  function toggleSelected(skillId: string) {
    void navigate({
      // 明寫型別：`navigate` 的 `prev` 是一個含 `{}` 的聯集（這一頁的 search 也可能
      // 是空的），而下一行要讀它的一個欄位。`import type` 在編譯期被抹掉，所以這不會
      // 讓 router 與這一頁形成執行期的循環相依。
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

      {/*
        No `disabled` any more. The four live dimensions used to be dead until a
        search had been run — 「先描述一次任務，才會有結果可以篩」 — because there
        was nothing on the page to narrow. There is now: they filter the
        catalogue below. A control that is live in one state of a screen and
        dead in the other, for a reason the reader has to be told, is one fewer
        thing to explain once the reason stops being true.
      */}
      <FilterBar filters={filters} onChange={(next) => submitSearch(next)} />

      {browsing && (
        <Catalog
          query={catalog}
          selected={selected}
          onToggle={toggleSelected}
          narrowing={Object.values(filters).some((value) => value !== undefined)}
        />
      )}

      {!browsing && isFetching && <p role="status">搜尋中…</p>}
      {/* DISC-001 serves this page to anyone, so a 401 here is not the ordinary
          case — but `/api/skills/search` is the one read that can answer one
          anyway, and 「搜尋失敗，請稍後再試。」 told a reader to retry something
          retrying cannot fix, while throwing away what the server said. */}
      {!browsing && <ReadFailure error={error} what="搜尋結果" />}

      {!browsing && data && (
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
          {/*
            設計系統 §4.3 wants 「共 N 筆，這裡顯示 M 筆，因為 X」. Until the server
            grew `total` (2026-08-25) this said 「超過 N 個」 — a lower bound, and a
            lower bound cannot say 共: a reader could not tell 21 from 2100 by it,
            which is most of what they wanted to know. The reason half was
            already here.
          */}
          {data.truncated && (
            <p className="notice" role="status">
              符合的 Skill 共 {data.total} 個，這裡只列出最接近的 {data.results.length} 個。
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
                // Every DISC-003 filter, because submitSearch shallow-merges over
                // the current search: a key left out of this object stays on the
                // URL. `agent` was left out, so 「清除所有篩選」 announced a
                // clearing it did not perform and the results stayed filtered
                // (M1 audit, 2026-08-24).
                onClick={clearFilters}
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
                設計 §2.2 第三向。DISC-006 的目錄補的是「搜尋找不到時，『這裡有什麼』
                沒有位址」——位址現在有了，而**最需要它的那個狀態原本仍然到不了**：
                這一頁的搜尋態總共只有三個連結，沒有一個回得去目錄，唯一的出口是
                頁首的產品標題。刻意不放進 `filtered_out`：那裡東西是在的，正確的
                建議是放寬篩選，把人送去目錄等於叫他重新開始。
              */}
              <p className="note">
                <Link to="/" search={{}}>
                  看看目錄裡有什麼
                </Link>
                ——不帶任何查詢，列出這個部署收錄的全部 Skill。
              </p>
              {/*
                IA-5's flag-off half: with generation unexposed this state had
                no exit at all — both empty states asked for another search,
                and /workspace/import's only way in was the nav bar (an
                in-page inbound count of 0, §2.3). One sentence, one Link,
                and deliberately NOT in filtered_out: there the matches
                exist and clearing filters is the right advice — an import
                link would send the user to build what the catalogue already
                has.
              */}
              <p className="note">
                {loggedIn ? (
                  <>
                    手上已經有一個 Skill 套件的話，也可以
                    <Link to="/workspace/import">直接匯入它</Link>。
                  </>
                ) : (
                  // A visitor is the emptiest case of all, and /workspace/import
                  // needs a session — offering them that link would be an exit
                  // to a page they cannot open (the shape IA-6 is about). Say
                  // what login buys instead, the way ForkAction does.
                  //
                  // 「the way ForkAction does」——而 ForkAction 當時**也**沒有帶
                  // 動作，兩處一起補。設計 §2.2 第三向：擋住人的訊息要說下一步是
                  // 什麼，而「登入後可以」在沒有登入入口的情況下不是下一步，是一句
                  // 感想。全 app 只有 SignInAction 一份登入動作（components/SignIn）。
                  <>
                    手上已經有一個 Skill 套件的話，登入後可以把它匯入你自己的工作區。{" "}
                    <SignInAction />
                  </>
                )}
              </p>
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
              {/*
                設計 §3 第 2 條「答案有被標記成答案嗎？」與第 9 條。搜尋態的整份大綱
                以前只有一行——`h1 用一句話描述你的任務`——也就是**這一頁最大的字，在
                讀者已經描述完、正在看結果的時候，還在叫他描述你的任務**；結果清單只有
                一個 `aria-label`，沒有標題。目錄那一半早就有 `h2 目錄裡有什麼`，而
                `Compare.tsx` 也已經因為同一條理由補過 `h2 逐項比較`（它的註解逐字寫著
                「the answer was on screen with no heading marking it as the answer」）。
                首頁是同一形狀的未修版本。
              */}
              <h2 id="results-heading">符合「{data.query}」的 Skill</h2>
              <p role="status" className="note">
                找到 {data.results.length} 個 Skill。
              </p>
              <MarkerLegend />
              <ul className="search-results" aria-labelledby="results-heading">
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
 * 02:DISC-006 —— 目錄，也就是「還沒問問題的人看到什麼」。
 *
 * The rows are `SearchResultRow`, unchanged and not a variant. The server sends
 * the same `PublicSearchResult` here as it does for a search — same facets, same
 * wording — because these are two states of ONE screen and 02:NFR-007 第 3 條
 * does not let one surface word a fact two ways. Every row arrives with
 * `rank: null` and the server's own `rank_note` saying what ordered the page,
 * which is the contract the degraded search path already uses.
 *
 * 比較 works from here too（the checkboxes are the same ones）: 「並排比較這兩個」
 * is exactly the question a browsing reader has, and it was previously reachable
 * only by first guessing a search that returned both.
 */
function Catalog({
  query,
  selected,
  onToggle,
  narrowing,
}: {
  query: ReturnType<typeof useCatalog>;
  selected: string[];
  onToggle: (skillId: string) => void;
  narrowing: boolean;
}) {
  if (query.isPending) return <Loading what="目錄" />;
  if (query.error) return <ReadFailure error={query.error} what="目錄" />;
  if (!query.data) return null;

  const { results, total, truncated } = query.data;
  return (
    <section aria-labelledby="catalog-heading">
      <h2 id="catalog-heading">目錄裡有什麼</h2>
      {/*
        設計 §2.1 的強形式：空狀態要說出這個「空」**不是**什麼。An empty catalogue
        and a catalogue that failed to load look identical if the empty one says
        nothing, and the two call for opposite actions.
      */}
      {results.length === 0 ? (
        <p>
          {narrowing
            ? "沒有 Skill 符合目前的篩選條件；這不是讀取失敗。清掉篩選條件可查看完整目錄。"
            : "目錄現在是空的——代表這個部署還沒有匯入任何 Skill；這不是讀取失敗。"}
        </p>
      ) : (
        <>
          {/*
            DISC-002「排序依據需可被簡要說明」，而且只講強制得了的事（§2.2）：這兩
            句各自指得出強制它的那一行——排序在 BrowseCatalogSkills 的 ORDER BY，
            「不用人氣排序」是那條 ORDER BY 裡沒有的東西，不是一句承諾。
          */}
          <p className="note">{results[0]?.rank_note ?? "未提供目錄排序說明。"}</p>
          <CompareBar selected={selected} />
          {/*
            設計 §4.3：被截斷的清單必須說出總數與截斷理由。這條路上的 total 永遠精確
            （沒有候選窗），所以它說得出「共」而不是「超過」。
          */}
          <p role="status" className="note">
            {truncated
              ? `目錄共 ${total} 個 Skill，這裡列出 ${results.length} 個。目前沒有翻頁；用上面的搜尋或篩選縮小範圍。`
              : `目錄共 ${total} 個 Skill，全部列在下面。`}
          </p>
          <MarkerLegend />
          <ul className="search-results" aria-label="目錄">
            {results.map((hit) => (
              <SearchResultRow
                key={hit.skill_id}
                hit={hit}
                checked={selected.includes(hit.skill_id)}
                atLimit={selected.length >= MAX_COMPARE}
                onToggle={onToggle}
                /* 設計 §3 第 14 條. Every row on this page carries the SAME
                   `rank_note` — one sentence, N copies, directly under a
                   sentence at the top of the list that says the same thing.
                   The server still sends it（a client rendering one row alone
                   needs it, and `rank: null` owes an explanation）; this page
                   states it once, which is the 標記說明 precedent above and
                   §0's 「數量留在外面，段落收進去」. */
                rankNoteInList={false}
              />
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

/**
 * DISC-003 (spec 02:DISC-002「使用者可依類別、來源層級、Agent、是否包含 Script、
 * 是否需要 MCP 與驗證狀態篩選」).
 *
 * Four of the six dimensions have per-row data in this build and are live
 * controls. The other two are rendered as disabled controls carrying the
 * reason, rather than being hidden or — far worse — offered as controls that
 * accept a value and narrow nothing. The server rejects those two with 400 for
 * the same reason, so a hand-edited URL cannot get an unfiltered page that looks
 * filtered.
 *
 * The wording of each reason is the honest one, not a "coming soon": MCP has no
 * source of truth anywhere in the pipeline, and 類別 is curation data the
 * platform never stored.
 *
 * Agent 相容 became live with the M2 baseline measurements (0022): 45 skills,
 * one sandbox Run each. Only its runtime axis is a filter — every measured skill
 * came back `activated`, so a capability filter would separate nothing.
 * 來源層級 became live with migration 0042, which is the first time a reviewed
 * skill was distinguishable from an unreviewed one in a column rather than in a
 * spreadsheet.
 */
const UNAVAILABLE_FILTERS: Array<{ key: string; label: string; reason: string }> = [
  {
    key: "category",
    label: "類別",
    reason: "類別目前只存在於策展清單，沒有存進平台，匯入流程也不收這個欄位，因此無法篩選。",
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
}: {
  filters: SearchFilters;
  onChange: (next: SearchFilters) => void;
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
  /*
    設計 §0 的裁定，落地。

    §0 uses this exact bar as its worked example of two correct rules breaking a
    screen between them: 原則 2.4 puts a full paragraph beside every dead
    control, and the six of them together pushed the first search result to
    1180px on a phone — 義務 1.2's 「為了拿到頭條而費力」. It also wrote the answer:
    「把六個篩選在窄螢幕收進一個 <details>，<summary> 上留「3 個條件目前無法篩選」
    ——數量留在外面，段落收進去」, because 順位低的規則讓步時，讓的是形式，順位高的
    規則的內容一個字都不能少. Nothing below is deleted or reworded.

    Measured 2026-09-03, before this: the first result sat at y1437 on a phone —
    further down than the 1180 §0 recorded, i.e. the example had got worse in
    the year nobody implemented its ruling.

    **The starting position is decided by whether a filter is ON, not by the
    viewport width**, and the second version of this is the one worth keeping.

    A width test was the obvious reading of 「窄螢幕」 and it was wrong twice over.
    It left the desktop measurably failing 義務 1.2 — measured the same day, the
    first result at y958 in a 900px window, with the bar open above it — and it
    answers a question nobody asked: what decides whether these controls are
    worth 330px of the screen is not how wide the screen is, it is whether they
    are doing anything.

    Open when any filter is set, because then the bar is not an offer, it is the
    thing removing rows from the answer — 設計 §2.2「會擋住人的東西必須在他撞上
    之前顯示」. A shared URL carrying ?tier=curated opens showing what is
    narrowing it.

    Closed otherwise, including on the catalogue state: 330px of controls and
    their six paragraphs, above the list the reader came to look at.

    After the first render it is the reader's, with one exception in one
    direction: a filter arriving later re-opens it, and nothing ever shuts it.
    The exception is not tidiness — the first version decided this on mount
    only, and a reader who followed a ?tier=curated link from an unfiltered page
    got a shut bar over a shortened list with nothing on screen saying what had
    removed the rows, which is the §2.2 failure this rule exists to prevent.
    One-way is also what §1.3 / ADR-042 決策 5 permits: a rule that can only
    expand is a default, a rule that can shut things is 「精簡模式」.

    Toggling it does not survive a reload, for the same reason.
  */
  const narrowing = Object.values(filters).some(Boolean);
  const [open, setOpen] = useState(narrowing);
  useEffect(() => {
    if (narrowing) setOpen(true);
  }, [narrowing, filters.script, filters.validation, filters.agent, filters.tier]);

  return (
    <details
      className="filter-disclosure"
      open={open}
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      {/*
        The counts are 設計 §0 的「數量留在外面」, and they are COUNTED rather than
        typed: a hand-written 「2 項」 beside two hand-written labels is a number
        that goes wrong the first time somebody adds a third, silently, which is
        the shape this whole review keeps finding. `disc.test.tsx` pins the two
        numbers to the controls actually rendered.
      */}
      <summary>
        篩選條件（{LIVE_FILTERS} 項可用、{UNAVAILABLE_FILTERS.length} 項目前無法篩選）
      </summary>
      <FilterControls filters={filters} onChange={onChange} />
    </details>
  );
}

/**
 * The number of filters that are dimensions of the platform rather than
 * placeholders. Declared beside the controls it counts, and pinned to them by a
 * test, because it is read out in the summary above.
 */
const LIVE_FILTERS = 4;

function FilterControls({
  filters,
  onChange,
}: {
  filters: SearchFilters;
  onChange: (next: SearchFilters) => void;
}) {
  return (
    <div className="filter-bar" role="group" aria-label="篩選條件">
      <label>
        是否包含 Script
        <select
          value={filters.script ?? ""}
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

      {/*
        DISC-002 來源層級, live since migration 0042 stored `skills.curation_tier`.
        The badge on each row (LabelledBadge kind="tier") is the server's own
        copy and is unchanged; this control only narrows, and its option text is
        the same two badge strings so a filter and a badge cannot disagree.

        Two options, not three: `external` means "not imported at all", so no row
        can carry it — offering it would be a control promising a page that
        cannot exist (server: curationTierValues in discovery/http.go).
      */}
      <label>
        來源層級
        <select
          value={filters.tier ?? ""}
          aria-describedby="filter-why-tier"
          onChange={(event) =>
            onChange({ tier: (event.target.value || undefined) as SearchFilters["tier"] })
          }
        >
          <option value="">不限</option>
          <option value="curated">精選</option>
          <option value="indexed">已索引</option>
        </select>
        {/*
          「已索引」 is emphatically not 「沒被審查過」: 精選 needs the nine-item
          review to have passed *and* the reviewed version to still be the newest
          one, so publishing a new version drops a curated skill back to 已索引
          on its own. Copy that said 未經人工審查 would be false for exactly those
          rows (server: discovery/detail.go, tier resolution).
        */}
        <span id="filter-why-tier" className="note">
          「精選」是這一版通過九項人工審查的 Skill；「已索引」是目前這一版沒有帶著人工審查結論——
          包含出了新版本、審查還沒跟上的那些，不等於從沒被審過。
        </span>
      </label>

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
        page: a filter bar with dead controls has to say why on the spot,
        or it reads as a broken UI instead of an absent capability.
      */}
      <p className="note">
        篩選條件只會用平台真的有的資料。上面標為「無法篩選」的兩項，是因為平台目前沒有這些資料，
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
          <span className="note">未測量——沒有擷取到依賴資訊，不等於沒有依賴。</span>
        )}
      </dd>

      <dt>風險提示</dt>
      <dd>
        <RiskSummary risk={hit.risk} />
      </dd>

      <dt>最近驗證時間</dt>
      <dd>
        {hit.verified_at ? (
          /* 設計 §3 第 14 條，而且這一處是曝光量最大的時間欄位——每一列搜尋結果
             與每一列目錄都有它。`.slice(0, 10)` 切的是伺服器的 UTC 字串前十碼：
             對 UTC+8 的讀者，任何 16:00Z 之後驗證的 Skill 都少報一天，而且 DOM 裡
             沒有 `<time dateTime>`，輔助科技拿到的只是一段散文。
             `design-system.test.ts` 的裸時間戳掃描要求 `{…_at}` 整個閉合，所以
             「切一刀」剛好從守門的門縫走過去。 */
          <Timestamp at={hit.verified_at} />
        ) : (
          <span className="note">未測量——這個 Skill 還沒有匯入內容可以驗證。</span>
        )}
      </dd>
    </dl>
  );
}

/**
 * 五個來源標記的但書，一份，兩張清單共用。
 *
 * 設計 §2.4 第 3 項: the five provenance markers on each row below
 * (AI 改寫／作者原文／來源未標示／AI 產生／規則產生) carried their qualification in
 * `title=` only, and a tooltip does not exist on a touch device. Once per list
 * rather than once per row, for the reason §0 gives: 「順位低的規則讓步時，讓的是
 * 形式」 — five extra sentences on every card would push the first result past the
 * fold that 義務 §1.2 is about, and the content is not reduced by being stated
 * once. Same shape as `SkillFiles`'s file-tree sentence and `SkillDetail`'s
 * 「AI 產生」 explanation.
 *
 * **提出來成為一個元件，是因為 2026-09-03 的目錄批只改到了搜尋那一半。** 目錄用的
 * 是同一個 `SearchResultRow`，卻沒有這一段，於是那三顆徽章的但書在**落地首頁的
 * 預設狀態**上退回成只有 `title=`——也就是手機上不存在。§0 把這一族排在順位 1
 * （安全與不誤導），所以它不能只在其中一種狀態下成立。
 */
function MarkerLegend() {
  return (
    <p className="note">
      標記說明：「AI 改寫」與「AI 產生」由模型寫成，未經人工核對——你的 Agent 讀到的是套件自己的
      description，不是這裡的改寫；「作者原文」是套件的 frontmatter
      description；「規則產生」依查詢與文件的關鍵字重疊組出；
      「來源未標示」代表伺服器沒有回報這段摘要的來源。
    </p>
  );
}

function SearchResultRow({
  hit,
  checked,
  atLimit,
  onToggle,
  rankNoteInList = true,
}: {
  hit: PublicSearchResult;
  checked: boolean;
  atLimit: boolean;
  onToggle: (skillId: string) => void;
  /**
   * False when the list already states, once, why nothing here has a
   * similarity. Only the catalogue does: on a search page the reason is
   * per-row（this one was not enriched, that one was）and has to stay per row.
   */
  rankNoteInList?: boolean;
}) {
  return (
    <li className="search-result">
      {/*
        名字在前，勾選框在後。設計 §3 第 1 條在卡片這一層的同型：「一整排控制項排在
        答案前面」——二十張卡的第一段文字以前完全一樣（「加入比較」），而 DOM 順序
        就是 Tab 順序，所以鍵盤讀者掃二十筆要先踩二十次次要功能的勾選框。這張卡的
        主要動作是「點進這一筆」，區辨力在名字上。CSS 的 `margin-right` 跟著改成
        `margin-left`。
      */}
      <Link to="/skills/$skillId" params={{ skillId: hit.skill_id }}>
        {hit.name}
      </Link>
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
      {(hit.rank !== null || rankNoteInList) && (
        <p className="rank">
          {hit.rank === null
            ? (hit.rank_note ?? "未計算語意相似度。")
            : `相似度 ${hit.rank.toFixed(2)}`}
        </p>
      )}
    </li>
  );
}
