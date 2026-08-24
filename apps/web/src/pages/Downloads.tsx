import { Loading } from "../components/Loading";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  deleteDownload,
  downloadHref,
  useDownloadRecords,
  useDownloads,
  type DownloadArtifact,
} from "../api/packaging";
import { ConfirmDelete } from "../components/ConfirmDelete";
import { DownloadArtifactFacts } from "../components/DownloadArtifactFacts";

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
  const [message, setMessage] = useState("");

  const remove = useMutation({
    mutationFn: (artifactId: string) => deleteDownload(artifactId),
    onSuccess: async () => {
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

      {downloads.isPending && <Loading what="下載紀錄" />}
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
                <DownloadHistory artifact={artifact} />
                <DownloadActions
                  artifact={artifact}
                  pending={remove.isPending}
                  onAskDelete={() => setMessage("")}
                  onConfirmDelete={() => remove.mutate(artifact.artifact_id)}
                />
              </li>
            ))}
          </ul>
        ))}
    </section>
  );
}

/**
 * WS-004's own words are 「誰、何時、哪一筆 artifact、哪一個 profile」. The facts
 * above answer the last two plus a count; this answers the first two, one row per
 * download, which is what a count cannot do.
 *
 * Fetched only when the disclosure is opened: a history page holds every package
 * this workspace ever built, and one request per row on load would make the
 * cheapest question on the page the most expensive read in the API.
 *
 * A count of zero is not the same as a list that failed to load, and neither is
 * rendered as the other.
 */
function DownloadHistory({ artifact }: { artifact: DownloadArtifact }) {
  const [open, setOpen] = useState(false);
  // Opened AND worth asking: the artifact already carries the count, so a zero
  // needs no round trip to answer with.
  const records = useDownloadRecords(artifact.artifact_id, open && artifact.download_count > 0);

  return (
    <details onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)}>
      <summary>誰下載過、什麼時候（{artifact.download_count}）</summary>
      {artifact.download_count === 0 ? (
        <p className="note">還沒有人下載過這個檔案。建立一個套件不等於取走它。</p>
      ) : (
        <>
          {/* `isFetching`, not `isPending`: a disabled query is "pending" forever,
              so a closed disclosure would claim to be loading something it has
              not asked for. */}
          {/* The expected count is already on the summary above (download_count),
              so this is the one loading line in the app that can say how many
              rows it is waiting for rather than only that it is waiting. */}
          {records.isFetching && (
            <p role="status" className="note">
              載入下載紀錄中…（共 {artifact.download_count} 筆）
            </p>
          )}
          {records.error && <p role="alert">無法讀取逐筆下載紀錄：{records.error.message}</p>}
          {records.data && (
            <ul className="note">
              {records.data.records.map((r) => (
                <li key={`${r.downloaded_at}-${r.actor}`}>
                  {r.downloaded_at}｜{r.actor}
                </li>
              ))}
            </ul>
          )}
          <p className="note">
            這是給你自己看的下載紀錄，與稽核事件是兩份不同的紀錄——保存期限與可見範圍都不一樣。
          </p>
        </>
      )}
    </details>
  );
}

function DownloadActions({
  artifact,
  pending,
  onAskDelete,
  onConfirmDelete,
}: {
  artifact: DownloadArtifact;
  pending: boolean;
  onAskDelete: () => void;
  onConfirmDelete: () => void;
}) {
  const client = useQueryClient();
  // The server's own predicate, not a copy of it (04 丙-29 ⑤). This read
  // `status === "available"` plus a locally derived expiry — two of the three things
  // download.go checks, missing the purge, which is not on this shape at all. So
  // a purged artifact offered a link that was known to 404, which puts the
  // refusal in a new tab instead of on this page. The reason it is not stated
  // here twice: `serve_state.label` above already says which of the four it is.
  return (
    <p>
      {artifact.servable ? (
        <a
          href={downloadHref(artifact.artifact_id)}
          // The server writes a `download_records` row when it serves the bytes,
          // and this list's `download_count` is computed from that table. A click
          // unmounts nothing and `refetchOnWindowFocus` is off (api/queryClient),
          // so without this the page kept saying 「還沒有人下載過這個檔案」 about a
          // file the reader had just taken — and DownloadHistory's `enabled` guard
          // reads the same stale 0, so opening the disclosure did not even ask.
          onClick={() => void client.invalidateQueries({ queryKey: ["downloads"] })}
        >
          下載
        </a>
      ) : (
        <span className="note">目前不提供下載。</span>
      )}
      {" ｜ "}
      <Link to="/skills/$skillId" params={{ skillId: artifact.skill_id }}>
        來源 Skill
      </Link>
      {" ｜ "}
      <ConfirmDelete
        scopeId={`delete-scope-${artifact.artifact_id}`}
        pending={pending}
        onAsk={onAskDelete}
        onConfirm={onConfirmDelete}
        scope={
          <>
            刪除的是這個套件的檔案本身。
            「你曾經下載過幾次」的紀錄會保留，因為那件事已經發生過；這一筆之後不再出現在清單裡，下載連結也失效。
            同一個版本隨時可以重新打包一次。重複刪除不算失敗。
          </>
        }
      />
    </p>
  );
}
