import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { useState } from "react";
import { useDeleteRunArtifact, useRunArtifacts } from "../../runs.service";
import { ConfirmDelete } from "../../../../shared/ui/ConfirmDelete";
import { RunArtifactFacts } from "./RunArtifactFacts";

export function RunArtifacts({ runId }: { runId: string }) {
  const artifacts = useRunArtifacts(runId);
  const [message, setMessage] = useState("");
  const remove = useDeleteRunArtifact(runId);

  return (
    <>
      <h2>這次 Run 的產出</h2>
      {artifacts.isPending && <Loading what="產出清單" />}
      <ReadFailure error={artifacts.error} what="產出清單" />
      <ReadFailure error={remove.error} what="刪除這個產出">
        <p role="alert">沒有刪成，可以再按一次。</p>
      </ReadFailure>
      {message && <p role="status">{message}</p>}
      {artifacts.data?.truncated && (
        <p className="notice" role="status">
          有些產出未被收集；清單只保留成功收集的檔案。
        </p>
      )}

      {artifacts.data &&
        (artifacts.data.artifacts.length === 0 ? (
          <p>
            {artifacts.data.truncated
              ? "收集結果為空，無法據此判定這次 Run 沒有產生檔案。"
              : "這次 Run 沒有留下任何檔案產出。"}
          </p>
        ) : (
          <>
            <p className="note">
              這些檔案平台都不提供下載連結：每一個都是沙箱的產出，控制平面不打開它。
              {artifacts.data.artifacts.some((a) => !a.expires_at) &&
                "「尚未定值」是平台還沒有為 Run 產出定下保存期限，這不表示它會永久保留。"}
            </p>
            <ul className="download-list">
              {artifacts.data.artifacts.map((artifact) => (
                <li key={artifact.artifact_id} className="download-item">
                  <RunArtifactFacts artifact={artifact} />
                  <p>
                    <ConfirmDelete
                      scopeId={`run-artifact-scope-${artifact.artifact_id}`}
                      pending={remove.isPending}
                      onAsk={() => setMessage("")}
                      onConfirm={() =>
                        remove.mutate(artifact.artifact_id, {
                          onSuccess: () =>
                            setMessage("已刪除。檔案不再存在，引用過它的評估會顯示證據已不存在。"),
                        })
                      }
                      scope={
                        <>
                          刪除的是這個檔案本身，這個 Run
                          的執行紀錄與評估判定都會保留。沒有回收桶也沒有保留期，這一頁沒有還原的地方，
                          刪了就取不回這個檔案。引用過這個檔案的評估不會被改寫，
                          它會顯示證據已不存在——那是當時真的看過的東西，判定不因為檔案被刪就變得不成立。
                          重複刪除不算失敗。
                        </>
                      }
                    />
                  </p>
                </li>
              ))}
            </ul>
          </>
        ))}
    </>
  );
}
