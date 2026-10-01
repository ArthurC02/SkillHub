import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import type { OwnSkills } from "../../../../core/api/types";
import { useCreateTestCase } from "../../testcases.service";
import { CreateValidation } from "./CreateValidation";
import { MAX_NAME_BYTES, MAX_PROMPT_BYTES, oversizeReason } from "../test-cases.model";
import { MutationError } from "./MutationError";

export function CreateTestCaseForm({
  skills,
  skillsError,
  chosenSkill,
  onSkillChange,
  name,
  onNameChange,
  prompt,
  onPromptChange,
  create,
  onCreated,
}: {
  skills: OwnSkills | undefined;
  skillsError: Error | null;
  chosenSkill: string;
  onSkillChange: (skillId: string) => void;
  name: string;
  onNameChange: (name: string) => void;
  prompt: string;
  onPromptChange: (prompt: string) => void;
  create: ReturnType<typeof useCreateTestCase>;
  onCreated: (testCaseId: string) => void;
}) {
  const sizeReason = oversizeReason(name, prompt);

  return (
    <>
      <ReadFailure error={skillsError} what="你的小工具清單" />
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (oversizeReason(name, prompt)) return;
          create.mutate(
            { skillId: chosenSkill, name, prompt },
            { onSuccess: (tc) => onCreated(tc.test_case_id) },
          );
        }}
      >
        <p className="field">
          <label htmlFor="tc-skill">小工具</label>
          <select id="tc-skill" value={chosenSkill} onChange={(e) => onSkillChange(e.target.value)}>
            <option value="">請選擇</option>
            {skills?.skills.map((s) => (
              <option key={s.skill_id} value={s.skill_id}>
                {s.name}
              </option>
            ))}
          </select>
        </p>
        <p className="field">
          <label htmlFor="tc-name">名稱</label>
          <input
            id="tc-name"
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            size={40}
          />{" "}
          <span className="note">名稱最多 {MAX_NAME_BYTES} bytes。</span>
        </p>
        <p className="field">
          <label htmlFor="tc-prompt">User Prompt</label>
          <textarea
            id="tc-prompt"
            rows={4}
            cols={60}
            value={prompt}
            onChange={(e) => onPromptChange(e.target.value)}
          />
          <br />
          <span className="note">User Prompt 最多 {MAX_PROMPT_BYTES} bytes。</span>
        </p>
        <CreateValidation skillId={chosenSkill} name={name} prompt={prompt} />
        {sizeReason && (
          <p className="note" role="status" id="create-size-why">
            {sizeReason}
          </p>
        )}
        <button
          type="submit"
          className="action"
          disabled={
            create.isPending ||
            chosenSkill === "" ||
            name.trim() === "" ||
            prompt.trim() === "" ||
            sizeReason !== null
          }
          aria-describedby={
            chosenSkill === "" || name.trim() === "" || prompt.trim() === ""
              ? "create-why"
              : sizeReason
                ? "create-size-why"
                : undefined
          }
        >
          {create.isPending ? "建立中…" : "建立"}
        </button>
      </form>
      <MutationError
        error={create.error}
        what="測試題"
        fallback="建立沒有成功，可以再按一次。"
        serverSaysStatuses={[400]}
      />
    </>
  );
}
