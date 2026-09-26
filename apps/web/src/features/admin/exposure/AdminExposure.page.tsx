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
      {exposureCase.isPending && <Loading what="這一筆的曝光審核資料" />}
      <ReadFailure error={exposureCase.error} what="這一筆的曝光審核資料" />
      {exposureCase.data && (
        <ExposureReview exposureCase={exposureCase.data} publication={publication} />
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
      {queue.isPending && <Loading what="待審清單" />}
      <ReadFailure error={queue.error} what="待審清單" />
      {queue.data && <ExposureQueue entries={queue.data.publications} />}

      {publication && <ExposureCaseSection publication={publication} />}
    </AdminPage>
  );
}
