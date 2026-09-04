-- 資訊架構 IA-11 (2026-09-04): which build the reporter was looking at.
--
-- A closed-beta report of 「這一頁怪怪的」 with no build behind it cannot be
-- reproduced, and 04 already records one incident where the same commit was
-- green on one OS and red on another. The value is the identifier the page
-- prints in its own footer (the build-time VITE_BUILD_ID: a commit SHA prefix
-- in CI, a labelled local build otherwise), sent automatically by the feedback
-- form. It identifies the software, not the person — the beta-design §5 rule
-- against automatic capture is about what a reporter cannot see, and this is
-- a field on every page. Bounded like page_path: the check is the contract's
-- maxLength, so an oversized value is refused the same way wherever it arrives.
ALTER TABLE feedback_reports
    ADD COLUMN build_id text CHECK (build_id IS NULL OR length(build_id) <= 64);
