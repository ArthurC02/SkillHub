import { useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useCreateTestCase, useTestCases } from "../testcases.service";
import { SkillWorkspaceNav, useOwnSkills } from "../../skill";
import { ExistingTestCasesSection } from "./components/ExistingTestCasesSection";
import { CreateTestCaseForm } from "./components/CreateTestCaseForm";
import "./TestCaseList.page.css";

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
    <section className="test-case-list-page">
      <header className="test-case-list-header">
        <p className="page-eyebrow">Evaluation workspace</p>
        <h1>Test Case</h1>
        <p className="note" data-role="teaching">
          把任務提示、測試資料與驗收條件組成可反覆驗證的情境。
        </p>
        <a className="action-secondary test-case-create-jump" href="#new-test-case">
          建立 Test Case
        </a>
      </header>
      {filter && <SkillWorkspaceNav skillId={filter} versionId={version} />}

      <div className="test-case-workspace">
        <section className="test-case-index" aria-labelledby="existing-test-cases-heading">
          <p className="page-eyebrow">Scenario library · {rows.length}</p>
          <h2 id="existing-test-cases-heading">既有的 Test Case</h2>
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
        </section>

        <section
          className="test-case-editor"
          id="new-test-case"
          aria-labelledby="create-test-case-heading"
        >
          <p className="page-eyebrow">New scenario</p>
          <h2 id="create-test-case-heading">建立新的 Test Case</h2>
          <p>先定義要驗證的 Skill 與任務；建立後再補資料集、驗收條件與 Rubric。</p>
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
      </div>
    </section>
  );
}
