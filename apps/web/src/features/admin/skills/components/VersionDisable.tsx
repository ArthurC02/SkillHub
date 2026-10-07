import { useState } from "react";
import { ApiError } from "../../../../core/api/client";
import { ConfirmDelete } from "../../../../shared/ui/ConfirmDelete";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { OPERATOR_NOTE_MAX_BYTES, operatorNoteBytes } from "../../admin.model";
import { useDisableVersion, useOperatorVersion } from "../../admin.service";
import { WriteFailure } from "../../components/WriteFailure";

const versionIdPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function VersionDisable() {
  const [draft, setDraft] = useState("");
  const [versionId, setVersionId] = useState("");
  const valid = versionIdPattern.test(draft.trim());

  return (
    <section aria-labelledby="admin-version-heading">
      <h2 id="admin-version-heading">停用特定版本</h2>
      <p className="note">
        以精確 Skill Version ID 核對目標；不提供跨工作區版本清單，也不顯示私人套件內容。
      </p>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (valid) setVersionId(draft.trim());
        }}
      >
        <div className="field">
          <label htmlFor="admin-version-id">Skill Version ID</label>
          <input
            id="admin-version-id"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            aria-invalid={draft.trim() !== "" && !valid}
            aria-describedby={!valid ? "admin-version-id-hint" : undefined}
          />
        </div>
        {!valid && (
          <p className="note" id="admin-version-id-hint">
            請輸入完整的版本 UUID，再查詢。
          </p>
        )}
        <button
          type="submit"
          className="action"
          disabled={!valid}
          aria-describedby={!valid ? "admin-version-id-hint" : undefined}
        >
          核對版本
        </button>
      </form>
      {versionId !== "" && draft.trim() !== versionId && (
        <p role="status">版本 ID 已變更；請按「核對版本」後再操作。</p>
      )}
      {versionId !== "" && draft.trim() === versionId && (
        <VersionDecision key={versionId} versionId={versionId} />
      )}
    </section>
  );
}

function VersionDecision({ versionId }: { versionId: string }) {
  const [reason, setReason] = useState("");
  const status = useOperatorVersion(versionId);
  const disable = useDisableVersion(versionId);
  const reasonBytes = operatorNoteBytes(reason);
  const reasonTooLong = reasonBytes > OPERATOR_NOTE_MAX_BYTES;

  return (
    <div>
      {status.isFetching && <Loading what="版本狀態" />}
      {!status.isFetching && (
        <ReadFailure
          error={status.error}
          what="版本狀態"
          onRetry={() => void status.refetch()}
          retrying={status.isFetching}
        />
      )}
      {!status.isFetching && !status.error && status.data && (
        <>
          <p>
            版本 <code>{status.data.version_id}</code> · 第 {status.data.version_number} 版 ·{" "}
            {status.data.disabled ? "已停用" : "可建立新 Run"}
          </p>
          <button type="button" onClick={() => void status.refetch()}>
            重新讀取狀態
          </button>
          {status.data.disabled ? (
            <p className="notice" role="status">
              {disable.error instanceof ApiError && disable.error.status === 409
                ? "此版本在送出前已停用；重複操作已記錄。"
                : "這個版本已停用，不再接受新 Run；既有 Run 和版本內容沒有改動。"}
            </p>
          ) : (
            <>
              <div className="field">
                <label htmlFor="admin-version-disable-reason">
                  停用理由（必填，最多 {OPERATOR_NOTE_MAX_BYTES} 位元組，會寫進動作紀錄）
                </label>
                <textarea
                  id="admin-version-disable-reason"
                  value={reason}
                  onChange={(event) => setReason(event.target.value)}
                  readOnly={disable.isPending}
                  aria-invalid={reasonTooLong}
                  aria-describedby={reasonTooLong ? "admin-version-disable-reason-long" : undefined}
                />
              </div>
              {reasonTooLong && (
                <p id="admin-version-disable-reason-long" className="note">
                  理由太長：目前 {reasonBytes} 位元組，上限 {OPERATOR_NOTE_MAX_BYTES} 位元組。
                </p>
              )}
              {reason.trim() === "" ? (
                <p className="note">填寫理由後才能停用。</p>
              ) : !reasonTooLong ? (
                <ConfirmDelete
                  key={reason.trim()}
                  scopeId="admin-version-disable-scope"
                  scope={
                    <>
                      停用版本 <code>{status.data.version_id}</code>（第{" "}
                      {status.data.version_number} 版），理由：{reason.trim()}
                      。停用不可恢復；只阻止新 Run，既有 Run
                      與版本內容不變。若停錯，擁有者需要建立新版本。
                    </>
                  }
                  pending={disable.isPending}
                  label="停用版本"
                  confirmLabel="確認停用版本"
                  onConfirm={() => disable.mutate(reason.trim())}
                />
              ) : null}
              <WriteFailure error={disable.error} />
            </>
          )}
        </>
      )}
    </div>
  );
}
