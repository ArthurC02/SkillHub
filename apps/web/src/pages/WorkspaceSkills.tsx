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
                  <ConfirmDelete
                    scopeId={`skill-delete-scope-${s.skill_id}`}
                    pending={remove.isPending}
                    onAsk={() => setMessage("")}
                    onConfirm={() => remove.mutate(s.skill_id)}
                    scope={
                      <>
                        刪除的是這個 Skill
                        在你工作區裡的存在：它會離開這份清單與搜尋結果，也不能再拿來試跑或打包。
                        版本快照會凍結保留 30 天再清除，所以誤刪在那段期間內還有救； 別人 Fork
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
