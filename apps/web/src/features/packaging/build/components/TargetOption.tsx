import type { PackagingTarget } from "../../packaging.service";
import { EnvVars } from "./EnvVars";
import { Verification } from "./Verification";

export function TargetOption({
  target,
  selected,
  onSelect,
}: {
  target: PackagingTarget;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <li className="packaging-target">
      <label>
        <input type="radio" name="packaging-target" checked={selected} onChange={onSelect} />{" "}
        <strong>{target.display_name}</strong>{" "}
        <span className="badge">
          {target.kind === "standard_package" ? "標準套件" : "安裝 Profile"}
        </span>{" "}
        <span className={`badge badge-${target.support_status}`}>
          {target.support_status === "verified" ? "已驗證" : "未驗證"}
        </span>
      </label>
      <p className="note">
        {target.support_status === "verified"
          ? "Skill Hub 實際把套件裝進這個目標跑過。"
          : "Skill Hub 沒有把套件裝進這個目標跑過，這裡不保證它裝得起來或能正常運行。"}
        {` 設定版本 ${target.version}。`}
      </p>
      <p className="note">
        安裝位置：
        {target.install_location ??
          "不指定——這個目標不指名任何 Agent，也就不假裝知道你的 Agent 把 Skill 放哪。"}
      </p>
      <EnvVars target={target} />
      {target.notes.length > 0 && (
        <details>
          <summary>已知限制與安裝時要注意的事（{target.notes.length}）</summary>
          <ul className="note">
            {target.notes.map((n) => (
              <li key={n}>{n}</li>
            ))}
          </ul>
        </details>
      )}
      <Verification target={target} />
    </li>
  );
}
