import type { PackagingTarget } from "../../packaging.service";

export function Verification({ target }: { target: PackagingTarget }) {
  const steps = target.verification_steps ?? [];
  const count = steps.length + (target.verification_prompt ? 1 : 0);
  if (count === 0) {
    return <p className="note">這個目標沒有提供安裝後的驗證方式。</p>;
  }
  return (
    <details>
      <summary>裝好之後怎麼確認（{count}）</summary>
      {target.verification_prompt && (
        <p className="note">
          對你的 Agent 下這個 Prompt：<q>{target.verification_prompt}</q>
        </p>
      )}
      {steps.length > 0 && (
        <ol className="note">
          {steps.map((s) => (
            <li key={s}>{s}</li>
          ))}
        </ol>
      )}
    </details>
  );
}
