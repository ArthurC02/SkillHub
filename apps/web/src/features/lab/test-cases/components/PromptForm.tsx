import { useState } from "react";
import { useUpdateTestCase } from "../../testcases.service";
import type { TestCase } from "../../testcases.service";
import { MAX_NAME_BYTES, MAX_PROMPT_BYTES, oversizeReason } from "../test-cases.model";
import { MutationError } from "./MutationError";

export function PromptForm({ testCase }: { testCase: TestCase }) {
  const [name, setName] = useState(testCase.name);
  const [prompt, setPrompt] = useState(testCase.user_prompt);
  const sizeReason = oversizeReason(name, prompt);
  const save = useUpdateTestCase(testCase.test_case_id);

  return (
    <>
      <h2>名稱與 User Prompt</h2>
      <p className="field">
        <label htmlFor="edit-name">名稱</label>
        <input
          id="edit-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          size={40}
        />{" "}
        <span className="note">名稱最多 {MAX_NAME_BYTES} bytes。</span>
      </p>
      <p className="field">
        <label htmlFor="edit-prompt">User Prompt</label>
        <textarea
          id="edit-prompt"
          rows={5}
          cols={60}
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
        />
        <br />
        <span className="note">User Prompt 最多 {MAX_PROMPT_BYTES} bytes。</span>
      </p>
      <button
        type="button"
        disabled={
          save.isPending || prompt.trim() === "" || name.trim() === "" || sizeReason !== null
        }
        aria-describedby={
          name.trim() === "" || prompt.trim() === ""
            ? "edit-required-reason"
            : sizeReason
              ? "edit-size-reason"
              : undefined
        }
        onClick={() => {
          if (sizeReason) return;
          save.mutate({ name, user_prompt: prompt });
        }}
      >
        {save.isPending ? "儲存中…" : "儲存"}
      </button>{" "}
      {(name.trim() === "" || prompt.trim() === "") && (
        <span id="edit-required-reason" className="note" role="status">
          還不能儲存，因為：
          {[
            name.trim() === "" ? "名稱是空的" : "",
            prompt.trim() === "" ? "User Prompt 是空的" : "",
          ]
            .filter((s) => s !== "")
            .join("、")}
          。兩個都是必填。
        </span>
      )}
      {sizeReason && name.trim() !== "" && prompt.trim() !== "" && (
        <span id="edit-size-reason" className="note" role="status">
          {sizeReason}
        </span>
      )}
      {save.isSuccess && <p role="status">已儲存。</p>}
      <MutationError
        error={save.error}
        what="名稱與 User Prompt"
        fallback="儲存沒有成功，可以再試一次。"
        serverSaysStatuses={[400]}
      />
    </>
  );
}
