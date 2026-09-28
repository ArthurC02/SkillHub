import { Loading } from "../../../shared/ui/Loading";
import { LoginRequired, ReadFailure } from "../../../shared/ui/LoginRequired";
import { unauthenticated } from "../../../shared/ui/LoginRequired.model";
import { useMe } from "../../../core/session/me.service";
import { Link, useSearch } from "@tanstack/react-router";
import { useState } from "react";
import { useConfirmAndStartRun, usePreflight } from "../lab.service";
import { useOwnSkills } from "../../skill";
import { useTestCase } from "../testcases.service";
import { PreflightShell } from "./components/PreflightShell";
import { PreflightFacts } from "./components/PreflightFacts";
import { RunStartControl } from "./components/RunStartControl";

type LabSearch = { skill?: string; version?: string; test_case?: string };

export function RunPreflight() {
  const {
    skill = "",
    version: linkedVersion = "",
    test_case: testCase = "",
  } = useSearch({ strict: false }) as LabSearch;
  // Search params change without remounting the route; the key starts a fresh form.
  return (
    <Preflight
      key={`${skill}|${linkedVersion}|${testCase}`}
      skill={skill}
      linkedVersion={linkedVersion}
      testCase={testCase}
    />
  );
}

function Preflight({
  skill,
  linkedVersion,
  testCase,
}: {
  skill: string;
  linkedVersion: string;
  testCase: string;
}) {
  const [picked, setPicked] = useState("");
  const version = picked || linkedVersion;
  const ready = skill !== "" && testCase !== "";
  const me = useMe();
  const testCaseInfo = useTestCase(testCase);
  const ownSkills = useOwnSkills();
  const preflight = usePreflight(skill, version, testCase, ready && version !== "");
  const start = useConfirmAndStartRun(skill, version, testCase);

  if (unauthenticated(me.error)) {
    return (
      <section>
        <h1>執行前權限確認</h1>
        <LoginRequired what="試跑與執行前權限確認" />
      </section>
    );
  }

  if (!ready) {
    return (
      <section>
        <h1>執行前權限確認</h1>
        <p>
          {skill !== ""
            ? "還要先挑一個 Test Case，才知道這次要跑哪一段題目。"
            : "要先挑一個 Skill 與一個 Test Case。"}
          {" 到 "}
          <Link to="/lab/test-cases" search={{ skill: skill || undefined }}>
            Test Case 頁
          </Link>
          建立或選一個,再從那裡連過來;要跑哪一個 Skill Version 在這個頁面上選。
        </p>
      </section>
    );
  }

  const skillName = ownSkills.data?.skills.find((sk) => sk.skill_id === skill)?.name;
  const criteria = testCaseInfo.data?.acceptance_criteria.length;

  const shellProps = {
    skill,
    version,
    skillName,
    ownSkills,
    testCaseInfo,
    criteria,
    onPick: (id: string) => {
      setPicked(id);
      start.reset();
    },
  };

  if (version === "")
    return (
      <PreflightShell {...shellProps}>
        <p>請先在上面選一個 Skill Version,才有權限摘要可以看。</p>
      </PreflightShell>
    );
  if (preflight.isPending)
    return (
      <PreflightShell {...shellProps}>
        <Loading what="權限摘要" />
      </PreflightShell>
    );
  if (preflight.error) {
    return (
      <PreflightShell {...shellProps}>
        <ReadFailure error={preflight.error} what="權限摘要">
          <p role="alert">無法讀取權限摘要:{preflight.error.message}</p>
        </ReadFailure>
      </PreflightShell>
    );
  }

  const {
    summary,
    summary_hash: hash,
    estimated_cost: cost,
    quota,
    blocked,
    notes,
  } = preflight.data;

  return (
    <PreflightShell {...shellProps}>
      <p>以下是這次 Run 可以接觸的範圍。確認後才會開始執行。</p>

      <PreflightFacts summary={summary} cost={cost} quota={quota} />

      {notes.map((n) => (
        <p key={n} className="notice">
          {n}
        </p>
      ))}

      <RunStartControl start={start} hash={hash} blocked={blocked} />
    </PreflightShell>
  );
}
