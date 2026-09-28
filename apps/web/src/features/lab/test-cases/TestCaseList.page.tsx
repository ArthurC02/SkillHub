import { useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useCreateTestCase, useTestCases } from "../testcases.service";
import { SkillWorkspaceNav, useOwnSkills } from "../../skill";
import { ExistingTestCasesSection } from "./components/ExistingTestCasesSection";
import { CreateTestCaseForm } from "./components/CreateTestCaseForm";

export function TestCaseList() {
  const navigate = useNavigate();
  const { skill: filter, version } = useSearch({ from: "/lab/test-cases" });
  const testCases = useTestCases(filter);
  const skills = useOwnSkills();
  const [skillId, setSkillId] = useState("");
  const [name, setName] = useState("");
  const [prompt, setPrompt] = useState("");
  const rows = testCases.data?.pages.flatMap((page) => page.test_cases) ?? [];
  const ownedSkill = skills.data?.skills.find((s) => s.skill_id === filter);
  const notMine = Boolean(filter) && Boolean(skills.data) && !ownedSkill;
  const chosenSkill = skillId || ownedSkill?.skill_id || "";

  const create = useCreateTestCase();

  return (
    <section>
      <h1>Test Case</h1>
      {filter && <SkillWorkspaceNav skillId={filter} versionId={version} />}
      <p className="note" data-role="teaching">
        Test Case 是可編輯的草稿：User Prompt、測試資料與驗收條件。
      </p>

      <h2>既有的 Test Case</h2>
      <ExistingTestCasesSection
        filter={filter}
        version={version}
        ownedSkillName={ownedSkill?.name}
        notMine={notMine}
        isPending={testCases.isPending}
        error={testCases.error}
        rows={rows}
        hasNextPage={testCases.hasNextPage}
        isFetchingNextPage={testCases.isFetchingNextPage}
        onFetchNextPage={() => testCases.fetchNextPage()}
      />

      <h2>建立新的 Test Case</h2>
      <CreateTestCaseForm
        skills={skills.data}
        skillsError={skills.error}
        chosenSkill={chosenSkill}
        onSkillChange={setSkillId}
        name={name}
        onNameChange={setName}
        prompt={prompt}
        onPromptChange={setPrompt}
        create={create}
        onCreated={(testCaseId) =>
          navigate({
            to: "/lab/test-cases/$testCaseId",
            params: { testCaseId },
            search: { version: chosenSkill === filter ? version : undefined },
          })
        }
      />
    </section>
  );
}
