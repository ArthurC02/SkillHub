import { Link } from "@tanstack/react-router";

import { Timestamp } from "../../../../shared/ui/Timestamp";
import type { SkillSource } from "../../../../core/api/types";
import { ExternalLink } from "../../../../shared/ui/ExternalLink";
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
          來源網址： <ExternalLink href={source.url}>{source.url}</ExternalLink>
        </p>
      )}
      {source.plugin && (
        <>
          <p>
            來自 Agent Plugin：<code>{source.plugin.name}</code>
            {source.plugin.version ? ` ${source.plugin.version}` : ""}
          </p>
          {source.plugin.repository && (
            <p>
              Plugin 的 repository：{" "}
              <ExternalLink href={source.plugin.repository}>
                {source.plugin.repository}
              </ExternalLink>
            </p>
          )}
          <p className="note">{source.plugin.note}</p>
        </>
      )}

      {source.path && (
        <p>
          它在來源內的路徑：<code>{source.path}</code>
        </p>
      )}

      {source.fetched_at && (
        <p>
          擷取時間：
          <Timestamp at={source.fetched_at} />
        </p>
      )}

      {source.siblings && source.siblings.length > 0 && (
        <>
          <h3>同一個來源帶進來的其他 Skill（{source.siblings.length}）</h3>
          <p className="note">
            它們和這一個是同一次匯入進來的，各自是獨立的 Skill：各自有版本、各自試跑、各自下載。
            平台沒有「一次取得整套」這個動作。
          </p>
          <ul>
            {source.siblings.map((sibling) => (
              <li key={sibling.skill_id}>
                <Link to="/skills/$skillId" params={{ skillId: sibling.skill_id }}>
                  {sibling.name}
                </Link>
                {sibling.path && (
                  <>
                    {" "}
                    <code>{sibling.path}</code>
                  </>
                )}
              </li>
            ))}
          </ul>
        </>
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
