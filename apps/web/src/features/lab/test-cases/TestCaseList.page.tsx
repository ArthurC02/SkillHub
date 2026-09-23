import { Loading } from "../../../shared/ui/Loading";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { useState } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useCreateTestCase, useTestCases } from "../testcases.service";
import { useOwnSkills } from "../../skill";
import { CreateValidation } from "./components/CreateValidation";
import { MAX_NAME_BYTES, MAX_PROMPT_BYTES, oversizeReason } from "./test-cases.model";
import { MutationError } from "./components/MutationError";

export function TestCaseList() {
  const navigate = useNavigate();
  const { skill: filter } = useSearch({ from: "/lab/test-cases" });
  const testCases = useTestCases(filter);
  const skills = useOwnSkills();
  const [skillId, setSkillId] = useState("");
  const [name, setName] = useState("");
  const [prompt, setPrompt] = useState("");
  const sizeReason = oversizeReason(name, prompt);
  const rows = testCases.data?.pages.flatMap((page) => page.test_cases) ?? [];
  const ownedSkill = skills.data?.skills.find((s) => s.skill_id === filter);
  const notMine = Boolean(filter) && Boolean(skills.data) && !ownedSkill;
  const chosenSkill = skillId || ownedSkill?.skill_id || "";

  const create = useCreateTestCase();

  return (
    <section>
      <h1>Test Case</h1>
      <p className="note" data-role="teaching">
        Test Case 是可編輯的草稿：User Prompt、測試資料與驗收條件。
      </p>

      <h2>既有的 Test Case</h2>
      {filter && (
        <p className="note" role="status">
          只顯示 <strong>{rows[0]?.skill_name || ownedSkill?.name || "某一個 Skill"}</strong> 的
          Test Case。{" "}
          <Link to="/lab/test-cases" search={{ skill: undefined }}>
            顯示全部
          </Link>
        </p>
      )}
      {notMine && (
        <p className="notice" role="status">
          這個 Skill 不在你的工作區。Test Case 屬於工作區，所以這裡看不到它，建立表單的 Skill
          選單也選不到它——
          <Link to="/skills/$skillId" params={{ skillId: filter as string }}>
            先把它 Fork 一份
          </Link>
          ，才會有屬於你的版本可以建立 Test Case。
        </p>
      )}
      {testCases.isPending && <Loading what=" Test Case 清單" />}
      <ReadFailure error={testCases.error} what=" Test Case" />
      {testCases.data &&
        (rows.length === 0 ? (
          notMine ? null : (
            <p>{filter ? "這個 Skill 還沒有 Test Case。" : "還沒有 Test Case。"}</p>
          )
        ) : (
          <ul className="search-results" data-role="evidence">
            {rows.map((tc) => (
              <li key={tc.test_case_id} className="search-result">
                <Link to="/lab/test-cases/$testCaseId" params={{ testCaseId: tc.test_case_id }}>
                  {tc.name}
                </Link>
                <p className="note">
                  Skill：
                  {tc.skill_name === ""
                    ? "這個 Skill 已經不在你的清單裡（已刪除，或已下架）"
                    : tc.skill_name}
                </p>
                <p className="note">
                  驗收條件已確認 {tc.criteria_confirmed}/{tc.criteria_total} 條 · Rubric{" "}
                  {tc.has_rubric ? "有" : "無"} · 最後修改 <Timestamp at={tc.updated_at} />
                </p>
              </li>
            ))}
          </ul>
        ))}
      {testCases.hasNextPage && (
        <button
          type="button"
          disabled={testCases.isFetchingNextPage}
          onClick={() => testCases.fetchNextPage()}
        >
          {testCases.isFetchingNextPage ? "載入中…" : "載入更多"}
        </button>
      )}

      <h2>建立新的 Test Case</h2>
      <ReadFailure error={skills.error} what="你的 Skill 清單" />
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (oversizeReason(name, prompt)) return;
          create.mutate(
            { skillId: chosenSkill, name, prompt },
            {
              onSuccess: (tc) =>
                navigate({
                  to: "/lab/test-cases/$testCaseId",
                  params: { testCaseId: tc.test_case_id },
                }),
            },
          );
        }}
      >
        <p className="field">
          <label htmlFor="tc-skill">Skill</label>
          <select id="tc-skill" value={chosenSkill} onChange={(e) => setSkillId(e.target.value)}>
            <option value="">請選擇</option>
            {skills.data?.skills.map((s) => (
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
            onChange={(e) => setName(e.target.value)}
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
            onChange={(e) => setPrompt(e.target.value)}
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
        what="Test Case"
        fallback="建立沒有成功，可以再按一次。"
        serverSaysStatuses={[400]}
      />
    </section>
  );
}
