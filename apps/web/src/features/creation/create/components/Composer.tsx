import type { ReactNode, RefObject } from "react";
import type { CreditBalance } from "../../../../core/session/credits.service";
import { ReferencePicker } from "../../generate/GenerateSkill";
import { MAX_MESSAGE_RUNES, points } from "../create.model";

type Reference = { id: string; name: string };

type ComposerProps = {
  hasSession: boolean;
  latestHidden: boolean;
  unseen: number;
  onShowLatest: () => void;
  failureBox: ReactNode;
  creditsBlocked: boolean;
  limitsFailed: boolean;
  credits: CreditBalance | undefined;
  choices: number[];
  budget: string;
  onBudget: (value: string) => void;
  busy: boolean;
  locked: boolean;
  frozen: boolean;
  dragging: boolean;
  onDragging: (value: boolean) => void;
  hasContent: boolean;
  file: File | undefined;
  preview: string | undefined;
  onChooseFile: (picked?: File) => void;
  onClearFile: () => void;
  fileInput: RefObject<HTMLInputElement | null>;
  refs: Reference[];
  onRefs: (update: Reference[] | ((old: Reference[]) => Reference[])) => void;
  onError: (error: Error) => void;
  picking: boolean;
  onPicking: (update: (old: boolean) => boolean) => void;
  message: string;
  onMessage: (value: string) => void;
  textarea: RefObject<HTMLTextAreaElement | null>;
  onSubmit: () => Promise<void>;
};

export function Composer({
  hasSession,
  latestHidden,
  unseen,
  onShowLatest,
  failureBox,
  creditsBlocked,
  limitsFailed,
  credits,
  choices,
  budget,
  onBudget,
  busy,
  locked,
  frozen,
  dragging,
  onDragging,
  hasContent,
  file,
  preview,
  onChooseFile,
  onClearFile,
  fileInput,
  refs,
  onRefs,
  onError,
  picking,
  onPicking,
  message,
  onMessage,
  textarea,
  onSubmit,
}: ComposerProps) {
  const startBlocked = !hasSession && (creditsBlocked || limitsFailed);
  return (
    <div className="composer-dock">
      {latestHidden && !failureBox && (
        <button type="button" className="to-latest" onClick={onShowLatest}>
          ↓ {unseen > 0 ? `${unseen} 則新訊息` : "回到最新"}
        </button>
      )}
      {startBlocked && (
        <p className="notice notice-danger" id="composer-why">
          {creditsBlocked ? credits?.block_reason : "讀不到這次可用的預算範圍，暫時不能開始。"}
        </p>
      )}
      {!hasSession && choices.length > 0 && (
        <BudgetPicker
          choices={choices}
          budget={budget}
          onBudget={onBudget}
          busy={busy}
          credits={creditsBlocked ? undefined : credits}
        />
      )}
      <div
        className="composer"
        data-dragging={dragging || undefined}
        data-empty={!hasContent || undefined}
        onDragOver={(e) => {
          if (locked) return;
          e.preventDefault();
          onDragging(true);
        }}
        onDragLeave={() => onDragging(false)}
        onDrop={(e) => {
          e.preventDefault();
          onDragging(false);
          if (!locked) onChooseFile(e.dataTransfer.files[0]);
        }}
      >
        {(file || refs.length > 0) && (
          <AttachedChips
            file={file}
            preview={preview}
            refs={refs}
            locked={locked}
            onClearFile={onClearFile}
            onRefs={onRefs}
          />
        )}
        <MessageInput
          textarea={textarea}
          message={message}
          onMessage={onMessage}
          busy={busy}
          locked={locked}
          frozen={frozen}
          creditsBlocked={creditsBlocked}
          hasChoices={choices.length > 0}
          onChooseFile={onChooseFile}
          onSubmit={onSubmit}
        />
        <ComposerTools
          fileInput={fileInput}
          disabled={locked || frozen}
          onChooseFile={onChooseFile}
          picking={picking}
          onPicking={onPicking}
          refCount={refs.length}
          message={message}
          busy={busy}
          hasSession={hasSession}
          sendWhy={startBlocked ? "composer-why" : undefined}
          onSubmit={onSubmit}
        />
        {picking && (
          <ReferenceChooser refs={refs} locked={locked} onRefs={onRefs} onError={onError} />
        )}
        {failureBox}
      </div>
      <span id="composer-limits">
        Enter 送出，Shift＋Enter 換行。流程圖可以貼上或拖進來：PNG、JPEG、WebP，最多 4,000,000
        位元組（約 3.8 MB）；參考 Skill 最多三個。
      </span>
    </div>
  );
}

function BudgetPicker({
  choices,
  budget,
  onBudget,
  busy,
  credits,
}: {
  choices: number[];
  budget: string;
  onBudget: (value: string) => void;
  busy: boolean;
  credits: CreditBalance | undefined;
}) {
  return (
    <fieldset className="budget-picker">
      <legend>這次預算上限</legend>
      <div className="quick-replies">
        {choices.map((v) => (
          <label key={v}>
            <input
              type="radio"
              name="creation-budget"
              value={v}
              checked={budget === String(v)}
              disabled={busy}
              onChange={(e) => onBudget(e.target.value)}
            />
            {points(v)}
          </label>
        ))}
      </div>
      {credits && (
        <span className="creation-fact">
          餘額 {credits.balance_credits} 點 · 這場約 {credits.estimated_session.low_credits}–
          {credits.estimated_session.high_credits} 點
          {credits.estimated_session.estimated && "（估計）"}
        </span>
      )}
    </fieldset>
  );
}

function AttachedChips({
  file,
  preview,
  refs,
  locked,
  onClearFile,
  onRefs,
}: {
  file: File | undefined;
  preview: string | undefined;
  refs: Reference[];
  locked: boolean;
  onClearFile: () => void;
  onRefs: ComposerProps["onRefs"];
}) {
  return (
    <ul className="chip-row">
      {file && (
        <li>
          <button type="button" disabled={locked} onClick={onClearFile}>
            {preview && <img className="chip-thumb" src={preview} alt="" />}
            移除流程圖：{file.name}
          </button>
        </li>
      )}
      {refs.map((r) => (
        <li key={r.id}>
          <button
            type="button"
            disabled={locked}
            onClick={() => onRefs((old) => old.filter((x) => x.id !== r.id))}
          >
            移除參考：{r.name}
          </button>
        </li>
      ))}
    </ul>
  );
}

function ComposerTools({
  fileInput,
  disabled,
  onChooseFile,
  picking,
  onPicking,
  refCount,
  message,
  busy,
  hasSession,
  sendWhy,
  onSubmit,
}: {
  fileInput: ComposerProps["fileInput"];
  disabled: boolean;
  onChooseFile: ComposerProps["onChooseFile"];
  picking: boolean;
  onPicking: ComposerProps["onPicking"];
  refCount: number;
  message: string;
  busy: boolean;
  hasSession: boolean;
  sendWhy: string | undefined;
  onSubmit: ComposerProps["onSubmit"];
}) {
  return (
    <div className="composer-tools">
      <label className="composer-attach">
        ＋ 流程圖
        <input
          ref={fileInput}
          type="file"
          accept="image/png,image/jpeg,image/webp"
          aria-describedby="composer-limits"
          disabled={disabled}
          onChange={(e) => onChooseFile(e.target.files?.[0])}
        />
      </label>
      <button
        type="button"
        aria-expanded={picking}
        aria-controls="composer-references"
        aria-describedby="composer-limits"
        disabled={disabled}
        onClick={() => onPicking((v) => !v)}
      >
        ＋ 參考 Skill{refCount > 0 && `（${refCount}）`}
      </button>
      <span className="note field-count" id="composer-count">
        {[...message].length.toLocaleString("zh-TW")} / {MAX_MESSAGE_RUNES.toLocaleString("zh-TW")}{" "}
        字{[...message].length > MAX_MESSAGE_RUNES && "——超過了，送出會被擋下"}
      </span>
      <button
        type="button"
        className="composer-send"
        disabled={disabled}
        aria-describedby={sendWhy}
        onClick={() => void onSubmit()}
      >
        {busy ? "送出中…" : hasSession ? "送出" : "開始創作"}
      </button>
    </div>
  );
}

function MessageInput({
  textarea,
  message,
  onMessage,
  busy,
  locked,
  frozen,
  creditsBlocked,
  hasChoices,
  onChooseFile,
  onSubmit,
}: {
  textarea: ComposerProps["textarea"];
  message: string;
  onMessage: ComposerProps["onMessage"];
  busy: boolean;
  locked: boolean;
  frozen: boolean;
  creditsBlocked: boolean;
  hasChoices: boolean;
  onChooseFile: ComposerProps["onChooseFile"];
  onSubmit: ComposerProps["onSubmit"];
}) {
  return (
    <label>
      <textarea
        id="creation-message"
        ref={textarea}
        aria-label="想完成的任務"
        aria-describedby="composer-count composer-limits"
        value={message}
        onChange={(e) => onMessage(e.target.value)}
        onKeyDown={(e) => {
          // isComposing: an IME's Enter confirms the selected character,
          // it doesn't mean submit.
          if (e.key !== "Enter" || e.shiftKey || e.nativeEvent.isComposing) return;
          e.preventDefault();
          if (!locked && !frozen) void onSubmit();
        }}
        onPaste={(e) => {
          const picked = [...e.clipboardData.files].find((f) => f.type.startsWith("image/"));
          if (!picked || locked) return;
          e.preventDefault();
          onChooseFile(picked);
        }}
        disabled={busy || frozen}
        placeholder={
          frozen && !creditsBlocked && hasChoices
            ? "先在上方選這次的預算上限"
            : "描述任務或回覆 Agent"
        }
      />
    </label>
  );
}

function ReferenceChooser({
  refs,
  locked,
  onRefs,
  onError,
}: {
  refs: Reference[];
  locked: boolean;
  onRefs: ComposerProps["onRefs"];
  onError: ComposerProps["onError"];
}) {
  return (
    <div id="composer-references">
      <ReferencePicker
        disabled={locked}
        references={refs}
        onToggle={(skillID, name) => {
          if (refs.some((r) => r.id === skillID)) onRefs(refs.filter((r) => r.id !== skillID));
          else if (refs.length >= 3) onError(new Error("參考 Skill 最多三個；先移除一個再加。"));
          else onRefs([...refs, { id: skillID, name }]);
        }}
      />
    </div>
  );
}
