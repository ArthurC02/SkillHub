import type { CategorizedFindings, ImportFinding } from "../api/import";

export function Findings({
  findings,
  level = 3,
}: {
  findings: CategorizedFindings;
  level?: 3 | 4;
}) {
  const GroupHeading = `h${level}` as "h3" | "h4";
  const groups = [
    ["阻擋錯誤", findings.errors],
    ["警告", findings.warnings],
    ["資訊", findings.infos],
  ] as const;

  if (groups.every(([, items]) => items.length === 0)) {
    return (
      <p className="note">
        靜態檢查跑完了，沒有任何發現——這是「掃過了，沒掃到」，不是「沒掃」。它讀套件內容、不執行其中的
        Script，<strong>既不是人工審查，也不是簽章驗證</strong>
        ：MVP 的套件不帶數位簽章，平台也不驗簽（ADR-027 決策 3
        是明文的「不做」），所以簽章這一項不是還沒驗，是這裡永遠不會有人替你驗。
      </p>
    );
  }

  return (
    <div>
      {groups.map(([label, items]) =>
        items.length ? (
          <section key={label}>
            <GroupHeading>
              {label}（{items.length}）
            </GroupHeading>
            <ul className="finding-list">
              {items.map((finding, index) => (
                <li className="criterion" key={`${finding.code}-${index}`}>
                  <FindingBody finding={finding} />
                </li>
              ))}
            </ul>
          </section>
        ) : null,
      )}
    </div>
  );
}

function FindingBody({ finding }: { finding: ImportFinding }) {
  return (
    <>
      <p>
        {finding.message}（<span className="risk-code">{finding.code}</span>）
        {finding.path && (
          <>
            {" "}
            <code>{finding.path}</code>
          </>
        )}
      </p>
      {finding.details && finding.details.length > 0 && (
        <details>
          <summary>逐項列出（{finding.details.length} 筆）</summary>
          <ul className="risk-list">
            {finding.details.map((detail, index) => (
              <li key={`${detail}-${index}`}>{detail}</li>
            ))}
          </ul>
        </details>
      )}
    </>
  );
}
