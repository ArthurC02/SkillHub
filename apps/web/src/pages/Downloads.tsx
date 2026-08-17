import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  deleteDownload,
  downloadHref,
  useDownloads,
  type DownloadArtifact,
} from "../api/packaging";
import { DownloadArtifactFacts, isExpired } from "../components/DownloadArtifactFacts";

/**
 * 02:WS-002 第 1 條「使用者可查看……下載紀錄」, served by GET /downloads (WS-004).
 *
 * Three rules, none cosmetic:
 *
 * 1. **An expired package stays in the list and says it expired.** Dropping the
 *    row would answer 「我下載過什麼」 wrongly: 「已過期」 and 「從來沒有這一筆」 are
 *    different facts and the empty state below is the second one.
 * 2. **Delete states its scope before it runs** (02:WS-002 第 3 條, SEC-006). The
 *    bytes go; the record that you once downloaded it stays, because that stayed
 *    true. It is idempotent by contract, so a repeat is not an error.
 * 3. **No fabricated rows and no fabricated states.** A failed read is shown as
 *    a failed read, not as an empty history.
 */
export function Downloads() {
  const downloads = useDownloads();
  const client = useQueryClient();
  const [confirming, setConfirming] = useState("");
  const [message, setMessage] = useState("");

  const remove = useMutation({
    mutationFn: (artifactId: string) => deleteDownload(artifactId),
    onSuccess: async () => {
      setConfirming("");
      setMessage("已刪除。檔案不再提供下載，下載紀錄本身保留。");
      await client.invalidateQueries({ queryKey: ["downloads"] });
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "刪除失敗。"),
  });

  return (
    <section className="page">
      <h1>下載紀錄</h1>
      <p className="note">
        這個工作區打包過的套件，新的在上面。到期的仍然列在這裡並標示已過期——那與「沒有這一筆」是兩個不同的答案。
      </p>

      {downloads.isPending && <p>載入下載紀錄中…</p>}
      {downloads.error && <p role="alert">無法讀取下載紀錄：{downloads.error.message}</p>}
      {message && <p role="status">{message}</p>}

      {downloads.data &&
        (downloads.data.downloads.length === 0 ? (
          <p>還沒有打包過任何套件。這裡是空的，代表沒有發生過下載，不是紀錄被清掉了。</p>
        ) : (
          <ul className="download-list">
            {downloads.data.downloads.map((artifact) => (
              <li key={artifact.artifact_id} className="download-item">
                <DownloadArtifactFacts artifact={artifact} />
                <DownloadActions
                  artifact={artifact}
                  confirming={confirming === artifact.artifact_id}
                  pending={remove.isPending}
                  onAskDelete={() => {
                    setMessage("");
                    setConfirming(artifact.artifact_id);
                  }}
                  onCancelDelete={() => setConfirming("")}
                  onConfirmDelete={() => remove.mutate(artifact.artifact_id)}
                />
              </li>
            ))}
          </ul>
        ))}
    </section>
  );
}

function DownloadActions({
  artifact,
  confirming,
  pending,
  onAskDelete,
  onCancelDelete,
  onConfirmDelete,
}: {
  artifact: DownloadArtifact;
  confirming: boolean;
  pending: boolean;
  onAskDelete: () => void;
  onCancelDelete: () => void;
  onConfirmDelete: () => void;
}) {
  // Only `available` and unexpired bytes are served, and the server checks again
  // on every request. Offering a link that is known to 404 would put the refusal
  // in a new tab instead of on this page.
  const servable = artifact.status === "available" && !isExpired(artifact);

  return (
    <p>
      {servable ? (
        <a href={downloadHref(artifact.artifact_id)}>下載</a>
      ) : (
        <span className="note">目前不提供下載。</span>
      )}
      {" ｜ "}
      <Link to="/skills/$skillId" params={{ skillId: artifact.skill_id }}>
        來源 Skill
      </Link>
      {" ｜ "}
      {confirming ? (
        <>
          <button type="button" disabled={pending} onClick={onConfirmDelete}>
            確認刪除
          </button>{" "}
          <button type="button" disabled={pending} onClick={onCancelDelete}>
            取消
          </button>
          <span className="note">
            刪除的是這個套件的檔案本身。
            「你曾經下載過幾次」的紀錄會保留，因為那件事已經發生過；這一筆之後不再出現在清單裡，下載連結也失效。
            同一個版本隨時可以重新打包一次。重複刪除不算失敗。
          </span>
        </>
      ) : (
        <button type="button" onClick={onAskDelete}>
          刪除
        </button>
      )}
    </p>
  );
}
