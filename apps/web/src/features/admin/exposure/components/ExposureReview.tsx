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
    return <p>尚未進索引：搜尋與目錄目前沒有這個 Skill 可以顯示的內容。</p>;
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
  const [decision, setDecision] = useState<ExposureDecision>("approved");
  const review = useReviewExposure(publication);

  return (
    <ActionForm
      id="admin-exposure-review"
      submitLabel={`送出${DECISION_LABEL[decision]}`}
      pending={review.isPending}
      error={review.error}
      done={review.isSuccess && "已送出，上面的狀態已更新。"}
      onSubmit={(reason) =>
        review.mutate({
          release_id: c.release.release_id,
          expected_sequence: c.sequence,
          decision,
          reason,
        })
      }
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
              onChange={() => setDecision(value)}
            />
            {DECISION_LABEL[value]}
          </label>
        ))}
      </fieldset>
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
      <p>
        發佈物：
        <strong>
          {c.publisher}/{c.name}
        </strong>
        ，狀態：{STATUS_LABEL[c.status]}
      </p>
      <p>
        最新 Release：版本 {c.release.version_number}，內容雜湊{" "}
        <code>{c.release.content_hash}</code>
        ，發佈於 <Timestamp at={c.release.released_at} />
      </p>
      <p>{c.exposed ? "目前曝光中：搜尋與目錄看得到它。" : "目前未曝光：搜尋與目錄看不到它。"}</p>
      <p className="note">
        審核序號：{c.sequence}（送出審核結果時，伺服器用這個序號確認你看到的還是最新的一筆）。
      </p>

      <h3>搜尋索引會收錄與顯示的內容</h3>
      <SnapshotSection exposureCase={c} />

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

      <h2>審核這一版</h2>
      <ReviewForm exposureCase={c} publication={publication} />
    </>
  );
}
