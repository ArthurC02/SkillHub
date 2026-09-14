import { useState } from "react";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { VersionDiff } from "../../../runs";
import { useSkillVersions, skillDiffUrl } from "../../skills.service";

export function VersionHistory({ skillId }: { skillId: string }) {
  const versions = useSkillVersions(skillId);
  const [pair, setPair] = useState<{ from: string; to: string } | null>(null);

  const list = versions.data?.versions ?? [];

  return (
    <section>
      <h2>版本</h2>
      {versions.isPending && <Loading what="版本歷史" />}
      <ReadFailure error={versions.error} what="版本歷史" />

      {versions.data &&
        (list.length === 0 ? (
          <p>
            無權檢視——這個工作區看不到這個 Skill 的版本內容。別人的 Skill 要 Fork
            之後才會有屬於你的版本；這不代表它沒有版本。
          </p>
        ) : (
          <>
            <p>
              共 {list.length} 版，最新 v{list[0].version_number}（
              <Timestamp at={list[0].created_at} />）
            </p>
            <details>
              <summary>每一版與它跟上一版的差異</summary>
              <ul className="search-results">
                {list.map((version, index) => {
                  const previous = list[index + 1];
                  const open = pair?.to === version.version_id;
                  return (
                    <li key={version.version_id} className="search-result">
                      <p>
                        <strong>v{version.version_number}</strong>{" "}
                        <span className="note">
                          建立時間：
                          <Timestamp at={version.created_at} />
                        </span>
                      </p>
                      {previous ? (
                        <p>
                          <button
                            type="button"
                            onClick={() =>
                              setPair(
                                open ? null : { from: previous.version_id, to: version.version_id },
                              )
                            }
                          >
                            {open ? "收起與上一版的比較" : "與上一版比較"}
                          </button>
                        </p>
                      ) : (
                        <p className="note">這是最早的版本，沒有上一版可以比較。</p>
                      )}
                      {open && <VersionDiff url={skillDiffUrl(skillId, pair.from, pair.to)} />}
                    </li>
                  );
                })}
              </ul>
              <p className="note">
                版本不可變：採用改善建議會建立新的一版。差異比對的是套件內容，不是試跑結果。
              </p>
            </details>
          </>
        ))}
    </section>
  );
}
