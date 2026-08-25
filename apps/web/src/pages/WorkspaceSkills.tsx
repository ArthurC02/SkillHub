import { Loading } from "../components/Loading";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { deleteSkill } from "../api/skills";
// GET /skills already has a consumer: the Test Case screen's skill picker. One
// query for one endpoint, wherever it was first needed (WS-004).
import { useOwnSkills } from "../api/testcases";
import { ConfirmDelete } from "../components/ConfirmDelete";
import { useGenerateEntryPoint } from "../api/generate";
import { GenerateSkill } from "../components/GenerateSkill";
import { GeneratedNotice } from "../components/GeneratedNotice";
import { RiskSummary } from "../components/RiskIndicator";
import type { Redistribution } from "../api/types";

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
const REDISTRIBUTION_BADGE: Record<Redistribution, { text: string; danger?: true }> = {
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

  const remove = useMutation({
    mutationFn: deleteSkill,
    // The server's `note` is the authoritative scope, so it is shown verbatim
    // rather than restated — the sentence before the deletion is this side's
    // job, the sentence after it is the server's.
    onSuccess: async (result) => {
      setMessage(`已刪除。${result.note}`);
      await client.invalidateQueries({ queryKey: ["own-skills"] });
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "刪除失敗。"),
  });

  return (
    <section className="page">
      <h1>我的 Skill</h1>
      <p className="note">
        這個工作區裡的 Skill：Fork 進來的、自己匯入的都在這裡，新的在上面。 公開目錄裡的 Skill
        不會出現在這裡，除非你 Fork 過它。
      </p>

      {/*
        GEN-004's second entry point (the first is the search's no-results
        state). Behind ADR-052's flag, read from /me: off by default, and off is
        what every beta deployment is in until 01 §11.2's first funnel segment
        has a reading.
      */}
      {generateExposed && <GenerateSkill />}

      {skills.isPending && <Loading what="你的 Skill 清單" />}
      {skills.error && <p role="alert">無法讀取你的 Skill 清單：{skills.error.message}</p>}
      {message && <p role="status">{message}</p>}

      {skills.data &&
        (skills.data.skills.length === 0 ? (
          <p>
            還沒有任何 Skill。到<Link to="/">首頁</Link>
            搜尋一個再 Fork，或匯入自己的套件——這裡是空的代表你還沒有建立過，不是清單讀取失敗。
          </p>
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
                </p>
                {/*
                  §1.1: this is a list of code you own and will run, and until
                  2026-08-22 it carried nothing to decide by (04 丙-31). The same
                  component the public search row uses, on purpose — the two are
                  the same fact about the same skill, and 02:NFR-007 第 3 條 does
                  not let them be worded independently.
                */}
                <p className="badge-row">
                  <RiskSummary risk={s.risk} />
                </p>
                {/*
                  §2.9. The state, not a timestamp: a fork's newest version row
                  was created the instant somebody pressed Fork, so the field
                  that reads as 「剛剛掃過」 belongs to the one case where nothing
                  was scanned. Label and note both come from the server (§4.4),
                  which is why there is no enum→中文 map on this side.
                */}
                <p className="badge-row">
                  <span
                    className={
                      s.verification.value === "scanned" ? "badge" : "badge badge-unverified"
                    }
                  >
                    掃描狀態：{s.verification.label}
                    {s.verification.scanned_at
                      ? `（${s.verification.scanned_at.slice(0, 10)}）`
                      : ""}
                  </span>
                </p>
                <p className="note">{s.verification.note}</p>
                {/*
                  GEN-004: two named absences on the list as well as on the
                  detail page, because this list is a generated skill's only
                  entry point — it is not in the catalogue and not in search,
                  including the owner's own (GEN-007). If the sentence lived only
                  on the detail page, the one screen it can be met on would not
                  say it.
                */}
                {s.redistribution === "generated" && <GeneratedNotice skillId={s.skill_id} />}
                {/*
                  Still true and still needed, narrowed to what is genuinely not
                  here. Compatibility is not a contract gap: nothing in the
                  product writes `skill_runtime_compatibility` at all — only the
                  catalogue seeding tool does — so a compatibility column on this
                  list could say 未測量 and nothing else, which is a facet that
                  looks like evidence and can never carry any. **Do not delete
                  this sentence to make room for one.**
                */}
                <p className="note">
                  相容性驗證（Agent 是否載入、Runtime 是否齊備）不在這份清單的資料裡，
                  平台目前也不會為你自己的 Skill 量測它。要看逐項掃描結果，請開這個 Skill 的頁面。
                </p>
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
                    onAsk={() => setMessage("")}
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
          這個工作區的 Skill 超過 {skills.data.limit} 個，上面只列出前 {skills.data.limit} 個。
          目前沒有翻頁，其餘的要用搜尋找。
        </p>
      )}

      <h2>這個工作區的其他清單</h2>
      <ul className="risk-list">
        <li>
          <Link to="/lab/test-cases">Test Case</Link>：這個工作區的 Test Case 與驗收條件。
        </li>
        <li>
          <Link to="/workspace/downloads">下載紀錄</Link>：打包過的套件，含已到期的。
        </li>
        <li>
          <Link to="/workspace/runs">Run 歷史</Link>：這個工作區跑過的 Run，含執行狀態與清理狀態。
        </li>
        <li>
          <Link to="/workspace/account">帳號</Link>
          ：刪除整個帳號，以及刪除之後哪些東西會留下。逐項的保存期限見
          <Link to="/policy">資料保存政策</Link>。
        </li>
      </ul>
    </section>
  );
}
