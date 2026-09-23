import { Loading } from "../../../../shared/ui/Loading";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useDeleteDataset, useTestCaseDatasets } from "../../testcases.service";
import { ConfirmDelete } from "../../../../shared/ui/ConfirmDelete";
import { bytes } from "../../../../shared/format";
import { MutationError } from "./MutationError";

export function DatasetSection({ testCaseId }: { testCaseId: string }) {
  const datasets = useTestCaseDatasets(testCaseId);
  const [message, setMessage] = useState("");
  const remove = useDeleteDataset(testCaseId);

  return (
    <>
      <h2>測試資料</h2>
      <p>
        <Link to="/lab/datasets" search={{ test_case: testCaseId }}>
          上傳檔案
        </Link>
      </p>
      {datasets.isPending && <Loading what="檔案清單" />}
      <ReadFailure error={datasets.error} what="檔案清單" />
      {datasets.data &&
        (datasets.data.datasets.length === 0 ? (
          <p>還沒有上傳任何檔案。</p>
        ) : (
          <>
            <p className="note">
              目前 {datasets.data.datasets.length} 個檔案，合計 {bytes(datasets.data.total_bytes)}
              。上限在上傳頁的「大小限制」。
            </p>
            <ul className="file-tree" data-role="evidence">
              {datasets.data.datasets.map((d) => (
                <li key={d.dataset_id}>
                  {d.file_name}{" "}
                  <span className="note">
                    （{d.content_type}・{bytes(d.size_bytes)}）
                  </span>{" "}
                  <ConfirmDelete
                    scopeId={`dataset-delete-scope-${d.dataset_id}`}
                    scope={
                      <>
                        會刪掉 <strong>{d.file_name}</strong> 的檔案本體，儲存的位元組會被移除，
                        之後的 Run 讀不到它。沒有回收桶也沒有保留期，刪了就沒有備份可以還原。
                        已經跑過的 Run 不受影響——它們的快照仍保留這個檔案的名稱與內容雜湊， 所以那些
                        Run 仍可追溯，只是不再可重現。這個 Test Case
                        本身與其他檔案都還在；要連草稿一起刪，在這一頁最下面的「刪除這個 Test
                        Case」。
                      </>
                    }
                    pending={remove.isPending}
                    onAsk={() => remove.reset()}
                    onConfirm={() =>
                      remove.mutate(d.dataset_id, {
                        onSuccess: (result) => setMessage(result.note),
                      })
                    }
                    label="刪除這個檔案"
                    confirmLabel="確認刪除這個檔案"
                  />
                  <p className="note">
                    保存到 <Timestamp at={d.expires_at} /> 自動刪除
                  </p>
                </li>
              ))}
            </ul>
          </>
        ))}
      {message && <p role="status">{message}</p>}
      <MutationError error={remove.error} what="這個檔案" fallback="刪除沒有成功，可以再試一次。" />
    </>
  );
}
