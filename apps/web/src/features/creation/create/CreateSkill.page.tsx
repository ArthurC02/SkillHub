import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useGenerateEntryPoint } from "../generate.service";
import { useCreationEntryPoint } from "../creation.service";
import { GenerateSkill } from "../generate/GenerateSkill";
import { CreationSession } from "./components/CreationSession";

export function CreateSkill() {
  const generateExposed = useGenerateEntryPoint();
  const creationExposed = useCreationEntryPoint();
  const { session } = useSearch({ from: "/workspace/creations" });
  const navigate = useNavigate({ from: "/workspace/creations" });
  const selectSession = (id: string) => {
    void navigate({ search: id ? { session: id } : {} });
  };

  if (!generateExposed) {
    return (
      <>
        <h1>這一頁現在不存在</h1>
        <p>
          你可以回到 <Link to="/library">資產庫</Link>，或到{" "}
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
        <CreationSession sessionId={session} onSessionChange={selectSession} />
      ) : (
        <>
          <nav>
            <Link to="/library">← 回到資產庫</Link>
          </nav>
          <GenerateSkill />
        </>
      )}
    </>
  );
}
