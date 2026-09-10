import { Link } from "@tanstack/react-router";
import { useGenerateEntryPoint } from "../api/generate";
import { useCreationEntryPoint } from "../api/creation";
import { GenerateSkill } from "../components/GenerateSkill";
import { CreationSession } from "../components/CreationSession";

export function CreateSkill() {
  const generateExposed = useGenerateEntryPoint();
  const creationExposed = useCreationEntryPoint();

  if (!generateExposed) {
    return (
      <>
        <h1>這一頁現在不存在</h1>
        <p>
          你可以回到 <Link to="/workspace/skills">我的 Skill</Link>，或到{" "}
          <Link to="/" search={{}}>
            目錄
          </Link>{" "}
          看看已經有的。
        </p>
      </>
    );
  }

  return (
    <>
      {creationExposed ? (
        <CreationSession />
      ) : (
        <>
          <nav>
            <Link to="/workspace/skills">← 回到我的 Skill</Link>
          </nav>
          <GenerateSkill />
        </>
      )}
    </>
  );
}
