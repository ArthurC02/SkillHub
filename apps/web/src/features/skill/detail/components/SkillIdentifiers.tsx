import { Timestamp } from "../../../../shared/ui/Timestamp";
import type { SkillDetail } from "../../../../core/api/types";

export function SkillIdentifiers({ skill }: { skill: SkillDetail }) {
  return (
    <details>
      <summary>進階資訊（版本與識別碼）</summary>
      {skill.version ? (
        <ul>
          <li>版本編號：v{skill.version.version_number}</li>
          <li>
            版本 ID：<code>{skill.version.version_id}</code>
          </li>
          <li>
            內容雜湊：<code>{skill.version.content_hash}</code>
          </li>
          <li>
            建立時間：
            <Timestamp at={skill.version.created_at} />
          </li>
        </ul>
      ) : (
        <p>無權檢視——這個工作區看不到這個小工具的版本內容（原因見上面的〈版本〉）。</p>
      )}
      {skill.derivation.forked_from_version_id && (
        <p>
          分岔自版本：<code>{skill.derivation.forked_from_version_id}</code>
        </p>
      )}
    </details>
  );
}
