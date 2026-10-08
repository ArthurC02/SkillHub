import { Link, useSearch } from "@tanstack/react-router";
import { useExposureCase, useExposureQueue } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ListFreshness } from "../../../shared/ui/ListFreshness";
import { AdminPage } from "../components/AdminPage";
import { ExposureQueue } from "./components/ExposureQueue";
import { ExposureReview } from "./components/ExposureReview";

function ExposureCaseSection({ publication }: { publication: string }) {
  const exposureCase = useExposureCase(publication);
  return (
    <>
      <h2>審這一筆：{publication}</h2>
      <p>
        <Link to="/admin/exposure" search={{}}>
          返回待審清單
        </Link>
      </p>
      <p className="note">
        {exposureCase.data && !exposureCase.error && (
          <>
            審核資料上次取得於{" "}
            <Timestamp at={new Date(exposureCase.dataUpdatedAt).toISOString()} relative />。{" "}
          </>
        )}
        <button
          type="button"
          disabled={exposureCase.isFetching}
          onClick={() => void exposureCase.refetch()}
        >
          {exposureCase.isFetching ? "重新整理中…" : "重新整理審核資料"}
        </button>
      </p>
      {exposureCase.isPending && <Loading what="這一筆的曝光審核資料" />}
      <ReadFailure error={exposureCase.error} what="這一筆的曝光審核資料" />
      {exposureCase.data && !exposureCase.error && (
        <ExposureReview exposureCase={exposureCase.data} publication={publication} />
      )}
    </>
  );
}

function ExposureQueueSection() {
  const queue = useExposureQueue();
  return (
    <>
      <h2>待審清單</h2>
      {queue.isPending && <Loading what="待審清單" />}
      <ReadFailure error={queue.error} what="待審清單">
        <p role="alert">
          暫時無法讀取待審清單。{queue.data ? "先前載入的內容已隱藏。" : "請稍後再試。"}
        </p>
        <button type="button" disabled={queue.isFetching} onClick={() => void queue.refetch()}>
          {queue.isFetching ? "重新讀取中…" : "再試一次"}
        </button>
      </ReadFailure>
      {queue.data && !queue.error && (
        <>
          {queue.data.publications.length > 0 && (
            <p role="status">待審：共 {queue.data.publications.length} 筆。</p>
          )}
          <ListFreshness
            inFlight={false}
            showWhenIdle
            updatedAt={queue.dataUpdatedAt}
            fetching={queue.isFetching}
            refetch={queue.refetch}
            subject="待審"
          />
          <ExposureQueue entries={queue.data.publications} />
        </>
      )}
    </>
  );
}

export function AdminExposure() {
  const { publication } = useSearch({ from: "/admin/exposure" });

  return (
    <AdminPage
      heading="曝光審核"
      lede="發佈物的最新 Release 要先由 operator 核准，才會出現在搜尋與目錄裡。"
    >
      {publication ? <ExposureCaseSection publication={publication} /> : <ExposureQueueSection />}
    </AdminPage>
  );
}
