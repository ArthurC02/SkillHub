import { Loading } from "../../../shared/ui/Loading";
import { LoginRequired, ReadFailure } from "../../../shared/ui/LoginRequired";
import { unauthenticated } from "../../../shared/ui/LoginRequired.model";
import { useMe } from "../../../core/session/me.service";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useConfirmAndStartRun, usePreflight } from "../lab.service";
import { useSkillDetail, useSkillVersions } from "../../skill";
import { useTestCase } from "../testcases.service";
import { PreflightShell } from "./components/PreflightShell";
import { PreflightFacts } from "./components/PreflightFacts";
import { RunStartControl } from "./components/RunStartControl";
import "./RunPreflight.page.css";

type RunPreflightParams = { skillId?: string; testCaseId?: string };
type RunPreflightSearch = { version?: string };
type PreflightProps = { skill: string; linkedVersion: string; testCase: string };

export function RunPreflight() {
  const { skillId: skill = "", testCaseId: testCase = "" } = useParams({
    strict: false,
  }) as RunPreflightParams;
  const { version: linkedVersion = "" } = useSearch({ strict: false }) as RunPreflightSearch;
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

function Preflight({ skill, linkedVersion, testCase }: PreflightProps) {
  const navigate = useNavigate();
  const version = linkedVersion;
  const me = useMe();
  const testCaseInfo = useTestCase(testCase);
  const skillInfo = useSkillDetail(skill);
  const versions = useSkillVersions(skill);
  const contextMatches = testCaseInfo.data?.skill_id === skill;
  const versionKnown = versions.data?.versions.some((item) => item.version_id === version) ?? false;
  const preflight = usePreflight(skill, version, testCase, contextMatches && versionKnown);
  const start = useConfirmAndStartRun(skill, version, testCase);

  if (unauthenticated(me.error)) {
    return (
      <section>
        <h1>執行前權限確認</h1>
        <LoginRequired what="試跑與執行前權限確認" />
      </section>
    );
  }

  const criteria = testCaseInfo.data?.acceptance_criteria.length;

  const shellProps = {
    skill,
    version,
    skillInfo,
    testCaseInfo,
    criteria,
    onPick: (id: string) => {
      start.reset();
      void navigate({
        to: "/skills/$skillId/test-cases/$testCaseId/runs/new",
        params: { skillId: skill, testCaseId: testCase },
        search: { version: id },
      });
    },
  };

  if (testCaseInfo.isPending)
    return (
      <PreflightShell {...shellProps}>
        <Loading what="Test Case 脈絡" />
      </PreflightShell>
    );
  if (testCaseInfo.error)
    return (
      <PreflightShell {...shellProps}>
        <p className="note">Test Case 可讀取後，才會顯示這次 Run 的權限摘要。</p>
      </PreflightShell>
    );
  if (!contextMatches)
    return (
      <PreflightShell {...shellProps}>
        <ContextMismatch testCaseId={testCase} skillId={testCaseInfo.data.skill_id} />
      </PreflightShell>
    );
  if (version === "")
    return (
      <PreflightShell {...shellProps}>
        <p>請先在上面選一個 Skill Version，才有權限摘要可以看。</p>
      </PreflightShell>
    );
  if (versions.isPending)
    return (
      <PreflightShell {...shellProps}>
        <p className="note">正在確認這個 Version 是否屬於目前的 Skill。</p>
      </PreflightShell>
    );
  if (versions.error)
    return (
      <PreflightShell {...shellProps}>
        <p className="note">Version 清單可讀取後，才會顯示權限摘要。</p>
      </PreflightShell>
    );
  if (!versionKnown)
    return (
      <PreflightShell {...shellProps}>
        <p className="notice notice-danger" role="alert">
          這個 Version 不屬於這個 Skill。請從上面的清單改選；平台沒有讀取權限摘要，也不會開始 Run。
        </p>
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
        <ReadFailure error={preflight.error} what="權限摘要" />
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

function ContextMismatch({ testCaseId, skillId }: { testCaseId: string; skillId: string }) {
  return (
    <section className="notice notice-danger" role="alert">
      <h2>Skill 與 Test Case 不相符</h2>
      <p>平台沒有讀取權限摘要，也不會開始 Run。</p>
      <p>
        <Link
          to="/skills/$skillId/test-cases/$testCaseId/runs/new"
          params={{ skillId, testCaseId }}
          search={{ version: undefined }}
        >
          回到這個 Test Case 所屬的 Skill，再選擇 Version
        </Link>
      </p>
    </section>
  );
}
