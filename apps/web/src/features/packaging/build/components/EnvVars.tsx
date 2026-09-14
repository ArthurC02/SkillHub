import type { PackagingTarget } from "../../packaging.service";

export function EnvVars({ target }: { target: PackagingTarget }) {
  if (target.env_vars.length === 0) {
    return <p className="note">環境變數需求：這個目標不需要任何環境變數。</p>;
  }
  return (
    <details>
      <summary>環境變數需求（{target.env_vars.length}）</summary>
      <ul className="note">
        {target.env_vars.map((v) => (
          <li key={v.name}>
            <code>{v.name}</code> {v.required ? "（必要）" : "（選用）"} {v.description}
            {v.example ? (
              <>
                {" "}
                範例值：<code>{v.example}</code>
              </>
            ) : (
              ""
            )}
          </li>
        ))}
      </ul>
      <p className="note">
        這些值要由你自己在你的環境裡設定。Skill Hub 產生的套件裡不會有任何金鑰。
      </p>
    </details>
  );
}
