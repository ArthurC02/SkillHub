import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import type { SkillVersionSummary } from "../../../core/api/types";
import {
  useOwnPublications,
  type OwnerPublicationSummary,
  type Publication,
} from "../publishing.service";
import "./CatalogExposure.css";

const catalogExposureCopy: Record<
  OwnerPublicationSummary["catalog_exposure"]["state"],
  { label: string; note: string; caution?: boolean }
> = {
  listed: {
    label: "已列入 Catalog",
    note: "任何人都能從搜尋與 Catalog 找到這個 Release。",
  },
  awaiting_review: {
    label: "等待 Catalog 審核",
    note: "公開位址可使用；這個 Release 還不會出現在搜尋與 Catalog。",
  },
  revoked: {
    label: "Catalog 曝光已撤銷",
    note: "公開位址仍可使用，但這個 Release 不再出現在搜尋與 Catalog。",
    caution: true,
  },
  review_outdated: {
    label: "需要重新審核",
    note: "Catalog 收錄所依據的搜尋內容已變更；目前不會曝光。",
  },
  not_eligible: {
    label: "目前不符合曝光條件",
    note: "Publication 已撤回，或小工具的可用性或散布條件不允許曝光。",
    caution: true,
  },
  search_not_ready: {
    label: "搜尋內容尚未就緒",
    note: "Release 已通過審核，但 Catalog 的搜尋內容尚未可列出。",
  },
  unreleased: {
    label: "尚無 Release",
    note: "建立第一個不可變 Release 後才能進入 Catalog 審核。",
  },
};

export function CatalogExposure({ publication }: { publication: OwnerPublicationSummary }) {
  const copy = catalogExposureCopy[publication.catalog_exposure.state];
  return (
    <div className="catalog-exposure">
      <p>
        <span className={copy.caution ? "badge badge-danger" : "badge"}>{copy.label}</span>
      </p>
      <p className="note">{copy.note}</p>
    </div>
  );
}

export function VersionCatalogExposure({
  skillId,
  publication,
  version,
}: {
  skillId: string;
  publication: Publication;
  version: SkillVersionSummary;
}) {
  const selectedRelease = publication.releases.find(
    (release) => release.version_id === version.version_id,
  );
  const overview = useOwnPublications(Boolean(selectedRelease));

  if (!selectedRelease) {
    return (
      <ExposureSection>
        <p>
          <span className="badge">不適用</span>
        </p>
        <p className="note">
          這一版的目前狀態不適用：v{version.version_number} 還沒有 Release；Catalog 只審核不可變
          Release。
        </p>
      </ExposureSection>
    );
  }

  if (overview.isPending) {
    return (
      <ExposureSection>
        <Loading what="Catalog 曝光狀態" />
      </ExposureSection>
    );
  }

  if (overview.error) {
    return (
      <ExposureSection>
        <ReadFailure error={overview.error} what="Catalog 曝光狀態" />
        <ExposureRefresh fetching={overview.isFetching} refetch={overview.refetch} />
      </ExposureSection>
    );
  }

  const ownerPublication = overview.data?.publications.find(
    (candidate) =>
      candidate.skill_id === skillId &&
      candidate.publisher === publication.publisher &&
      candidate.name === publication.name,
  );
  const latest = ownerPublication?.latest_release;
  const latestRelease = latest
    ? publication.releases.find((release) => release.version_id === latest.version_id)
    : undefined;

  if (
    !ownerPublication ||
    !latest ||
    !latestRelease ||
    latestRelease.version_number !== latest.version_number ||
    ownerPublication.catalog_exposure.state === "unreleased"
  ) {
    return (
      <ExposureSection>
        <p role="alert">Catalog 曝光狀態暫時無法確認：發佈資料與工作區清單目前對不上。</p>
        <ExposureRefresh fetching={overview.isFetching} refetch={overview.refetch} />
      </ExposureSection>
    );
  }

  if (selectedRelease.version_id !== latest.version_id) {
    return (
      <ExposureSection>
        <p>
          <span className="badge">不適用</span>
        </p>
        <p className="note">
          這一版的目前狀態不適用：Catalog 曝光狀態只描述{" "}
          <Link
            to="/skills/$skillId/versions/$versionId"
            params={{ skillId, versionId: latest.version_id }}
          >
            最新 Release v{latest.version_number}
          </Link>
          ，不會套用到 v{version.version_number}。
        </p>
        <ExposureRefresh
          updatedAt={overview.dataUpdatedAt}
          fetching={overview.isFetching}
          refetch={overview.refetch}
        />
      </ExposureSection>
    );
  }

  return (
    <ExposureSection>
      <CatalogExposure publication={ownerPublication} />
      <ExposureRefresh
        updatedAt={overview.dataUpdatedAt}
        fetching={overview.isFetching}
        refetch={overview.refetch}
      />
    </ExposureSection>
  );
}

function ExposureSection({ children }: { children: ReactNode }) {
  return (
    <section aria-labelledby="catalog-exposure-title">
      <h3 id="catalog-exposure-title">Catalog 曝光</h3>
      {children}
    </section>
  );
}

function ExposureRefresh({
  updatedAt,
  fetching,
  refetch,
}: {
  updatedAt?: number;
  fetching: boolean;
  refetch: () => unknown;
}) {
  return (
    <p className="note">
      {updatedAt ? (
        <>
          狀態上次取得於 <Timestamp at={new Date(updatedAt).toISOString()} relative />。{" "}
        </>
      ) : null}
      <button type="button" disabled={fetching} onClick={() => void refetch()}>
        {fetching ? "重新整理中…" : "重新整理 Catalog 曝光狀態"}
      </button>
    </p>
  );
}
