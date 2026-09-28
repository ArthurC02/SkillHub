import { ExternalLink } from "../../../../shared/ui/ExternalLink";
import type { SkillSource } from "../../../../core/api/types";

export function SourcePluginInfo({ plugin }: { plugin: NonNullable<SkillSource["plugin"]> }) {
  return (
    <>
      <p>
        來自 Agent Plugin：<code>{plugin.name}</code>
        {plugin.version ? ` ${plugin.version}` : ""}
      </p>
      {plugin.repository && (
        <p>
          Plugin 的 repository：{" "}
          <ExternalLink href={plugin.repository}>{plugin.repository}</ExternalLink>
        </p>
      )}
      <p className="note">{plugin.note}</p>
    </>
  );
}
