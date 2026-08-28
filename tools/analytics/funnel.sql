-- The core funnel of 01 §11.2, as one query (O11Y-004's "查詢" half, ADR-029
-- 決策 6). Seven segments, run against the platform database:
--
--   psql -v ON_ERROR_STOP=1 \
--        -v from="'2026-09-01T00:00:00Z'" -v to="'2026-09-15T00:00:00Z'" \
--        -f tools/analytics/funnel.sql
--
-- The range is half-open, [from, to), and both bounds must name their zone. A
-- bare date is resolved with the server's `TimeZone`, while every day bucket
-- below is `AT TIME ZONE 'UTC'` — two reports run from two psql sessions would
-- otherwise disagree by a few hours and neither would say so. Omit either `-v`
-- and that side is unbounded; omit both and the report is all-time, which
-- includes every integration-test row the database has ever seen.
--
-- A script and not a dashboard, and not an endpoint. ADR-029 決策 6 settled the
-- read side as a back-office query: the beta cohort is twelve people, the report
-- is read a handful of times, and a service nobody calls is a service somebody
-- has to keep working. BETA-002's numbers come from here.
--
-- **Analytics is never the source of truth** (ADR-029 決策 1). Only what happens
-- before a login — or before anything is written — reads `analytics_events`:
-- segment 1 entirely, and segment 2's denominator. Every segment about a Run, an
-- evaluation, a suggestion or a download reads the domain table that owns the
-- fact, segment 7 included: 01 §11.2's seventh item asks who came back to run or
-- re-verify, and a browser visit is not that. Where both appear in one row, the
-- numerator is always the domain table's: `download_started` counts intentions,
-- `download_records` counts downloads, and only the second one is a fact.
--
-- **Every row carries its own precision limit** (02:O11Y-004 last clause). Not a
-- footnote in a document somebody may not have open: a percentage without its
-- caveat reads as a measurement, and these are magnitudes. The `note` column is
-- part of the result set so a report cannot be produced without it.
--
-- The joining key is deliberately weak and says so. Segment 1 is per analytics
-- session, because before login there is nothing else; segments 2 onward are per
-- workspace, because that is what a domain row carries. A person who searched
-- anonymously and then signed in is two identifiers, and nothing here pretends
-- otherwise (ADR-029 決策 4 forbids the linkage that would fix it).
--
-- ponytail: two population units in one table, joined by nothing. Upgrade path if
-- the beta ever needs a true per-person funnel: stamp the analytics session onto
-- the login event and follow it forward — which is a product decision about
-- identity, not a query change.

-- `\set` is assignment, not defaulting: setting both unconditionally threw away
-- whatever `-v` passed and made every report all-time (M5 audit, 2026-08-25).
\set QUIET on
\if :{?from}
\else
\set from '''-infinity'''
\endif
\if :{?to}
\else
\set to '''infinity'''
\endif
\set QUIET off

WITH bounds AS (
    SELECT :from::timestamptz AS lo, :to::timestamptz AS hi
),

-- --- the analytics half: what happens before a domain row exists -------------
searched AS (
    SELECT DISTINCT session_id FROM analytics_events, bounds
    WHERE event_name = 'search_performed' AND occurred_at >= lo AND occurred_at < hi
),
-- Ordered, not a set intersection. 01 §11.2's first item is a detail view that
-- happened *after* an intent: a session that opened a shared /skills/{id} link at
-- 11:00, searched fruitlessly at 11:05 and left is not a conversion, and counting
-- it as one moves the exact number the M5 exposure boundary is waiting on.
viewed_after_search AS (
    SELECT DISTINCT v.session_id FROM analytics_events v, bounds
    WHERE v.event_name = 'skill_detail_viewed'
      AND v.occurred_at >= lo AND v.occurred_at < hi
      AND EXISTS (
          SELECT 1 FROM analytics_events s
          WHERE s.session_id = v.session_id
            AND s.event_name = 'search_performed'
            AND s.occurred_at <= v.occurred_at
            AND s.occurred_at >= lo AND s.occurred_at < hi
      )
),
-- The workspaces that reached a detail page. NULL workspace_id is an anonymous
-- view and is excluded here on purpose: segment 2 is about what somebody did
-- next, and somebody who never signed in did nothing next that a domain table
-- can see.
viewing_workspaces AS (
    SELECT DISTINCT workspace_id FROM analytics_events, bounds
    WHERE event_name = 'skill_detail_viewed' AND workspace_id IS NOT NULL
      AND occurred_at >= lo AND occurred_at < hi
),
-- Ordered, for segment 1's reason: 01 §11.2's second item is a fork or a trial
-- that followed the detail view. A workspace that forked at 09:00 and opened a
-- detail page at 17:00 did nothing "next", and a set intersection counts it.
acted_after_view AS (
    SELECT DISTINCT v.workspace_id FROM analytics_events v, bounds
    WHERE v.event_name = 'skill_detail_viewed' AND v.workspace_id IS NOT NULL
      AND v.occurred_at >= lo AND v.occurred_at < hi
      AND (EXISTS (
              SELECT 1 FROM skills s
              WHERE s.workspace_id = v.workspace_id
                AND s.forked_from_skill_id IS NOT NULL
                AND s.created_at >= v.occurred_at
                AND s.created_at >= lo AND s.created_at < hi
          )
        OR EXISTS (
              SELECT 1 FROM runs r
              WHERE r.workspace_id = v.workspace_id
                AND r.created_at >= v.occurred_at
                AND r.created_at >= lo AND r.created_at < hi
          ))
),
download_intents AS (
    SELECT DISTINCT workspace_id FROM analytics_events, bounds
    WHERE event_name = 'download_started' AND workspace_id IS NOT NULL
      AND occurred_at >= lo AND occurred_at < hi
),

-- --- the domain half: every fact about a Run, a verdict or a file -------------
-- Segment 6's denominator: 01 §11.2 says "完成試跑後", and a created Run is not a
-- completed one. Same column as segment 3's numerator so the two rows agree about
-- what "completed" means.
succeeded_workspaces AS (
    SELECT DISTINCT workspace_id FROM runs, bounds
    WHERE status = 'succeeded' AND created_at >= lo AND created_at < hi
),
-- Segment 7 is a domain question, not a browser one: 01 §11.2 asks who came back
-- to 「試跑或重新驗證」, so a day counts when a workspace created a Run or an
-- evaluation. Opening the catalogue for one second on day two is not that, and
-- the number is read as retention.
activity_days AS (
    SELECT workspace_id, date_trunc('day', created_at AT TIME ZONE 'UTC') AS day
    FROM runs, bounds WHERE created_at >= lo AND created_at < hi
    UNION
    SELECT workspace_id, date_trunc('day', created_at AT TIME ZONE 'UTC')
    FROM evaluations, bounds WHERE created_at >= lo AND created_at < hi
),
first_use_workspaces AS (SELECT DISTINCT workspace_id FROM activity_days),
returning_workspaces AS (
    SELECT workspace_id FROM activity_days
    GROUP BY workspace_id HAVING count(DISTINCT day) > 1
),
runs_created AS (
    SELECT count(*) AS n FROM runs, bounds WHERE created_at >= lo AND created_at < hi
),
runs_succeeded AS (
    SELECT count(*) AS n FROM runs, bounds
    WHERE status = 'succeeded' AND created_at >= lo AND created_at < hi
),
-- Only the current revision of each evaluation: ADR-026 makes re-evaluation
-- append-only, and counting superseded rows would count one run twice.
evaluations_current AS (
    SELECT e.* FROM evaluations e, bounds
    WHERE e.superseded_at IS NULL AND e.created_at >= lo AND e.created_at < hi
),
feedback_given AS (SELECT count(*) AS n FROM evaluations_current WHERE feedback_helpful IS NOT NULL),
-- 01 §11.2's fourth item counts completed Runs, not answered questionnaires. The
-- denominator was the answers, which measured "of the people who bothered to
-- reply, how many were happy" — a different quantity, and a flattering one.
helpful_runs AS (
    SELECT count(*) AS n FROM runs r, bounds
    WHERE r.status = 'succeeded' AND r.created_at >= lo AND r.created_at < hi
      AND EXISTS (
          SELECT 1 FROM evaluations_current e
          WHERE e.run_id = r.id AND e.feedback_helpful
      )
),
suggestions_decided AS (
    SELECT count(*) AS n FROM evaluation_suggestions, bounds
    WHERE decision <> 'pending' AND created_at >= lo AND created_at < hi
),
suggestions_accepted AS (
    SELECT count(*) AS n FROM evaluation_suggestions, bounds
    WHERE decision = 'accepted' AND created_at >= lo AND created_at < hi
),
-- 01 §11.2's sixth item is 「完成試跑後打包下載」, so the download has to follow the
-- Run. COALESCE is not caution: `runs.finished_at` is nullable and no CHECK ties
-- it to `status = 'succeeded'`, so a succeeded run with no finish time would
-- otherwise drop out of the numerator entirely.
downloaded_after_success AS (
    SELECT DISTINCT r.workspace_id FROM runs r, bounds
    WHERE r.status = 'succeeded' AND r.created_at >= lo AND r.created_at < hi
      AND EXISTS (
          SELECT 1 FROM download_records d
          WHERE d.workspace_id = r.workspace_id
            AND d.downloaded_at >= COALESCE(r.finished_at, r.created_at)
            AND d.downloaded_at >= lo AND d.downloaded_at < hi
      )
)

SELECT * FROM (
    VALUES
    (1, '輸入意圖後查看至少一個 Skill 詳情',
     (SELECT count(*) FROM viewed_after_search),
     (SELECT count(*) FROM searched),
     'analytics; per session, and the detail view must come after the search. One ' ||
     'person on two devices, or who cleared the cookie, counts twice, so this ' ||
     'denominator is systematically high. Read as a magnitude.'),

    (2, '查看詳情後 Fork 或啟動試跑',
     (SELECT count(*) FROM acted_after_view),
     (SELECT count(*) FROM viewing_workspaces),
     'analytics denominator, domain numerator; per workspace, and the fork or the ' ||
     'Run must come after the detail view. Anonymous detail ' ||
     'views are excluded because nothing they did next is visible — so this ' ||
     'denominator is lower than segment 1''s and the two do not chain arithmetically.'),

    (3, '建立 Run 後成功完成',
     (SELECT n FROM runs_succeeded), (SELECT n FROM runs_created),
     'domain only. `succeeded` is EXECUTION, never a task verdict (ADR-025): a run ' ||
     'that finished and produced nothing useful is counted here as a success.'),

    (4, '完成 Run 後認為結果有幫助',
     (SELECT n FROM helpful_runs), (SELECT n FROM runs_succeeded),
     'domain only; per completed Run, which is what 01 §11.2 asks for — so a Run ' ||
     'whose owner never answered sits in the denominator and this is a floor, not ' ||
     'a satisfaction rate. Volunteered feedback skews to the two extremes. For ' ||
     'contrast, ' || (SELECT n FROM feedback_given)::text ||
     ' evaluations got an answer of any kind.'),

    (5, '改善建議被採用',
     (SELECT n FROM suggestions_accepted), (SELECT n FROM suggestions_decided),
     'domain only; denominator excludes `pending`, so a user who ignored every ' ||
     'suggestion is absent rather than counted as a rejection.'),

    (6, '完成試跑後打包下載',
     (SELECT count(*) FROM downloaded_after_success),
     (SELECT count(*) FROM succeeded_workspaces),
     'domain numerator, domain denominator; per workspace, the download must come ' ||
     'after the Run finished, and the numerator is a ' ||
     'subset of the denominator by construction. A workspace whose Run succeeded ' ||
     'inside the window but downloaded after it closes counts as no download. ' ||
     '`download_started` is ' ||
     'NOT used here — it records that somebody pressed the button, and the four ' ||
     'locks may still have refused. Compare the two to see refusals: ' ||
     (SELECT count(*) FROM download_intents)::text || ' workspaces started a download.'),

    (7, '首次使用後再次回來試跑或重新驗證',
     (SELECT count(*) FROM returning_workspaces),
     (SELECT count(*) FROM first_use_workspaces),
     'domain only; per workspace, "came back" = created a Run or an evaluation on ' ||
     'two distinct UTC days. Re-opening the catalogue is deliberately NOT counted: ' ||
     '01 §11.2 asks for a trial or a re-verification, and a browser-side "came ' ||
     'back at all" number gets read as retention. A person whose second day fell ' ||
     'outside the window is invisible here, so this is a floor.')
) AS f(segment, description, numerator, denominator, note);
