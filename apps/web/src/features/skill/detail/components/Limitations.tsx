import type { SkillLimitation } from "../../../../core/api/types";

export function Limitations({ limitations }: { limitations: SkillLimitation[] }) {
  const fromModel = limitations.some((l) => l.source === "model");
  const fromScan = limitations.some((l) => l.source !== "model");

  return (
    <>
      <h3>限制</h3>
      {limitations.length === 0 ? (
        <p className="note">
          沒有任何來源指出限制——這代表沒有人說明過，不代表這個 Skill 沒有限制。
        </p>
      ) : (
        <ul className="risk-list">
          {limitations.map((limitation) => (
            <li key={`${limitation.source}-${limitation.text}`}>
              {limitation.text}
              {limitation.source === "model" ? (
                <span className="badge badge-source-model">AI 產生</span>
              ) : (
                <span className="badge badge-source-template">掃描推得</span>
              )}
            </li>
          ))}
        </ul>
      )}
      {fromModel && <p className="note">「AI 產生」的項目由模型重述套件內容，未經人工核對。</p>}
      {fromScan && (
        <p className="note">「掃描推得」的項目由匯入時的靜態掃描結果推得，掃描不執行套件內容。</p>
      )}
    </>
  );
}
