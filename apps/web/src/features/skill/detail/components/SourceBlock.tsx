import { Timestamp } from "../../../../shared/ui/Timestamp";
import type { SkillSource } from "../../../../core/api/types";
import { ExternalLink } from "../../../../shared/ui/ExternalLink";
import { GeneratedSourceBlock } from "./GeneratedSourceBlock";
import { SourcePluginInfo } from "./SourcePluginInfo";
import { SourceSiblingsList } from "./SourceSiblingsList";
import { SourceAvailabilityNote } from "./SourceAvailabilityNote";
import { SourceIdentifiersDetails } from "./SourceIdentifiersDetails";

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
      {source.plugin && <SourcePluginInfo plugin={source.plugin} />}

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
        <SourceSiblingsList siblings={source.siblings} />
      )}

      {source.availability && source.availability.value !== "available" && (
        <SourceAvailabilityNote
          availability={source.availability}
          unavailableSince={source.unavailable_since}
        />
      )}

      <SourceIdentifiersDetails source={source} />
    </>
  );
}
