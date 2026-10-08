import { useSearch } from "@tanstack/react-router";
import { useExposureCase, useExposureQueue } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { AdminPage } from "../components/AdminPage";
import { ExposureQueue } from "./components/ExposureQueue";
import { ExposureReview } from "./components/ExposureReview";

function ExposureCaseSection({ publication }: { publication: string }) {
  const exposureCase = useExposureCase(publication);
  return (
    <>
      <h2>審這一筆：{publication}</h2>
      {exposureCase.isFetching && <Loading what="這一筆的曝光審核資料" />}
      {!exposureCase.isFetching && (
        <ReadFailure
          error={exposureCase.error}
          what="這一筆的曝光審核資料"
          onRetry={() => void exposureCase.refetch()}
          retrying={exposureCase.isFetching}
        />
      )}
      {exposureCase.data && !exposureCase.error && !exposureCase.isFetching && (
        <ExposureReview
          exposureCase={exposureCase.data}
          publication={publication}
          onRefresh={() => {
            void exposureCase.refetch();
          }}
        />
      )}
    </>
  );
}

export function AdminExposure() {
  const { publication } = useSearch({ from: "/admin/exposure" });
  const queue = useExposureQueue();

  return (
    <AdminPage
      heading="曝光審核"
      lede="發佈物的最新 Release 要先由 operator 核准，才會出現在搜尋與目錄裡。"
    >
      <h2>待審清單</h2>
      {queue.data && !queue.error && (
        <button type="button" disabled={queue.isFetching} onClick={() => void queue.refetch()}>
          重新整理待審清單
        </button>
      )}
      {queue.isFetching && <Loading what="待審清單" />}
      {!queue.isFetching && (
        <ReadFailure
          error={queue.error}
          what="待審清單"
          onRetry={() => void queue.refetch()}
          retrying={queue.isFetching}
        />
      )}
      {queue.data && !queue.error && !queue.isFetching && (
        <ExposureQueue entries={queue.data.publications} />
      )}

      {publication && <ExposureCaseSection publication={publication} />}
    </AdminPage>
  );
}
