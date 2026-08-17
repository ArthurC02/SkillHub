import { Link } from "@tanstack/react-router";
// GET /skills already has a consumer: the Test Case screen's skill picker. One
// query for one endpoint, wherever it was first needed (WS-004).
import { useOwnSkills } from "../api/testcases";

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
 */
export function WorkspaceSkills() {
  const skills = useOwnSkills();

  return (
    <section className="page">
      <h1>我的 Skill</h1>
      <p className="note">
        這個工作區裡的 Skill：Fork 進來的、自己匯入的都在這裡，新的在上面。 公開目錄裡的 Skill
        不會出現在這裡，除非你 Fork 過它。
      </p>

      {skills.isPending && <p>載入中…</p>}
      {skills.error && <p role="alert">無法讀取你的 Skill 清單：{skills.error.message}</p>}

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
      </ul>
    </section>
  );
}
