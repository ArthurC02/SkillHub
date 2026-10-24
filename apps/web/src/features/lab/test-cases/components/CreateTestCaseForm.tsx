import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import type { OwnSkills } from "../../../../core/api/types";
import { useCreateTestCase } from "../../testcases.service";
import { CreateValidation } from "./CreateValidation";
import { MAX_NAME_CHARS, MAX_PROMPT_CHARS, oversizeReason } from "../test-cases.model";
import { MutationError } from "./MutationError";

export function CreateTestCaseForm({
  skills,
  skillsError,
  retrySkills,
  retryingSkills,
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
  retrySkills: () => void;
  retryingSkills: boolean;
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
  const skillUnavailable = !skills?.skills.some((skill) => skill.skill_id === chosenSkill);

  return (
    <>
      <ReadFailure
        error={skillsError}
        what="你的小工具清單"
        retry={retrySkills}
        retrying={retryingSkills}
        focusTarget="create-test-case-heading"
      />
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (oversizeReason(name, prompt) || skillsError || skillUnavailable) return;
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
          <span className="note">名稱最多 {MAX_NAME_CHARS} 個字。</span>
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
          <span className="note">User Prompt 最多 {MAX_PROMPT_CHARS} 個字。</span>
        </p>
        <CreateValidation skillId={chosenSkill} name={name} prompt={prompt} />
        {(skillsError || (chosenSkill && skillUnavailable)) && (
          <p className="note" role="status" id="create-skills-why">
            {skillsError
              ? "目前無法確認可用的小工具，請先重新讀取清單。"
              : "選擇的小工具目前不在可用清單裡，請重新選擇。"}
          </p>
        )}
        {sizeReason && (
          <p className="note" role="status" id="create-size-why">
            {sizeReason}
          </p>
        )}
        <CreateTestCaseButton
          pending={create.isPending}
          skillsError={skillsError}
          skillUnavailable={skillUnavailable}
          chosenSkill={chosenSkill}
          name={name}
          prompt={prompt}
          sizeReason={sizeReason}
        />
      </form>
      <MutationError
        error={create.error}
        what="測試題"
        fallback="建立沒有成功，可以再按一次。"
        unknown="尚未確認測試題是否已建立；清單已重新讀取，請先確認裡面有沒有它，再決定要不要重按，以免多一題。"
      />
    </>
  );
}

function CreateTestCaseButton({
  pending,
  skillsError,
  skillUnavailable,
  chosenSkill,
  name,
  prompt,
  sizeReason,
}: {
  pending: boolean;
  skillsError: Error | null;
  skillUnavailable: boolean;
  chosenSkill: string;
  name: string;
  prompt: string;
  sizeReason: string | null;
}) {
  return (
    <button
      type="submit"
      className="action"
      disabled={
        pending ||
        Boolean(skillsError) ||
        skillUnavailable ||
        chosenSkill === "" ||
        name.trim() === "" ||
        prompt.trim() === "" ||
        sizeReason !== null
      }
      aria-describedby={
        skillsError || (chosenSkill && skillUnavailable)
          ? "create-skills-why"
          : chosenSkill === "" || name.trim() === "" || prompt.trim() === ""
            ? "create-why"
            : sizeReason
              ? "create-size-why"
              : undefined
      }
    >
      {pending ? "建立中…" : "建立"}
    </button>
  );
}
