import { useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useCreateTestCase, useTestCases } from "../testcases.service";
import { SkillWorkspaceNav, useOwnSkills } from "../../skill";
import { ExistingTestCasesSection } from "./components/ExistingTestCasesSection";
import { CreateTestCaseForm } from "./components/CreateTestCaseForm";
import "./TestCaseList.page.css";
import { canRetainLoadedPage } from "../../../core/api/client";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { ListFreshness } from "../../../shared/ui/ListFreshness";

export function TestCaseList() {
  const navigate = useNavigate();
  const { skill: filter, version } = useSearch({ from: "/lab/test-cases" });
  const skills = useOwnSkills();
  const [skillId, setSkillId] = useState("");
  const [name, setName] = useState("");
  const [prompt, setPrompt] = useState("");
  const ownSkills = skills.error ? undefined : skills.data;
  const ownedSkill = ownSkills?.skills.find((s) => s.skill_id === filter);
  const notMine = Boolean(filter) && Boolean(ownSkills) && !ownedSkill;
  const chosenSkill = skillId || ownedSkill?.skill_id || "";

  const create = useCreateTestCase();

  return (
    <section className="test-case-list-page">
      <header className="test-case-list-header">
        <p className="page-eyebrow">Evaluation workspace</p>
        <h1>測試題</h1>
        <p className="note" data-role="teaching">
          把任務提示、測試資料與驗收條件組成可反覆驗證的情境。
        </p>
        <a className="action-secondary test-case-create-jump" href="#new-test-case">
          建立測試題
        </a>
      </header>
      {filter && <SkillWorkspaceNav skillId={filter} versionId={version} />}

      <div className="test-case-workspace">
        <TestCaseIndex
          filter={filter}
          version={version}
          ownedSkillName={ownedSkill?.name}
          notMine={notMine}
        />

        <section
          className="test-case-editor"
          id="new-test-case"
          aria-labelledby="create-test-case-heading"
        >
          <p className="page-eyebrow">New scenario</p>
          <h2 id="create-test-case-heading" tabIndex={-1}>
            建立新的測試題
          </h2>
          <p>先定義要驗證的小工具與任務；建立後再補資料集、驗收條件與 Rubric。</p>
          <CreateTestCaseForm
            skills={ownSkills}
            skillsError={skills.error}
            retrySkills={() => void skills.refetch()}
            retryingSkills={skills.isFetching}
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

function TestCaseIndex({
  filter,
  version,
  ownedSkillName,
  notMine,
}: {
  filter: string | undefined;
  version: string | undefined;
  ownedSkillName: string | undefined;
  notMine: boolean;
}) {
  const testCases = useTestCases(filter);
  const nextPageFailure = canRetainLoadedPage(testCases.error, testCases.isFetchNextPageError);
  const data = testCases.error && !nextPageFailure ? undefined : testCases.data;
  const rows = data?.pages.flatMap((page) => page.test_cases) ?? [];
  return (
    <section className="test-case-index" aria-labelledby="existing-test-cases-heading">
      <p className="page-eyebrow">
        Scenario library · {data ? `已載入 ${rows.length}` : "尚未讀取"}
      </p>
      <h2 id="existing-test-cases-heading" tabIndex={-1}>
        既有的測試題
      </h2>
      <ReadFailure
        error={testCases.error}
        what="測試題"
        retry={() => void (nextPageFailure ? testCases.fetchNextPage() : testCases.refetch())}
        retrying={testCases.isFetching}
        focusTarget="existing-test-cases-heading"
      >
        {nextPageFailure && (
          <p role="alert">更多測試題讀取失敗；以下僅保留已載入的紀錄，尚未讀完。請再試一次。</p>
        )}
      </ReadFailure>
      {data && !testCases.error && (
        <ListFreshness
          inFlight={false}
          showWhenIdle
          subject="測試題"
          updatedAt={testCases.dataUpdatedAt}
          fetching={testCases.isFetching && !testCases.isFetchingNextPage}
          refetch={async () => {
            const result = await testCases.refetch();
            if (result.error) document.getElementById("existing-test-cases-heading")?.focus();
          }}
        />
      )}
      <ExistingTestCasesSection
        filter={filter}
        version={version}
        ownedSkillName={ownedSkillName}
        notMine={notMine}
        isPending={testCases.isPending}
        error={nextPageFailure ? null : testCases.error}
        rows={rows}
        hasNextPage={testCases.hasNextPage && !testCases.error}
        isFetchingNextPage={testCases.isFetchingNextPage}
        onFetchNextPage={() => testCases.fetchNextPage()}
      />
    </section>
  );
}
