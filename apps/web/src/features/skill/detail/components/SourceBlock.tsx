import { Timestamp } from "../../../../shared/ui/Timestamp";
import type { SkillSource } from "../../../../core/api/types";
import { GeneratedSourceBlock } from "./GeneratedSourceBlock";

export function SourceBlock({ source }: { source: SkillSource }) {
  if (source.type === "generated") {
    return <GeneratedSourceBlock source={source} />;
  }
  return (
    <>
      <p>匯入方式：{source.type === "git" ? "從 Git 來源擷取" : "使用者上傳"}</p>
      {source.url && (
        <p>
          來源網址：{" "}
          <a href={source.url} rel="noreferrer noopener">
            {source.url}
          </a>
        </p>
      )}
      {source.fetched_at && (
        <p>
          擷取時間：
          <Timestamp at={source.fetched_at} />
        </p>
      )}

      {source.unavailable_since ? (
        <p className="badge badge-risk">
          來源已失效，自 <Timestamp at={source.unavailable_since} />{" "}
          起無法取得。目前顯示的是失效前保存的內容。
        </p>
      ) : !source.last_checked_at ? (
        <p className="note">尚未檢查過來源是否仍可取得。</p>
      ) : null}

      {(source.source_version ||
        source.content_hash ||
        (!source.unavailable_since && source.last_checked_at)) && (
        <details>
          <summary>識別碼</summary>
          <ul>
            {!source.unavailable_since && source.last_checked_at && (
              <li>
                最近一次來源可用性檢查：
                <Timestamp at={source.last_checked_at} />
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
      )}
    </>
  );
}
