import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Findings } from "../components/Findings";
import { LoginRequired, ReadFailure, unauthenticated } from "../components/LoginRequired";
import { useMe } from "../api/me";
import { ApiError } from "../api/client";
import {
  importSkillFromURL,
  isCategorizedFindings,
  uploadSkillPackage,
  type CategorizedFindings,
  type ImportResult,
} from "../api/import";

/**
 * 02:SKILL-002 / INGEST-008 — the one screen third-party code enters the
 * platform through, which makes it where design §1.1 costs the most: 每個畫面必須
 * 讓證據看得見，也必須讓證據的缺席看得見. Three of those absences rendered as blank
 * space here, and blank reads as 通過:
 *
 * 1. **A rejection never said it was rejected.** The 422 path suppressed the
 *    generic alert, so a blocked package produced an `<h2>阻擋錯誤</h2>` and a
 *    list — the words 匯入失敗 and 未匯入 appeared nowhere, nothing was in a live
 *    region, and the `<h1>` still read 匯入 Skill, which names the widget rather
 *    than answering. Restoring that alert alone would not have fixed it: a 422
 *    carries `CategorizedFindings` and no `error` key, so `ApiError.message`
 *    falls back to `response.statusText` and the line would have read
 *    「匯入失敗：Unprocessable Entity」. The rejection states the three things a
 *    rejection owes the reader instead — what was rejected, that nothing was
 *    imported, and what to do next.
 * 2. **A clean scan rendered as silence** (§2.1). Every empty group returned
 *    `null`, so a package with no findings showed 匯入完成 and nothing else,
 *    which is indistinguishable from a page that never scanned. The absence is
 *    written now, including what the scan is *not*: not a human review, and not
 *    a signature check — MVP has no signature mechanism to run (ADR-027 決策 3
 *    is an explicit 不做, not a pending item), so that gap is permanent until
 *    that ADR is reopened and the sentence says so rather than implying a check
 *    that is merely late.
 * 3. **`details` was carried and never rendered** (§1.1). One `external-url`
 *    finding aggregates per host and holds every `<file>: <url>` reference, so
 *    the page could say 「320 external URL」 with no way to see which. It folds
 *    into a `<details>` (§2.6). Those are paths the scanner derived and URLs it
 *    parsed out — never file contents, which 02:NFR-002 keeps off this path.
 */
export function ImportSkill() {
  const queryClient = useQueryClient();
  // 資訊架構 §5 IA-6 called this page's shape the worst in the audit: radio
  // buttons, a URL field, a file picker and an enabled 「開始匯入」 served to a
  // visitor who is refused only after choosing a file. 設計系統 §2.2「顯示與強制
  // 成對」and §2.4 both say the refusal has to be stated BEFORE the control is
  // used, not after.
  //
  // WorkspaceAccount's `useMe()` precedent (the whole query object), not
  // Home.tsx's `!!useMe().data`: that one reads 「還在問」 as 「沒登入」, which is
  // harmless when it swaps one sentence inside a paragraph and is a flash of a
  // false statement when it replaces a whole form. Only a resolved 401 does it.
  const me = useMe();
  const [source, setSource] = useState<"url" | "upload">("url");
  const [url, setURL] = useState("");
  const [file, setFile] = useState<File>();
  const [result, setResult] = useState<ImportResult>();
  const [rejected, setRejected] = useState<CategorizedFindings>();

  const mutation = useMutation({
    mutationFn: () => {
      if (source === "url") return importSkillFromURL(url.trim());
      if (!file) return Promise.reject(new Error("請選擇 zip 套件。"));
      return uploadSkillPackage(file);
    },
    onSuccess: async (data) => {
      setRejected(undefined);
      setResult(data);
      await queryClient.invalidateQueries({ queryKey: ["own-skills"] });
    },
    onError: (error) => {
      setResult(undefined);
      setRejected(
        error instanceof ApiError && isCategorizedFindings(error.body) ? error.body : undefined,
      );
    },
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    mutation.mutate();
  };

  return (
    <section>
      <h1>匯入 Skill</h1>
      <p className="note">套件只會做靜態檢查；匯入期間不執行其中的 Script。</p>

      {/*
        設計 §2.2「強制但不顯示」, which that section calls the second worst of the
        three and names this page as its instance: the platform enforces five
        things here and this screen had **not one number and not one rule** on
        it. 「GitHub 或允許的 URL」 below points at a whitelist and then does not
        show it. A user meets all five in a 4xx.
        <br />
        WHAT IS AND IS NOT STATED, and why the split is where it is:
        - **The rules are here**, because they are product decisions (PDM-002
          makes GitHub the first-batch source) rather than tunable values, so
          stating them creates no second copy that can drift.
        - **The numbers are not**, and their absence is stated rather than left
          blank (§2.9). There is no `GET /skills/import/limits` in
          `contracts/openapi/public.yaml` — `pages/DatasetUpload.tsx` has exactly
          that endpoint and does this properly, fail-closed, with a `<dl>` before
          the file input. Copying 10 MB / 100 MB / 1 MiB into this file would be
          the failure 04 乙-2 rules on from the other direction: a number the page
          asserts and nothing keeps true. Handed off; until it lands, the honest
          sentence.
      */}
      <ul className="note">
        <li>來源限 GitHub（PDM-002 的首批來源），其他網域一律拒絕。</li>
        <li>網址必須是 https，而且不得帶帳號密碼、查詢字串或錨點。</li>
        <li>
          大小上限見拒絕訊息——平台強制 zip 與解壓後的兩個上限，但這一頁還讀不到它們的值，
          所以這裡不印一個沒有來源的數字。
        </li>
        {/*
          設計 §2.2 第二向「強制但不顯示」，checklist 第 11 條「會擋住人的限制在他撞上
          之前看得見嗎」。上面兩條規則只管 URL；切到「上傳 zip」時它們都不適用，而**套件
          本身的結構要求在送出之前一個字都沒有**——第一次的人是在 422 裡才讀到
          `skill-md-missing: SKILL.md not found at package root`。
          這一條不撞上 §2.2 的另一半（不得印沒有來源的數字）：它不是數字，是產品決策，
          正是這一頁在上面幾行寫下的「rules are here, numbers are not」的同一側。
        */}
        <li>
          zip 的最上層（或單一頂層資料夾）要有 <code>SKILL.md</code>，而且它的 frontmatter 要有{" "}
          <code>name</code> 與 <code>description</code>——名稱、描述與 License 都從那裡讀，
          不必在這一頁手打。
        </li>
      </ul>
      {unauthenticated(me.error) ? (
        // Replaced rather than disabled: §2.4's fourth shape is a control taken
        // away with no reason given, and this sentence is the reason.
        <LoginRequired what="匯入 Skill" />
      ) : (
        <form onSubmit={submit}>
          <fieldset>
            <legend>來源</legend>
            <label>
              <input
                type="radio"
                name="skill-import-source"
                checked={source === "url"}
                onChange={() => setSource("url")}
              />
              GitHub 或允許的 URL
            </label>{" "}
            <label>
              <input
                type="radio"
                name="skill-import-source"
                checked={source === "upload"}
                onChange={() => setSource("upload")}
              />
              上傳 zip
            </label>
          </fieldset>
          {/*
          `.field` rather than a bare `<p>`: an inline `<label> <input>` gets the
          UA's intrinsic ~180px on a 1126px page, so a GitHub URL was unreadable
          in the field you paste it into — the highest-stakes input in the app.
          Same defect index.css records fixing for the Home search box, and the
          class already exists for exactly this (design §4.5, checklist 8).
        */}
          {source === "url" ? (
            <p className="field">
              <label htmlFor="skill-import-url">URL</label>
              <input
                id="skill-import-url"
                type="url"
                required
                value={url}
                onChange={(event) => setURL(event.target.value)}
              />
            </p>
          ) : (
            <p className="field">
              <label htmlFor="skill-import-file">Skill zip</label>
              <input
                id="skill-import-file"
                type="file"
                required
                accept=".zip,application/zip"
                onChange={(event) => setFile(event.target.files?.[0])}
              />
            </p>
          )}
          {/* 設計 §4.6.3（ADR-064）的表，`/workspace/import` 那一列：這一頁只做
              一件事，這顆按鈕就是那件事。 */}
          <button type="submit" className="action" disabled={mutation.isPending}>
            {mutation.isPending ? "匯入中…" : "開始匯入"}
          </button>
          {/*
            設計 §2.12 第 2 條：進行中的畫面要說出在哪一步、會不會自己結束、能不能離開。
            在此之前這一整段的變化只有按鈕上的四個字，而 URL 匯入這段時間平台正在
            GitHub 那一端抓一個最大 32 MB 的 zip、解壓、逐檔靜態掃描，**而且它是同步
            請求——關掉分頁就等於取消**。旗標後面的生成入口對同一個事實寫了一整塊
            `GenerateInFlight`；MVP 真正要用的這條路沒有。
            刻意不寫耗時數字：生成那塊敢寫「十幾秒到一分鐘」是因為量過十次，匯入沒有
            同等的量測，而 §2.2 不准印一個沒有來源的數字。
          */}
          {mutation.isPending && (
            <p role="status" className="note">
              正在取得套件並逐檔用靜態檢查看過它——這一步會自己結束，不需要你再按任何東西。
              <strong>請先不要關掉這個分頁</strong>：匯入是一個同步請求，關掉等於取消，
              而取消不會在你的工作區留下半成品版本。
            </p>
          )}
        </form>
      )}

      {/* A failure the server did not categorise: no findings to show, so a
          page-owned sentence by status is all there is. The categorised 422
          renders below instead (丙-150: no more raw err.message). */}
      {mutation.error && !rejected && (
        <ReadFailure error={mutation.error} what="匯入 Skill">
          <p role="alert">
            {mutation.error instanceof ApiError && mutation.error.status === 400
              ? "這個檔案不是可用的 zip 套件，或網址抓不到內容。"
              : mutation.error instanceof ApiError && mutation.error.status === 413
                ? "檔案超過上限。"
                : "匯入失敗，可以再按一次。"}
          </p>
        </ReadFailure>
      )}

      {rejected && (
        <section role="alert">
          <h2>匯入失敗：套件被擋下，沒有匯入任何東西</h2>
          <p>
            這個工作區沒有新增任何 Skill，也沒有建立新版本。
            下面每一則阻擋錯誤都要在套件裡修掉，再重新匯入一次；警告與資訊不擋匯入，一併列在後面。
          </p>
          <Findings findings={rejected} />
        </section>
      )}

      {result && (
        <>
          <div role="status" className="notice">
            <p>
              {result.duplicate ? "相同內容已存在，沿用既有版本。" : "匯入完成。"}版本 #
              {result.version_number}
            </p>
            <Link to="/skills/$skillId" params={{ skillId: result.skill_id }}>
              查看 Skill
            </Link>
          </div>
          {/* The three groups used to be `h2` siblings of the page `h1`, i.e.
              children of a section nobody had named. Named here, and the groups
              demoted to `h3` under it (checklist 6; axe never sees this — it
              fails a skipped level, not a level that should have gone down). */}
          <section>
            <h2>靜態檢查結果</h2>
            <Findings findings={result.findings} />
          </section>
        </>
      )}
    </section>
  );
}
