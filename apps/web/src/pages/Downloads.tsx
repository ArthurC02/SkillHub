import { Loading } from "../components/Loading";
import { ReadFailure } from "../components/LoginRequired";
import { Timestamp } from "../components/Timestamp";
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
    <section>
      <h1>下載紀錄</h1>
      <p className="note" data-role="teaching">
        這個工作區打包過的套件，新的在上面。到期的仍然列在這裡並標示已過期——那與「沒有這一筆」是兩個不同的答案。
      </p>

      {downloads.isPending && <Loading what="下載紀錄" />}
      <ReadFailure error={downloads.error} what="下載紀錄" />
      {message && <p role="status">{message}</p>}

      {downloads.data &&
        (downloads.data.downloads.length === 0 ? (
          /*
            主詞。這份清單列的是**打包過的 artifact**，不是下載事件——同一頁每一列
            都可能是 `download_count: 0`，那時列裡自己寫著「還沒有人下載過這個檔案。
            建立一個套件不等於取走它。」所以空狀態否認「沒有發生過下載」是在否認
            一件這份清單本來就不在回答的事。設計 §2.1 的強形式要的是「這個空**不是**
            什麼」，而說錯主詞不算說到。
            出口那一半是 IA-9 的同型：WorkspaceSkills 的空狀態也曾經說完下一步就叫
            人自己去導覽列找。打包的位址需要一個 skillId，這一頁手上沒有，所以下一步
            是先去挑一個 Skill。
          */
          <p>
            還沒有打包過任何套件。這裡是空的，代表你還沒有打包過東西，不是紀錄被清掉了。
            要打包，先從 <Link to="/workspace/skills">我的 Skill</Link> 挑一個。
          </p>
        ) : (
          <>
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
            {/*
              設計 §2.13 去重第 1 條（丙-142 第一批）。這三段以前跟著**每一列**印，
              逐位元相同：一句在十列上完全一樣的話，讀者從第 2 列起不可能因為它而作出
              不同判斷，所以它是這份清單的事實。搬的只有與列無關的那一句——每一列自己
              的日期、狀態詞與過期／遺失的說法一個字都沒有動（§2.10）。
              位置在清單下方而不是上方：它們是讀完一列之後才成立的限定語，而 §2.11(c)
              要的是「同一個區塊」，不是「在它前面」。
            */}
            <p className="note" data-role="teaching">
              每一列的徽章是打包目標；安裝說明在套件內的 INSTALL.md。到期後檔案刪除，
              同一版本隨時可以再打包一次。
            </p>
            <p className="note">
              每一列折疊起來的那兩串是雜湊，不是簽章。
              <strong>MVP 的套件不帶數位簽章，平台也不驗簽</strong>
              （ADR-027 決策 3 是明文的「不做」）——它們證明得了「位元組沒有被改過」，
              證明不了「這份東西是誰做的」。
            </p>
            <p className="note">
              這是給你自己看的下載紀錄，與稽核事件是兩份不同的紀錄——保存期限與可見範圍都不一樣。
            </p>
          </>
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
          <ReadFailure error={records.error} what="逐筆下載紀錄" />
          {records.data && (
            <ul className="note">
              {records.data.records.map((r) => (
                <li key={`${r.downloaded_at}-${r.actor}`}>
                  <Timestamp at={r.downloaded_at} />｜{r.actor}
                </li>
              ))}
            </ul>
          )}
          {/* 「與稽核事件是兩份不同的紀錄」搬到清單層級：它對每一列都成立，而且它
              以前只在 `download_count > 0` 的列上出現，也就是最需要它的那些列。 */}
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
  // refusal in a new tab instead of on this page.
  //
  // 2026-09-03（丙-142）：這裡以前寫的是「目前不提供下載。」——**只說結果，不說原因**，
  // 而原因（`serve_state.label`，例如「已過期,不再提供下載」）隔著幾行在另一段文字裡。
  // 設計 §2.4 第 2 型逐字管的就是這個形狀：控制項被**拿掉**而不是停用，替代文字只說
  // 「目前不提供」不說為什麼。修法是引用同一個 `serve_state.label`，不是另外寫一句——
  // 兩份措辭會漂移，而漂移的方向會是後改的那一份（04 丙-29 ⑤ 的同一條理由）。
  return (
    <p>
      {artifact.servable ? (
        // 設計 §4.6.3（ADR-064）：主要動作一頁至多一個，而這是清單裡的一列。
        // 十列各有一顆填色按鈕，等於一頁十個主要動作，也就是零個——強調只有在
        // 別的東西沒有被強調時才是強調。這一頁沒有「完成這一頁工作」的單一動作，
        // 所以它零個，連結本身一字未改。
        <a
          href={downloadHref(artifact.artifact_id)}
          // The server writes a `download_records` row when it serves the bytes,
          // and this list's `download_count` is computed from that table. A click
          // unmounts nothing and `refetchOnWindowFocus` is off (api/queryClient),
          // so without this the page kept saying 「還沒有人下載過這個檔案」 about a
          // file the reader had just taken — and DownloadHistory's `enabled` guard
          // reads the same stale 0, so opening the disclosure did not even ask.
          //
          // The invalidate and the browser's own fetch race, and the invalidate
          // can lose. **Decided: it loses, and that is the accepted outcome.**
          // What a reader sees when it does is one number: 「誰下載過（2）」 on a
          // file taken three times, correcting itself on the next visit to this
          // page. Never 「還沒有人下載過」 — that needs a count of zero, and the
          // losing refetch still overlaps the *previous* download's record.
          //
          // The two alternatives both cost more than that. A second refetch after
          // a delay does not close the race, it shrinks it, and buys that with a
          // number nobody can derive from anything (this repo has been bitten by
          // those). Fetching the bytes here and saving a blob does close it, and
          // pays with the whole file in memory and with the browser's own
          // download UI — progress, resume, the save dialog — replaced by a page
          // that appears to hang on a 200MB package. Neither is worth trading for
          // a count that is briefly one behind.
          onClick={() => void client.invalidateQueries({ queryKey: ["downloads"] })}
        >
          下載
        </a>
      ) : (
        <span className="note">{artifact.serve_state.label}</span>
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
