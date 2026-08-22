import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { deleteSkill } from "../api/skills";
// GET /skills already has a consumer: the Test Case screen's skill picker. One
// query for one endpoint, wherever it was first needed (WS-004).
import { useOwnSkills } from "../api/testcases";
import { ConfirmDelete } from "../components/ConfirmDelete";

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

      {skills.isPending && <p>載入中…</p>}
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
                  {s.redistribution === "allowed" ? (
                    <span className="badge">可打包下載</span>
                  ) : (
                    <span className="badge badge-danger">
                      {s.redistribution === "blocked" ? "不可散布" : "授權未知，不能打包"}
                    </span>
                  )}
                  {s.access_restriction && (
                    <span className="badge badge-danger">授權保留：{s.access_restriction}</span>
                  )}
                  {s.forked_from_skill_id ? (
                    <span className="badge">Fork 自其他 Skill</span>
                  ) : (
                    <span className="badge">自己匯入</span>
                  )}
                </p>
                {/*
                  §1.1 / §2.1. The two facets above are the ones GET /skills can
                  answer today; risk, compatibility and verification are on
                  `PublicSearchResult` and not on `Skill`, so they cannot be
                  rendered here without a contract change (04 丙-31). The absence
                  is stated rather than left blank — a list of code you own and
                  will run, with nothing on it to decide by, reads as approved.
                  **Narrow this sentence as facets land; do not delete it.** A
                  disclaimer that has gone false beside real evidence is worse
                  than the disclaimer alone.
                */}
                <p className="note">
                  風險掃描結果與相容性驗證不在這份清單的資料裡。這裡沒有它們不代表通過——要看那些，請開這個
                  Skill 的頁面。
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
                        版本快照會先凍結保留一段期間再清除，所以誤刪在那之前還有救； 別人 Fork
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
