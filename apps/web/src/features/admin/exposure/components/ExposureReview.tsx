import { useState } from "react";
import type { ExposureCase, ExposureDecision } from "../../admin.service";
import { useReviewExposure } from "../../admin.service";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ActionForm } from "../../components/ActionForm";

const STATUS_LABEL: Record<ExposureCase["status"], string> = {
  published: "已發佈",
  delisted: "已撤回",
};

const DECISION_LABEL: Record<ExposureDecision, string> = {
  approved: "核准",
  revoked: "撤銷",
};

function tagsOf(tags: unknown): string {
  if (Array.isArray(tags)) return tags.join("、");
  return JSON.stringify(tags);
}

function SnapshotSection({ exposureCase: c }: { exposureCase: ExposureCase }) {
  if (!c.snapshot) {
    return <p>尚未進索引：搜尋與目錄目前沒有這個小工具可以顯示的內容。</p>;
  }
  const snapshot = c.snapshot;
  return (
    <>
      {!snapshot.current && (
        <p className="note">
          搜尋索引目前收的不是這一版：作者在審核之後又上傳了新版本，或索引還沒追上這個
          Release。以下內容不是這一版會被搜尋與顯示的那一份。
        </p>
      )}
      {!snapshot.enriched && (
        <p className="note">
          這一版的搜尋內容還在補充，補完之前的文字不是最後會被搜尋與顯示的那一份。
        </p>
      )}
      <dl>
        <div>
          <dt>名稱</dt>
          <dd>{snapshot.name}</dd>
        </div>
        <div>
          <dt>摘要</dt>
          <dd>{snapshot.summary}</dd>
        </div>
        <div>
          <dt>加強摘要</dt>
          <dd>{snapshot.enriched_summary}</dd>
        </div>
        <div>
          <dt>任務範例</dt>
          <dd>{snapshot.task_examples}</dd>
        </div>
        <div>
          <dt>標籤</dt>
          <dd>{tagsOf(snapshot.tags)}</dd>
        </div>
        <div>
          <dt>限制</dt>
          <dd>{snapshot.limitations}</dd>
        </div>
      </dl>
    </>
  );
}

function ReviewForm({
  exposureCase: c,
  publication,
}: {
  exposureCase: ExposureCase;
  publication: string;
}) {
  const [decision, setDecision] = useState<ExposureDecision>();
  const review = useReviewExposure(publication);
  const approvalUnavailable = c.approval?.allowed
    ? undefined
    : (c.approval?.refusal?.error ?? "無法確認核准資格；請重新整理審核資料。");

  return (
    <ActionForm
      id="admin-exposure-review"
      submitLabel={decision ? `送出${DECISION_LABEL[decision]}` : "送出審核結論"}
      pending={review.isPending}
      error={review.error}
      done={review.isSuccess && "已送出，上面的狀態已更新。"}
      contextKey={`${publication}:${c.release.release_id}:${decision ?? "none"}`}
      ready={decision !== undefined && (decision !== "approved" || !approvalUnavailable)}
      onSubmit={(reason) => {
        if (!decision || (decision === "approved" && approvalUnavailable)) return;
        review.mutate({
          release_id: c.release.release_id,
          expected_sequence: c.sequence,
          expected_snapshot_digest: c.snapshot?.digest ?? "",
          decision,
          reason,
        });
      }}
    >
      <fieldset>
        <legend>結論</legend>
        {(["approved", "revoked"] as const).map((value) => (
          <label key={value}>
            <input
              type="radio"
              name="admin-exposure-decision"
              value={value}
              checked={decision === value}
              aria-describedby={
                value === "approved" && approvalUnavailable
                  ? "admin-exposure-approval-why"
                  : undefined
              }
              onChange={() => {
                setDecision(value);
                review.reset();
              }}
              disabled={review.isPending || (value === "approved" && Boolean(approvalUnavailable))}
            />
            {DECISION_LABEL[value]}
          </label>
        ))}
        {approvalUnavailable && (
          <p id="admin-exposure-approval-why" className="note">
            {approvalUnavailable}
          </p>
        )}
      </fieldset>
      <div className="notice" data-role="evidence">
        <strong>送出前確認</strong>
        <p>
          這次只決定版本 {c.release.version_number}（<code>{c.release.content_hash}</code>）的
          Catalog 曝光。
        </p>
        <p>
          {decision === "approved"
            ? "核准後，搜尋與 Catalog 可以顯示這個 Release。"
            : decision === "revoked"
              ? "撤銷後，搜尋與 Catalog 會隱藏這個 Release；公開位址不受影響。"
              : "先選擇核准或撤銷；系統不會預先替你選擇。"}
        </p>
      </div>
    </ActionForm>
  );
}

export function ExposureReview({
  exposureCase: c,
  publication,
}: {
  exposureCase: ExposureCase;
  publication: string;
}) {
  return (
    <>
      <p>{c.exposed ? "目前曝光中：搜尋與目錄看得到它。" : "目前未曝光：搜尋與目錄看不到它。"}</p>
      <p>
        <strong>
          {c.publisher}/{c.name}
        </strong>
        ：{STATUS_LABEL[c.status]}；最新 Release 版本 {c.release.version_number}。
      </p>
      <details>
        <summary>版本識別與審核序號</summary>
        <p>
          內容雜湊 <code>{c.release.content_hash}</code>，發佈於{" "}
          <Timestamp at={c.release.released_at} />。
        </p>
        <p className="note">
          審核序號：{c.sequence}（送出審核結果時，伺服器用這個序號確認你看到的還是最新的一筆）。
        </p>
      </details>

      <h3>搜尋索引會收錄與顯示的內容</h3>
      <SnapshotSection exposureCase={c} />

      <h3>審核這一版</h3>
      <ReviewForm
        key={`${c.release.release_id}:${c.sequence}:${c.snapshot?.digest ?? ""}:${c.approval?.refusal?.reason ?? c.approval?.allowed}`}
        exposureCase={c}
        publication={publication}
      />

      <h3>歷次審核</h3>
      {c.history.length === 0 ? (
        <p>還沒有審核紀錄：0 筆。</p>
      ) : (
        <ul className="download-list">
          {c.history.map((record) => (
            <li className="download-item" key={record.sequence}>
              <p>
                {DECISION_LABEL[record.decision]}：{record.reason}
              </p>
              <p className="note">
                <Timestamp at={record.reviewed_at} />
              </p>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
