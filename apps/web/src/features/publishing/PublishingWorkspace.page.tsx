import { Link } from "@tanstack/react-router";
import { Downloads } from "../packaging";
import { Loading } from "../../shared/ui/Loading";
import { ReadFailure } from "../../shared/ui/LoginRequired";
import { Timestamp } from "../../shared/ui/Timestamp";
import { BundleSection } from "./components/BundleSection";
import { PublisherSection } from "./components/PublisherSection";
import { useOwnPublications } from "./publishing.service";
import "./PublishingWorkspace.page.css";

export function PublishingWorkspace() {
  return (
    <section className="publishing-workspace">
      <header className="publishing-workspace-header">
        <p className="note">發佈</p>
        <h1>發佈與交付</h1>
        <p>
          從一個不可變版本開始，準備公開位址、組合 Bundle，或取回已建立的套件。公開位址不等於
          Catalog 曝光；曝光仍由營運者審核精確 Release。
        </p>
        <Link className="action" to="/workspace/skills">
          選擇要發佈的 Skill
        </Link>
      </header>

      <div className="publishing-workspace-grid">
        <div className="publishing-workspace-main">
          <PublicationOverview />
          <BundleSection />
          <Downloads embedded />
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

function PublicationOverview() {
  const overview = useOwnPublications();
  const publications = overview.data?.publications ?? [];

  return (
    <section aria-labelledby="publication-overview-title">
      <h2 id="publication-overview-title">Skill 發佈</h2>
      <p className="note">
        每筆 Publication 都指向一個不可變 Release；Catalog 是否曝光仍由營運者另行審核。
      </p>

      {overview.isPending && <Loading what="Skill 發佈清單" />}
      <ReadFailure error={overview.error} what="Skill 發佈清單" />

      {overview.data &&
        (publications.length === 0 ? (
          <p>
            還沒有任何 Skill Publication。先到 <Link to="/workspace/skills">資產庫</Link>{" "}
            選一個精確版本開始。
          </p>
        ) : (
          <ul className="download-list" data-role="evidence">
            {publications.map((publication) => (
              <li key={publication.skill_id} className="download-item">
                <p>
                  <strong>{publication.name}</strong>
                  <span
                    className={publication.status === "delisted" ? "badge badge-danger" : "badge"}
                  >
                    {publication.status === "published" ? "已發佈" : "已撤回"}
                  </span>
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
                <p className="note">
                  發佈狀態更新於 <Timestamp at={publication.status_changed_at} />
                </p>
              </li>
            ))}
          </ul>
        ))}
    </section>
  );
}
