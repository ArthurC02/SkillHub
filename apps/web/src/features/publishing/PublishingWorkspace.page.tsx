import { Link, useSearch } from "@tanstack/react-router";
import { useRef } from "react";
import { Downloads } from "../packaging";
import { Loading } from "../../shared/ui/Loading";
import { ReadFailure } from "../../shared/ui/LoginRequired";
import { Timestamp } from "../../shared/ui/Timestamp";
import { useContinuationFocus } from "../../shared/ui/useContinuationFocus";
import { BundleSection } from "./components/BundleSection";
import { CatalogExposure } from "./components/CatalogExposure";
import { PublisherSection } from "./components/PublisherSection";
import { useOwnPublications } from "./publishing.service";
import "./PublishingWorkspace.page.css";

export function PublishingWorkspace() {
  const { artifact, publication, bundleVersion } = useSearch({ from: "/workspace/downloads" });
  const targetCount = [artifact, publication, bundleVersion].filter(Boolean).length;
  const conflictingTargets = targetCount > 1;

  return (
    <section className="publishing-workspace">
      <header className="publishing-workspace-header">
        <p className="note">發佈</p>
        <h1>發佈與交付</h1>
        <p>
          從一個不可變版本開始，準備公開位址、組合 Bundle，或取回已建立的套件。公開位址不等於
          Catalog 曝光；曝光仍由營運者審核精確 Release。
        </p>
        {conflictingTargets && (
          <p role="alert">
            這個連結同時指定了多個續接位置，因此無法判斷要打開哪一筆。一次只能續接一筆。{" "}
            <Link to="/workspace/downloads" search={{}}>
              顯示完整清單
            </Link>
          </p>
        )}
        <Link className="action" to="/workspace/skills">
          選擇要發佈的 Skill
        </Link>
      </header>

      <div className="publishing-workspace-grid">
        <div className="publishing-workspace-main">
          <PublicationOverview selectedPublication={conflictingTargets ? undefined : publication} />
          <BundleSection selectedVersion={conflictingTargets ? undefined : bundleVersion} />
          <Downloads embedded selectedArtifact={conflictingTargets ? undefined : artifact} />
        </div>
        <aside className="publishing-workspace-rail" aria-label="發佈身分與開始方式">
          <PublisherSection />
          <section>
            <h2>從單一版本發佈</h2>
            <p>
              到資產庫打開 Skill 的「版本與發佈」，選定不可變版本後再建立
              Release；平台不會替你改成最新版本。
            </p>
          </section>
        </aside>
      </div>
    </section>
  );
}

function PublicationOverview({ selectedPublication }: { selectedPublication?: string }) {
  const overview = useOwnPublications();
  const publications = overview.data?.publications ?? [];
  const selected = publications.find(
    (publication) => `${publication.publisher}/${publication.name}` === selectedPublication,
  );
  const selectedElement = useRef<HTMLLIElement>(null);
  useContinuationFocus(selectedPublication, Boolean(selected), selectedElement);

  return (
    <section aria-labelledby="publication-overview-title">
      <h2 id="publication-overview-title">Skill 發佈</h2>
      <p className="note">
        每筆 Publication 都指向一個不可變 Release；Catalog 是否曝光仍由營運者另行審核。
        公開頁會說明目前誰有資格取得；取得者身分與下載次數尚未提供。
      </p>

      {overview.isPending && <Loading what="Skill 發佈清單" />}
      <ReadFailure error={overview.error} what="Skill 發佈清單" />
      {overview.data && selectedPublication && !selected && (
        <p role="status" className="note">
          這個工作區目前找不到這筆 Skill 發佈。它可能已不存在，或目前帳號無法檢視。{" "}
          <Link to="/workspace/downloads" search={{}}>
            顯示完整清單
          </Link>
        </p>
      )}

      {overview.data &&
        (publications.length === 0 ? (
          <p>
            還沒有任何 Skill Publication。先到 <Link to="/workspace/skills">資產庫</Link>{" "}
            選一個精確版本開始。
          </p>
        ) : (
          <ul className="download-list" data-role="evidence">
            {publications.map((publication) => {
              const current = publication === selected;
              return (
                <li
                  key={publication.skill_id}
                  className="download-item"
                  ref={current ? selectedElement : undefined}
                  tabIndex={current ? -1 : undefined}
                  aria-current={current ? "location" : undefined}
                >
                  <p>
                    <strong>{publication.name}</strong>
                    <span
                      className={publication.status === "delisted" ? "badge badge-danger" : "badge"}
                    >
                      {publication.status === "published" ? "已發佈" : "已撤回"}
                    </span>
                    {current && <span className="badge">續接位置</span>}
                  </p>
                  <p>
                    公開位址：
                    <Link
                      to="/p/$publisher/$name"
                      params={{ publisher: publication.publisher, name: publication.name }}
                    >
                      {publication.address}
                    </Link>
                  </p>
                  {publication.latest_release ? (
                    <p>
                      最新 Release：
                      <Link
                        to="/skills/$skillId/versions/$versionId"
                        params={{
                          skillId: publication.skill_id,
                          versionId: publication.latest_release.version_id,
                        }}
                      >
                        v{publication.latest_release.version_number}
                      </Link>
                      ，發佈於 <Timestamp at={publication.latest_release.released_at} />
                    </p>
                  ) : (
                    <p className="note">這筆 Publication 尚未建立 Release。</p>
                  )}
                  <CatalogExposure publication={publication} />
                  <p className="note">
                    發佈狀態更新於 <Timestamp at={publication.status_changed_at} />
                  </p>
                </li>
              );
            })}
          </ul>
        ))}
    </section>
  );
}
