import { CompatibilityStatus } from "../../../../shared/ui/CompatibilityStatus";
import type { SkillCompatibility } from "../../../../core/api/types";
import { CompatibilityVerdict } from "./CompatibilityVerdict";

export function CompatibilitySection({ compatibility }: { compatibility: SkillCompatibility }) {
  return (
    <>
      <h2>這個版本的相容性</h2>
      <CompatibilityVerdict compatibility={compatibility} />
      <details>
        <summary>相容性細項（每一軸的備註與實測環境）</summary>
        <CompatibilityStatus compatibility={compatibility} />
      </details>
      <p className="note" data-role="caveat">
        <strong>「規格驗證通過」不等於「裝得起來」，更不等於「腳本跑得動」</strong>。
      </p>
    </>
  );
}
