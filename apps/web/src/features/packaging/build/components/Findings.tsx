import type { PackageValidation } from "../../packaging.service";
import type { Finding } from "../../../../core/api/types";

export function Findings({ validation }: { validation: PackageValidation }) {
  const groups: Array<{ key: string; label: string; items: Finding[] }> = [
    { key: "errors", label: "阻擋級錯誤", items: validation.errors },
    { key: "warnings", label: "警告（不阻擋，但要知道）", items: validation.warnings },
    { key: "infos", label: "資訊", items: validation.infos },
  ];
  const total = groups.reduce((n, g) => n + g.items.length, 0);

  return (
    <>
      <h3>打包後的規格驗證</h3>
      <p className="risk-counts">
        阻擋級錯誤 {validation.errors.length} 項／警告 {validation.warnings.length} 項／資訊{" "}
        {validation.infos.length} 項
      </p>
      {total === 0 ? (
        <p className="note">
          這次重驗沒有產生任何發現。這是「掃過了，沒掃到」，不是「沒掃」——它讀套件內容、
          不執行其中的 Script，既不是人工審查，也不是簽章驗證。簽章這一項不是還沒驗，
          是這裡永遠不會有人替你驗。
        </p>
      ) : (
        groups
          .filter((g) => g.items.length > 0)
          .map((g) => (
            <div key={g.key}>
              <h4>
                {g.label}（{g.items.length}）
              </h4>
              <ul className="risk-list">
                {g.items.map((f, i) => (
                  <li key={`${f.code}-${i}`}>
                    {f.message}（<span className="risk-code">{f.code}</span>）
                    {f.path && (
                      <>
                        {" "}
                        <code>{f.path}</code>
                      </>
                    )}
                    {f.details && f.details.length > 0 && (
                      <ul className="note">
                        {f.details.map((d) => (
                          <li key={d}>{d}</li>
                        ))}
                      </ul>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))
      )}
    </>
  );
}
