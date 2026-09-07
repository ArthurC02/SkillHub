import { Loading } from "../components/Loading";
import { ReadFailure } from "../components/LoginRequired";
import { ApiError } from "../api/client";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Timestamp } from "../components/Timestamp";
import { deleteSkill } from "../api/skills";
// GET /skills already has a consumer: the Test Case screen's skill picker. One
// query for one endpoint, wherever it was first needed (WS-004).
import { useOwnSkills } from "../api/testcases";
import { ConfirmDelete } from "../components/ConfirmDelete";
import { useGenerateEntryPoint } from "../api/generate";
import { useCreationEntryPoint } from "../api/creation";
import { CreateHub } from "../components/CreateHub";
import { GeneratedNotice } from "../components/GeneratedNotice";
import { RiskSummary } from "../components/RiskIndicator";
import { FacetNotes, liftedNotes, type FacetNote } from "../components/FacetNotes";
import type { OwnSkill, Redistribution } from "../api/types";

/**
 * 這份清單上，哪幾句但書可以只講一次。規則與它的兩個分支在 `components/FacetNotes`；
 * 這裡只宣告這一頁有哪些 facet，以及每一列身上有沒有一個對得回那一句的詞。
 *
 * 為什麼這一頁需要它（2026-09-07，實測）：把 `GET /skills` 換成 8 列去渲染，整頁高
 * 5045px，而**每一列都印了同樣兩句話**——
 *
 *   風險：「來自匯入時的靜態掃描，不執行套件內任何程式碼；開啟 Skill 可看逐項結果。」
 *   驗證：「匯入這個版本時做過靜態掃描，不執行套件內任何程式碼；逐項結果在 Skill 頁面。」
 *
 * 三個宣稱相同、措辭不同，而且兩句各印 8 次。設計 §2.13 第 1 條：一句在整份清單上
 * 逐字相同的話不是關於某一列的事實，是關於這份清單的事實。這一頁**已經對相容性那句
 * 做過同樣的處置**（搬到清單層級印一次，見下方那段 `.note`），只是當時沒把這兩句
 * 一起帶走。
 *
 * `risk` 沒有 `by`：`scan_status: "unavailable"` 的那一列身上一個標記都沒有（與
 * `pages/Home.tsx` 同一個理由，逐字寫在 FacetNotes 的檔頭），所以它只有在整份清單
 * 逐字相同時才搬。`verification` 有 `by`，因為每一列都戴著「掃描狀態：<label>」那顆
 * 徽章——但那個 label 不是 note 的函數（兩列都可能是「已掃描（來源）」而句子裡嵌著
 * 不同的來源 Skill 名稱），所以 FacetNotes 的「一個詞只能對應一句話」那道守衛就是
 * 為這個呼叫端加的：撞到就整個不搬，句子留在列上。
 */
const OWN_SKILL_NOTES: Array<FacetNote<OwnSkill>> = [
  { key: "risk", label: "風險提示", note: (s) => s.risk?.note },
  {
    key: "verification",
    label: "掃描狀態",
    note: (s) => s.verification?.note,
    by: (s) => s.verification.label,
  },
];

/**
 * One sentence per redistribution value, and never one sentence for two of them:
 * the three that release do not make the same promise. `allowed` means somebody
 * checked the licence, `self_supplied` means you brought it in and the platform
 * is only handing it back (ADR-045), `generated` means the platform wrote it and
 * nobody has answered who owns that (0037, ADR-047 決策 4). Collapsing any of
 * them into 可打包下載 would tell a user something nobody established.
 *
 * Keyed by the union for the reason Packaging.tsx REDISTRIBUTION_GATE is: this
 * row used a ternary chain, `generated` was not in it, and every generated skill
 * rendered a red 授權未知，不能打包 next to a download the server would have
 * produced. A missing key is now a compile error rather than a wrong verdict.
 */
export const REDISTRIBUTION_BADGE: Record<Redistribution, { text: string; danger?: true }> = {
  allowed: { text: "可打包下載" },
  self_supplied: { text: "可下載（你自己帶進來的）" },
  generated: { text: "可下載（平台為你生成的）" },
  blocked: { text: "不可散布", danger: true },
  unknown: { text: "授權未知，不能打包", danger: true },
};

/**
 * `value` stays `string`: the union is what this file maintains, the wire is not
 * bound by it. A value with no row falls back to the refusing sentence, which is
 * the same direction every other copy of this gate fails in.
 */
function RedistributionBadge({ value }: { value: string }) {
  const badge = Object.prototype.hasOwnProperty.call(REDISTRIBUTION_BADGE, value)
    ? REDISTRIBUTION_BADGE[value as Redistribution]
    : REDISTRIBUTION_BADGE.unknown;
  return <span className={badge.danger ? "badge badge-danger" : "badge"}>{badge.text}</span>;
}

/**
 * 02:WS-002 第 1 條「使用者可查看自己的 Fork、版本、Test Case、Run 歷史與下載紀錄」
 * — the Fork half. GET /skills has existed since M1 and had no screen, which is
 * the 尺-1 shape 04 乙-7 rules on: the criterion's subject is 使用者可查看, so an
 * endpoint without a surface has not met it.
 *
 * What this page deliberately does NOT invent: **版本列表**. GET /skills answers
 * one row per skill, and the version history of one skill is reachable only
 * through its detail page and the diff route — so this page links there rather
 * than showing a version count nobody computed.
 *
 * It is also the only place a skill can be deleted (WS-005, 04 丙-22). The
 * endpoint has existed since M2 with no button on it, which is the same 尺-1
 * shape as the list itself: 使用者可刪除 is not met by a route nobody can reach.
 * Here rather than on the detail page because the detail page is the public
 * catalogue view — a reader looking at somebody else's skill and the owner
 * looking at their own see the same screen, and only one of them may delete it.
 */
export function WorkspaceSkills() {
  const skills = useOwnSkills();
  const client = useQueryClient();
  const [message, setMessage] = useState("");
  const generateExposed = useGenerateEntryPoint();
  const creationExposed = useCreationEntryPoint();
  const rows = skills.data?.skills ?? [];
  const hasSkills = rows.length > 0;
  const lifted = liftedNotes(rows, OWN_SKILL_NOTES);

  const remove = useMutation({
    mutationFn: deleteSkill,
    // The server's `note` is the authoritative scope, so it is shown verbatim
    // rather than restated — the sentence before the deletion is this side's
    // job, the sentence after it is the server's.
    onSuccess: async (result) => {
      setMessage(`已刪除。${result.note}`);
      await client.invalidateQueries({ queryKey: ["own-skills"] });
    },
    // 丙-150: keep the error object rather than `err.message`; ReadFailure
    // decides the sentence by status (401 → login, otherwise this page's own).
    onError: () => {},
  });

  return (
    <section>
      <h1>我的 Skill</h1>
      {/* §2.13,D 類:兩句講的是同一件事的正反面,而反面（公開目錄的不在）才是讀者
          會弄錯的那半。合成一句,沒有任何新的宣稱。 */}
      <p className="note" data-role="teaching">
        Fork 與匯入的都在這裡；公開目錄的不在。
      </p>

      {/*
        建立一個 Skill — the three ways in, gathered above the list instead of
        scattered across the nav, a 2845px detail page and a flag.

        The flag is READ HERE and passed down. GEN-004's second entry point (the
        first is the search's no-results state) is one of the cards, still behind
        ADR-052's flag from /me: off by default, and off is what every beta
        deployment is in until 01 §11.2's first funnel segment has a reading.
        `ia.test.ts`'s FLAG_OFF_ASSERTED names THIS file as the mount, and that
        roster may only get shorter — so the read stays where the roster and its
        flag-off test can find it.
      */}
      {/*
        空的時候，「建立一個 Skill」**就是**這一頁的答案，所以它排在最前面——那也是
        這個區塊 2026-09-03 落地時的位置，對空工作區從來沒有錯過。

        有東西的時候它排在清單之後（見下方同一個元件的第二個掛載點）。理由是
        §3 checklist 第 1 條「頭條在第一屏，而且是第一屏裡的第一個東西嗎」，它的
        「不過的樣子」逐字寫著**「一整排控制項排在答案前面」**——而 2026-09-07 量到
        的正是那個形狀：`h1` 叫「我的 Skill」，然後三張卡（含一張約 1000px 的表單），
        清單在第 1500px 才開始。帶著 8 個 Skill 的人來這一頁不是為了再建一個。

        兩個掛載點而不是一個帶 CSS `order` 的：`order` 只改視覺順序不改 DOM 順序，
        鍵盤與朗讀會走到與眼睛不同的地方（§1.1 的「可判斷」對這兩者是同一件事）。
      */}
      {!hasSkills && (
        <CreateHub generateExposed={generateExposed} creationExposed={creationExposed} />
      )}

      {skills.isPending && <Loading what="你的 Skill 清單" />}
      <ReadFailure error={skills.error} what="你的 Skill 清單" />
      {message && <p role="status">{message}</p>}
      {/* 丙-150: success and failure never share one element — success stays
          the `role="status"` message above, failure is its own alert here. */}
      {remove.error && (
        <ReadFailure error={remove.error} what="刪除 Skill">
          <p role="alert">
            {remove.error instanceof ApiError && remove.error.status === 404
              ? "這個 Skill 已經不在了。"
              : "沒有刪成，可以再按一次。"}
          </p>
        </ReadFailure>
      )}

      {/*
        Still true and still needed, narrowed to what is genuinely not here.
        Compatibility is not a contract gap: nothing in the product writes
        `skill_runtime_compatibility` at all — only the catalogue seeding tool
        does — so a compatibility column on this list could say 未測量 and nothing
        else, which is a facet that looks like evidence and can never carry any.
        **Do not delete this sentence to make room for one.**

        設計 §2.13 第 1 條 (2026-09-03): it used to print on EVERY row, byte for
        byte. A sentence that is identical on 100 rows is not a fact about a row,
        it is a fact about the list — from row 2 on, no reader can tell two rows
        apart by it. So it moved up here and is printed once. It is still flat
        text, still visible with no interaction: §2.9's typed absence does not
        care how many times it is repeated, only that it is legible without
        opening anything.
      */}
      {hasSkills && (
        <p className="note">
          相容性驗證（Agent 是否載入、Runtime 是否齊備）不在這份清單的資料裡，
          平台目前也不會為你自己的 Skill 量測它。要看某一個的逐項掃描結果，請開它的頁面。
        </p>
      )}
      {/*
        上面那一段的第二批。2026-09-03 把相容性那句搬到這裡時，理由是「一句在 100 列
        上逐字相同的話是關於這份清單的事實」；同一個判準對風險與掃描那兩句同樣成立，
        只是當時沒一起帶走。搬得動與搬不動由 `OWN_SKILL_NOTES` 與 FacetNotes 的兩個
        分支決定，不由這裡決定——搬不動的那一句會留在它的那一列上。
      */}
      {hasSkills && <FacetNotes rows={rows} facets={OWN_SKILL_NOTES} />}

      {skills.data &&
        (skills.data.skills.length === 0 ? (
          /*
            IA-9 / 資訊架構 §0.1 R3: the second in-page inbound edge for
            /workspace/import. The sentence already named importing and only
            prose carried it — the page said what to do next and then made you
            go find the nav to do it, which is the shape R3 calls one way in.
            Here rather than anywhere else on this page because an empty
            personal list IS the moment: nothing to Fork from, nothing to run.

            No visitor branch, unlike Home.tsx's no_results exit: GET /skills is
            RequireSession, so `skills.data` cannot exist without a session and
            this state is unreachable for anyone the link would 401. The nav's
            copy of the same link has no such guarantee — that is IA-6, and it
            is not this edge's to answer.

            ── 2026-09-07：兩條路的連結走了，型別詞留下 ──────────────────────────
            這一句是 2026-08-25 補的（IA-9），當時它是這一頁唯一講「下一步」的地方。
            9/3 之後不是了：這個分支只在 `!hasSkills` 時渲染，而**同一個條件下
            `CreateHub` 就掛在它正上方**，兩張卡逐字提供同樣的兩條路（「匯入 Skill」
            連 /workspace/import、「到目錄挑一個」連 /）。於是這一段變成第二份導覽，
            而且排在建立表單之後——外部評閱把它讀成「我到底是在填表還是在看清單」。

            留下的是它現在唯一在做的事：§2.9 的缺席型別詞。「空」在這一頁有兩種可能的
            意思（沒建立過／讀取失敗），而讀取失敗有自己的 `ReadFailure`，所以這一句
            要說的是它**不是**那一種。§2.10 第 10 項不准折疊的正是這半句，它留在外面。

            IA-9 的性質沒有變：`/workspace/import` 的頁內入邊仍是兩個來源檔
            （`pages/Home.tsx` 的 `no_results`、`components/CreateHub.tsx`），
            `ia.test.ts` 以來源檔計數且只斷言 0 與 1 那兩列。資訊架構 §5 IA-9 與
            §2.3 的邊圖同批更新。
          */
          <p>還沒有任何 Skill——這裡是空的代表你還沒有建立過，不是清單讀取失敗。</p>
        ) : (
          <ul className="search-results">
            {skills.data.skills.map((s) => (
              <li key={s.skill_id} className="search-result">
                <p>
                  <Link to="/skills/$skillId" params={{ skillId: s.skill_id }}>
                    <strong>{s.name}</strong>
                  </Link>
                </p>
                <p>{s.summary}</p>
                {/*
                  §2.2 in its second direction: two of the four locks that refuse
                  a download live on this row and were being dropped in
                  serialisation (04 丙-31). `unknown` is what a user's own import
                  carries by default, so without this the skill you cannot take
                  away looked exactly like the one you can, right up to the
                  packaging screen.
                */}
                <p className="badge-row">
                  <RedistributionBadge value={s.redistribution} />
                  {s.access_restriction && (
                    <span className="badge badge-danger">授權保留：{s.access_restriction}</span>
                  )}
                  {/*
                    Linked, not just stated: when the scan below is the source's
                    (ADR-042 決策 6) the attribution is only useful if the reader
                    can go and look at what it was attributed from. The id has
                    been on the row since 丙-31; it was rendering as a sentence.
                  */}
                  {s.forked_from_skill_id ? (
                    <span className="badge">
                      Fork 自
                      <Link to="/skills/$skillId" params={{ skillId: s.forked_from_skill_id }}>
                        來源 Skill
                      </Link>
                    </span>
                  ) : (
                    <span className="badge">自己匯入</span>
                  )}
                  {/*
                    §2.9. The state, not a timestamp: a fork's newest version row
                    was created the instant somebody pressed Fork, so the field
                    that reads as 「剛剛掃過」 belongs to the one case where nothing
                    was scanned. Label and note both come from the server (§4.4),
                    which is why there is no enum→中文 map on this side.
                  */}
                  <span
                    className={
                      s.verification.value === "scanned" ? "badge" : "badge badge-unverified"
                    }
                  >
                    掃描狀態：{s.verification.label}
                    {s.verification.scanned_at && (
                      <>
                        （<Timestamp at={s.verification.scanned_at} />）
                      </>
                    )}
                  </span>
                  {/*
                    §1.1: this is a list of code you own and will run, and until
                    2026-08-22 it carried nothing to decide by (04 丙-31). The same
                    component the public search row uses, on purpose — the two are
                    the same fact about the same skill, and 02:NFR-007 第 3 條 does
                    not let them be worded independently.
                  */}
                  <RiskSummary risk={s.risk} noteInRow={!lifted.risk} />
                </p>
                {/*
                  2026-09-07：三條 `.badge-row` 併成一條。它們本來是三個 `<p>`，於是
                  一列的可判斷事實佔了三行、中間各隔一次 `margin: 8px 0`——而 `.badge-row`
                  本來就是 `flex-wrap: wrap`，一條就裝得下，窄螢幕自己折。

                  §2.10 的十項一項都沒有被折疊：可散布性、授權保留、來源、掃描狀態、
                  風險揭露全部還是平鋪的徽章，不互動就看得到。被搬走的是**每一列都相同
                  的那句理由**，不是每一列不同的那個狀態（§2.13 的判準逐字是「會變的量
                  永遠平鋪，不會變的理由才可以折」）。
                */}
                {!lifted.verification && <p className="note">{s.verification.note}</p>}
                {/*
                  GEN-004: two named absences on the list as well as on the
                  detail page, because this list is a generated skill's only
                  entry point — it is not in the catalogue and not in search,
                  including the owner's own (GEN-007). If the sentence lived only
                  on the detail page, the one screen it can be met on would not
                  say it.
                */}
                {s.redistribution === "generated" && <GeneratedNotice skillId={s.skill_id} />}
                <p className="note">
                  <Link to="/skills/$skillId/files" params={{ skillId: s.skill_id }}>
                    檔案
                  </Link>
                  {" ｜ "}
                  {/*
                    No version to pass: this list is one row per skill. The
                    packaging page resolves the skill's latest version itself,
                    which is what `version` being optional is for.
                  */}
                  <Link
                    to="/skills/$skillId/package"
                    params={{ skillId: s.skill_id }}
                    search={{ version: undefined }}
                  >
                    打包與下載
                  </Link>
                  {" ｜ "}
                  {/*
                    核心第 3 點的另一半。這一列以前只有「散布」，沒有「先試一次」：
                    要試跑自己的 Skill，得先點進 /skills/$id，再從那裡的「試跑」區走。
                    頁尾的「這個工作區的其他清單」確實有一條 /lab/test-cases，但那是
                    **未篩選的全清單**，不帶這一列的 Skill。同一條連結 SkillDetail
                    已經有了（`TrialEntry`），這裡複製它而不是發明第二種形狀。
                  */}
                  <Link to="/lab/test-cases" search={{ skill: s.skill_id }}>
                    Test Case
                  </Link>
                </p>
                {/*
                  checklist 8: 刪除 used to sit in the same inline run as the two
                  navigation links, separated by a ｜ glyph — and in the
                  confirming state that one `<p>` also grew two buttons and a
                  ~100-character scope sentence. At 375px that is an
                  undifferentiated run of prose and controls. A destruction is
                  not a third link, so it gets its own row; no label is removed.
                */}
                <p>
                  <ConfirmDelete
                    scopeId={`skill-delete-scope-${s.skill_id}`}
                    pending={remove.isPending}
                    onAsk={() => {
                      setMessage("");
                      remove.reset();
                    }}
                    onConfirm={() => remove.mutate(s.skill_id)}
                    scope={
                      <>
                        刪除的是這個 Skill
                        在你工作區裡的存在：它會離開這份清單與搜尋結果，也不能再拿來試跑或打包。
                        {/*
                          §2.2: the numeral was 30 天 here, and PDM-006 — the
                          only thing that would ratify it — is unratified. The
                          same figure was removed from /policy and
                          /workspace/account for that reason, which left this
                          page as the last place the claim shipped from. The
                          server's own `note`, shown verbatim after the delete,
                          is where a grace period gets stated by something that
                          enforces it.
                        */}
                        版本快照會凍結保留，不隨這次刪除消失，所以誤刪還有救； 別人 Fork
                        過的版本與歷史 Run 引用的內容不受影響——那是他們的溯源鏈，不是你的。
                        已經打包好的下載檔案要另外刪，在下載紀錄那一頁。
                      </>
                    }
                  />
                </p>
              </li>
            ))}
          </ul>
        ))}

      {/*
        §2.2: the cap has always been there — the handler asked for 100 rows and
        skill 101 did not exist as far as this page was concerned. A limit the
        platform enforces and the page cannot see is the same defect as a limit
        the page shows and the platform does not, read from the other end. There
        is no pagination yet, which is why this says so rather than offering a
        next page it does not have.
      */}
      {skills.data?.truncated && (
        <p className="notice" role="status">
          這個工作區的 Skill 共 {skills.data.total} 個，上面只列出前 {skills.data.limit} 個。
          目前沒有翻頁，其餘的要用搜尋找。
        </p>
      )}

      {/*
        設計 §2.13 第 2 條 — 「東西要去哪裡刪」在這個 app 裡有四份地圖,措辭全不同:
        這裡、/workspace/account、/policy 與頁尾。留 /policy 那一份,因為它是
        02:O11Y-004 的揭露義務所在,也是四份裡唯一寫了「刪不掉的東西會留下什麼」的;
        其餘三處指過去。連結本身一條都沒有少（`ia.test.ts` 的 §2.3 可達性表數的是
        連結,不是句子）,少掉的是四句各自複述一次「那一頁裝什麼」的說明。
      */}
      {/*
        第二個掛載點，理由與第一個是同一段（見上方 `!hasSkills` 那裡）：答案先出來，
        動作在後面。**這也讓生成入口變得更不顯眼而不是更顯眼**——對已經有 Skill 的
        人，它從第一屏移到清單之後，而 `01` §10 邊界 1 要的正是這個方向。旗標的讀取
        仍然在這個檔案裡，`ia.test.ts` 的 FLAG_OFF_ASSERTED 名冊一個字都不用動。
      */}
      {hasSkills && (
        <CreateHub generateExposed={generateExposed} creationExposed={creationExposed} />
      )}

      <h2>這個工作區的其他清單</h2>
      <ul className="risk-list">
        <li>
          <Link to="/lab/test-cases">Test Case</Link>
        </li>
        <li>
          <Link to="/workspace/downloads">下載紀錄</Link>
        </li>
        <li>
          <Link to="/workspace/runs">Run 歷史</Link>
        </li>
        <li>
          <Link to="/workspace/account">帳號</Link>
        </li>
      </ul>
      <p className="note" data-role="teaching">
        要刪掉哪一樣東西、以及刪掉之後什麼會留下，一份寫在
        <Link to="/policy">資料保存政策</Link>。
      </p>
    </section>
  );
}
