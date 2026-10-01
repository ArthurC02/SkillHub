import { Link } from "@tanstack/react-router";
import type { SkillSource } from "../../../../core/api/types";

export function SourceSiblingsList({
  siblings,
}: {
  siblings: NonNullable<SkillSource["siblings"]>;
}) {
  return (
    <>
      <h3>同一個來源帶進來的其他小工具（{siblings.length}）</h3>
      <p className="note">
        它們和這一個是同一次匯入進來的，各自是獨立的小工具：各自有版本、各自試跑、各自下載。
        平台沒有「一次取得整套」這個動作。
      </p>
      <ul>
        {siblings.map((sibling) => (
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
  );
}
