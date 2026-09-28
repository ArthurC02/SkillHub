import { Timestamp } from "../../../../shared/ui/Timestamp";
import type { SkillSource } from "../../../../core/api/types";

export function SourceIdentifiersDetails({ source }: { source: SkillSource }) {
  const showLastChecked = !source.unavailable_since && source.last_checked_at;
  if (!source.source_version && !source.content_hash && !showLastChecked) return null;

  return (
    <details>
      <summary>識別碼</summary>
      <ul>
        {showLastChecked && (
          <li>
            最近一次來源可用性檢查：
            <Timestamp at={source.last_checked_at!} />
            （當時可取得）
          </li>
        )}
        {source.source_version && (
          <li>
            來源版本／Commit：<code>{source.source_version}</code>
          </li>
        )}
        {source.content_hash && (
          <li>
            內容雜湊：<code>{source.content_hash}</code>
          </li>
        )}
      </ul>
    </details>
  );
}
