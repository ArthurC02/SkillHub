import { useDeleteTestCase } from "../../testcases.service";
import { ConfirmDelete } from "../../../../shared/ui/ConfirmDelete";
import { MutationError } from "./MutationError";

export function DeleteTestCase({
  testCaseId,
  onDeleted,
}: {
  testCaseId: string;
  onDeleted: (result: { datasets_deleted: number }) => void;
}) {
  const remove = useDeleteTestCase(testCaseId);

  return (
    <>
      <h2>刪除這個 Test Case</h2>
      <p>
        <ConfirmDelete
          scopeId={`delete-scope-${testCaseId}`}
          scope="會刪掉這個草稿與它已上傳的檔案。沒有回收桶也沒有保留期，這一頁沒有還原的地方，按下去就沒有了。已經跑過的 Run 及其快照不受影響——那是那些 Run 執行內容的紀錄。"
          pending={remove.isPending}
          onAsk={() => remove.reset()}
          onConfirm={() => remove.mutate(undefined, { onSuccess: onDeleted })}
          label="刪除整個 Test Case"
          confirmLabel="確認刪除整個 Test Case"
        />
      </p>
      <MutationError
        error={remove.error}
        what="這個 Test Case"
        fallback="刪除沒有成功，可以再試一次。"
      />
    </>
  );
}
