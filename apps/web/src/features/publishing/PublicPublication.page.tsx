import { Link, useParams } from "@tanstack/react-router";
import { Loading } from "../../shared/ui/Loading";
import { ReadFailure } from "../../shared/ui/LoginRequired";
import { Timestamp } from "../../shared/ui/Timestamp";
import { LabelledBadge } from "../../shared/ui/LabelledBadge";
import { SOURCE_LABELS } from "../../shared/ui/LicenseBadge.model";
import { Findings } from "../../shared/ui/Findings";
import { API_BASE_URL, ApiError } from "../../core/api/client";
import {
  useAcquirePublication,
  usePublicPublication,
  type BundleMemberChange,
  type PublicPublication as PublicPublicationData,
  type PublicRelease,
} from "./publishing.service";
import { actionFailureSentence } from "./publishing.model";
import { DeliveryAudience } from "./components/DeliveryAudience";
import "./PublicPublication.page.css";

function memberChangeSentence(change: BundleMemberChange): string {
  switch (change.change) {
    case "added":
      return `${change.name}：新增（v${change.to}）`;
    case "removed":
      return `${change.name}：移除（原為 v${change.from}）`;
    default:
      return `${change.name}：v${change.from} 升到 v${change.to}`;
  }
}

function ReleaseHistory({
  releases,
  versionOf,
}: {
  releases: PublicRelease[];
  versionOf: (r: PublicRelease) => string;
}) {
  if (releases.length <= 1) return null;
  return (
    <details>
      <summary>歷次 Release（{releases.length}）</summary>
      <ul>
        {releases.map((r, i) => (
          <li key={`${r.content_hash}-${i}`}>
            {versionOf(r)}，<Timestamp at={r.released_at} />，<code>{r.content_hash}</code>
            {r.changes && r.changes.length > 0 && (
              <ul>
                {r.changes.map((c) => (
                  <li key={c.name}>{memberChangeSentence(c)}</li>
                ))}
              </ul>
            )}
          </li>
        ))}
      </ul>
    </details>
  );
}

function AcquireAction({ publisher, name }: { publisher: string; name: string }) {
  const acquire = useAcquirePublication(publisher, name);

  return (
    <section className="publication-acquire" data-role="publication-acquire">
      <div>
        <p className="page-eyebrow">Ready to use</p>
        <h2>取得這份套件</h2>
        <p>先核對上方的靜態掃描、License 與可散布性，再下載不可變的版本。</p>
      </div>
      <div className="publication-acquire-action">
        <button
          className="action"
          type="button"
          onClick={() => acquire.mutate()}
          disabled={acquire.isPending}
        >
          {acquire.isPending ? "建立下載中…" : "下載"}
        </button>
        {acquire.isError && (
          <ReadFailure error={acquire.error} what="這個下載">
            <p role="alert">
              {actionFailureSentence(acquire.error, "下載沒有成功，可以再試一次。")}
            </p>
          </ReadFailure>
        )}
        {acquire.isSuccess && (
          <p>
            <a href={`${API_BASE_URL}${acquire.data.content_url}`}>下載 {acquire.data.file_name}</a>
            {" ｜ "}
            <Link to="/workspace/downloads" search={{ artifact: acquire.data.artifact_id }}>
              在交付紀錄查看這一份
            </Link>
          </p>
        )}
      </div>
    </section>
  );
}

function MissingPublicationContent() {
  return <p className="note">未測量——這個發佈物標記為提供中，但伺服器沒有附上版本內容。</p>;
}

function UnavailablePublication({ data }: { data: PublicPublicationData }) {
  return (
    <article className="public-publication-page">
      <header className="publication-hero">
        <p className="page-eyebrow">Public package</p>
        <h1>{data.name}</h1>
        <p role="alert">{data.availability.label}</p>
        <p className="note">{data.availability.note}</p>
      </header>
      {data.delisted_at && (
        <p className="note">
          撤回時間：
          <Timestamp at={data.delisted_at} />
        </p>
      )}
    </article>
  );
}

function PublicBundle({
  data,
  publisher,
  name,
}: {
  data: PublicPublicationData;
  publisher: string;
  name: string;
}) {
  const bundle = data.bundle;
  const bundleRelease = data.bundle_release;
  if (!bundle || !bundleRelease) return <MissingPublicationContent />;

  return (
    <article className="public-publication-page">
      <header className="publication-hero">
        <p className="page-eyebrow">Public bundle</p>
        <h1>{data.name}</h1>
        <p>{bundle.description}</p>
        <p className="note">
          由 {data.publisher} 發佈，Bundle 版本 v{bundle.version}。
        </p>
      </header>
      <section>
        <h2>成員</h2>
        <ul data-role="evidence">
          {bundle.members.map((member) => (
            <li key={member.name}>
              {member.name} · v{member.version_number}
            </li>
          ))}
        </ul>
      </section>
      <DeliveryAudience publication={data} level={2} />
      <section className="publication-trust" data-role="publication-trust">
        <div className="publication-trust-card">
          <h2>靜態掃描</h2>
          <Findings findings={bundleRelease.findings} />
        </div>
      </section>
      {data.acquisition.available && <AcquireAction publisher={publisher} name={name} />}
      <details>
        <summary>進階資訊（識別碼）</summary>
        <p>
          內容雜湊：<code>{bundleRelease.content_hash}</code>
        </p>
      </details>
      <ReleaseHistory releases={data.releases} versionOf={(r) => `v${r.version}`} />
    </article>
  );
}

function PublicSkill({
  data,
  publisher,
  name,
}: {
  data: PublicPublicationData;
  publisher: string;
  name: string;
}) {
  const skill = data.skill;
  const release = data.release;
  if (!skill || !release) return <MissingPublicationContent />;

  return (
    <article className="public-publication-page">
      <header className="publication-hero">
        <p className="page-eyebrow">Public skill</p>
        <h1>{skill.name}</h1>
        <p>{skill.summary}</p>
        <p className="note">
          由 {data.publisher} 發佈，稱為 {data.name}。
        </p>
        <p>
          目前版本：v{release.version_number}，發佈於 <Timestamp at={release.released_at} />
        </p>
      </header>
      <DeliveryAudience publication={data} level={2} />
      <section className="publication-trust" data-role="publication-trust">
        <div className="publication-trust-card">
          <h2>靜態掃描</h2>
          <Findings findings={release.findings} />
        </div>
        <div className="publication-trust-card">
          <h2>License 與可散布性</h2>
          {release.license.expression ? (
            <p>
              <code className="license-expression">{release.license.expression}</code>{" "}
              {release.license.source && (
                <span className="badge badge-license-source">
                  {SOURCE_LABELS[release.license.source] ?? release.license.source}
                </span>
              )}
            </p>
          ) : (
            <p className="note">未宣告——沒有人替這個版本宣告 License。</p>
          )}
          <p>
            <LabelledBadge kind="redistribution" value={release.redistribution} />
          </p>
        </div>
      </section>
      {data.acquisition.available && <AcquireAction publisher={publisher} name={name} />}
      <details>
        <summary>進階資訊（識別碼）</summary>
        <p>
          內容雜湊：<code>{release.content_hash}</code>
        </p>
      </details>
      <ReleaseHistory releases={data.releases} versionOf={(r) => `v${r.version_number}`} />
    </article>
  );
}

export function PublicPublication() {
  const { publisher, name } = useParams({ from: "/p/$publisher/$name" });
  const publication = usePublicPublication(publisher, name);

  if (publication.isPending) return <Loading what="這個發佈物" />;
  if (publication.error instanceof ApiError && publication.error.status === 404) {
    return <p role="alert">沒有這個發佈物。</p>;
  }
  if (publication.error) return <ReadFailure error={publication.error} what="這個發佈物" />;
  if (!publication.data) return null;
  if (publication.data.availability.value !== "available") {
    return <UnavailablePublication data={publication.data} />;
  }
  if (publication.data.kind === "bundle") {
    return <PublicBundle data={publication.data} publisher={publisher} name={name} />;
  }
  return <PublicSkill data={publication.data} publisher={publisher} name={name} />;
}
