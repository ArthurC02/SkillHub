import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import {
  downloadHref,
  useDeleteDownload,
  useDownloadRecords,
  useDownloads,
  useRefreshDownloads,
  type DownloadArtifact,
} from "../packaging.service";
import { ConfirmDelete } from "../../../shared/ui/ConfirmDelete";
import { DownloadArtifactFacts } from "../components/DownloadArtifactFacts";
import { useContinuationFocus } from "../../../shared/ui/useContinuationFocus";

export function Downloads({
  embedded = false,
  selectedArtifact,
}: {
  embedded?: boolean;
  selectedArtifact?: string;
}) {
  const downloads = useDownloads();
  const [message, setMessage] = useState("");
  const remove = useDeleteDownload();
  const Heading = embedded ? "h2" : "h1";
  const selected = downloads.data?.downloads.find(
    (artifact) => artifact.artifact_id === selectedArtifact,
  );
  const selectedElement = useRef<HTMLLIElement>(null);
  useContinuationFocus(selectedArtifact, Boolean(selected), selectedElement);

  return (
    <section>
      <Heading>{embedded ? "交付紀錄" : "下載紀錄"}</Heading>
      <p className="note" data-role="teaching">
        這個工作區打包過的套件，新的在上面。到期的仍然列在這裡並標示已過期——那與「沒有這一筆」是兩個不同的答案。
      </p>

      {downloads.isPending && <Loading what="下載紀錄" />}
      <ReadFailure error={downloads.error} what="下載紀錄" />
      {downloads.data && selectedArtifact && !selected && (
        <p role="status" className="note">
          這個工作區目前找不到這筆交付紀錄。它可能已刪除，或目前帳號無法檢視。{" "}
          <Link to="/workspace/downloads" search={{}}>
            顯示完整清單
          </Link>
        </p>
      )}
      {message && <p role="status">{message}</p>}
      <ReadFailure error={remove.error} what="刪除">
        <p role="alert">沒有刪成，可以再按一次。</p>
      </ReadFailure>

      {downloads.data &&
        (downloads.data.downloads.length === 0 ? (
          <p>
            還沒有打包過任何套件。這裡是空的，代表你還沒有打包過東西，不是紀錄被清掉了。
            要打包，先從 <Link to="/library">資產庫</Link> 挑一個。
          </p>
        ) : (
          <>
            <ul className="download-list" data-role="evidence">
              {downloads.data.downloads.map((artifact) => {
                const current = artifact === selected;
                return (
                  <li
                    key={artifact.artifact_id}
                    className="download-item"
                    ref={current ? selectedElement : undefined}
                    tabIndex={current ? -1 : undefined}
                    aria-current={current ? "location" : undefined}
                  >
                    {current && (
                      <p>
                        <span className="badge">續接位置</span>
                      </p>
                    )}
                    <DownloadArtifactFacts artifact={artifact} />
                    <DownloadHistory artifact={artifact} />
                    <DownloadActions
                      artifact={artifact}
                      pending={remove.isPending}
                      onAskDelete={() => setMessage("")}
                      onConfirmDelete={() =>
                        remove.mutate(artifact.artifact_id, {
                          onSuccess: () =>
                            setMessage("已刪除。檔案不再提供下載，下載紀錄本身保留。"),
                        })
                      }
                    />
                  </li>
                );
              })}
            </ul>
            <p className="note" data-role="teaching">
              每一列的徽章是打包目標；安裝說明在套件內的 INSTALL.md。到期後檔案刪除，
              同一版本隨時可以再打包一次。
            </p>
            <p className="note">
              每一列折疊起來的那兩串是雜湊，不是簽章。
              <strong>MVP 的套件不帶數位簽章，平台也不驗簽</strong>
              （這是明文的「不做」）——它們證明得了「位元組沒有被改過」，
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

function DownloadHistory({ artifact }: { artifact: DownloadArtifact }) {
  const [open, setOpen] = useState(false);
  const records = useDownloadRecords(artifact.artifact_id, open && artifact.download_count > 0);

  return (
    <details onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)}>
      <summary>誰下載過、什麼時候（{artifact.download_count}）</summary>
      {artifact.download_count === 0 ? (
        <p className="note">還沒有人下載過這個檔案。建立一個套件不等於取走它。</p>
      ) : (
        <>
          {/* isFetching, not isPending: a disabled query is "pending" forever,
              so a closed disclosure would claim to be loading. */}
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
  const refreshDownloads = useRefreshDownloads();
  return (
    <p>
      {artifact.servable ? (
        <a
          href={downloadHref(artifact.artifact_id)}
          // Races the browser's own download request; the invalidated count can
          // still read one behind until the next visit.
          onClick={refreshDownloads}
        >
          下載
        </a>
      ) : (
        <span className="note">{artifact.serve_state.label}</span>
      )}
      {artifact.skill_id && artifact.skill_version_id ? (
        <>
          {" ｜ "}
          <Link
            to="/skills/$skillId/versions/$versionId"
            params={{ skillId: artifact.skill_id, versionId: artifact.skill_version_id }}
          >
            來源版本
          </Link>
        </>
      ) : artifact.skill_id ? (
        <>
          {" ｜ "}
          <Link to="/skills/$skillId" params={{ skillId: artifact.skill_id }}>
            來源 Skill
          </Link>
        </>
      ) : null}
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
