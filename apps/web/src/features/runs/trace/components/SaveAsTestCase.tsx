import { CreateTestCaseBodyNameMaxLength as MAX_NAME } from "@skillhub/api-client-ts/limits";
import { Link } from "@tanstack/react-router";
import { useId, useState } from "react";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { isWriteResultUnknown, serverReason } from "../../../../core/api/client";
import { useSaveRunAsTestCase } from "../../runs.service";
import { runeCount } from "../../../../shared/textLength.model";

function saveFailureSentence(error: unknown) {
  if (isWriteResultUnknown(error))
    return "尚未確認是否已存成測試題；請先到測試題清單確認，再決定要不要重送，以免重複。";
  return serverReason(error) ?? "沒有存成測試題，可以再試一次。";
}

function blockedBy(name: string) {
  if (name === "") return "先填測試題名稱。";
  if (runeCount(name) > MAX_NAME) return `名稱最多 ${MAX_NAME} 個字。`;
  return undefined;
}

export function SaveAsTestCase({ runId, versionId }: { runId: string; versionId?: string }) {
  const [name, setName] = useState("");
  const save = useSaveRunAsTestCase(runId);
  const reasonId = useId();
  const trimmed = name.trim();
  const reason = blockedBy(trimmed);

  return (
    <div>
      <p>
        <label htmlFor="save-run-test-case-name">存成測試題，名稱</label>{" "}
        <input
          id="save-run-test-case-name"
          value={name}
          onChange={(e) => {
            save.reset();
            setName(e.target.value);
          }}
          aria-invalid={trimmed !== "" && reason !== undefined}
        />{" "}
        <button
          type="button"
          aria-disabled={reason !== undefined || save.isPending}
          aria-describedby={reason ? reasonId : undefined}
          onClick={() => {
            if (!reason && !save.isPending) save.mutate(trimmed);
          }}
        >
          {save.isPending ? "存檔中…" : "存成測試題"}
        </button>
      </p>
      <p className="note" role="status" id={reasonId}>
        {reason ?? "照這次實際跑的提示、驗收條件與評分準則另存一題；上傳的檔案不會一起複製。"}
      </p>
      {save.data && (
        <p role="status">
          已存成新的測試題。{" "}
          <Link
            to="/lab/test-cases/$testCaseId"
            params={{ testCaseId: save.data }}
            search={{ version: versionId }}
          >
            打開這個測試題
          </Link>
        </p>
      )}
      {save.error && (
        <ReadFailure error={save.error} what="存成測試題">
          <p role="alert">{saveFailureSentence(save.error)}</p>
        </ReadFailure>
      )}
    </div>
  );
}
