import { Link } from "@tanstack/react-router";
import { Downloads } from "../packaging";
import { BundleSection } from "./components/BundleSection";
import { PublisherSection } from "./components/PublisherSection";
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
