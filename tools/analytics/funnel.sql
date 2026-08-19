-- The core funnel of 01 §11.2, as one query (O11Y-004's "查詢" half, ADR-029
-- 決策 6). Seven segments, run against the platform database:
--
--   psql -v ON_ERROR_STOP=1 -v from="'2026-09-01'" -v to="'2026-09-15'" \
--        -f tools/analytics/funnel.sql
--
-- A script and not a dashboard, and not an endpoint. ADR-029 決策 6 settled the
-- read side as a back-office query: the beta cohort is twelve people, the report
-- is read a handful of times, and a service nobody calls is a service somebody
-- has to keep working. BETA-002's numbers come from here.
--
-- **Analytics is never the source of truth** (ADR-029 決策 1). Four segments have
-- no domain table that can answer them — they happen before a login, or before
-- anything is written — and those four read `analytics_events`. Every segment
-- about a Run, an evaluation, a suggestion or a download reads the domain table
-- that owns the fact. Where both appear in one row, the numerator is always the
-- domain table's: `download_started` counts intentions, `download_records` counts
-- downloads, and only the second one is a fact.
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

\set QUIET on
\set from '''-infinity'''
\set to '''infinity'''
\set QUIET off

WITH bounds AS (
    SELECT :from::timestamptz AS lo, :to::timestamptz AS hi
),

-- --- the analytics half: the four segments no domain table can answer ---------
searched AS (
    SELECT DISTINCT session_id FROM analytics_events, bounds
    WHERE event_name = 'search_performed' AND occurred_at >= lo AND occurred_at < hi
),
viewed AS (
    SELECT DISTINCT session_id FROM analytics_events, bounds
    WHERE event_name = 'skill_detail_viewed' AND occurred_at >= lo AND occurred_at < hi
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
download_intents AS (
    SELECT DISTINCT workspace_id FROM analytics_events, bounds
    WHERE event_name = 'download_started' AND workspace_id IS NOT NULL
      AND occurred_at >= lo AND occurred_at < hi
),
session_workspaces AS (
    SELECT DISTINCT session_id, workspace_id,
           date_trunc('day', occurred_at AT TIME ZONE 'UTC') AS visit_day
    FROM analytics_events, bounds
    WHERE workspace_id IS NOT NULL AND occurred_at >= lo AND occurred_at < hi
),
-- Segment 7 asks who came back. session_started is a best-effort browser visit
-- marker; a signed-in event maps only that same UTC visit-day to its workspace. A
-- browser that later changes account therefore cannot rewrite earlier visits.
returning_workspaces AS (
    SELECT sw.workspace_id FROM analytics_events e
    JOIN session_workspaces sw ON sw.session_id = e.session_id
      AND sw.visit_day = date_trunc('day', e.occurred_at AT TIME ZONE 'UTC'), bounds
    WHERE e.event_name = 'session_started'
      AND e.occurred_at >= lo AND e.occurred_at < hi
    GROUP BY sw.workspace_id
    HAVING count(DISTINCT date_trunc('day', e.occurred_at AT TIME ZONE 'UTC')) > 1
),

-- --- the domain half: every fact about a Run, a verdict or a file -------------
forked AS (
    SELECT DISTINCT workspace_id FROM skills, bounds
    WHERE forked_from_skill_id IS NOT NULL AND created_at >= lo AND created_at < hi
),
ran AS (
    SELECT DISTINCT workspace_id FROM runs, bounds
    WHERE created_at >= lo AND created_at < hi
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
feedback_helpful AS (SELECT count(*) AS n FROM evaluations_current WHERE feedback_helpful),
suggestions_decided AS (
    SELECT count(*) AS n FROM evaluation_suggestions, bounds
    WHERE decision <> 'pending' AND created_at >= lo AND created_at < hi
),
suggestions_accepted AS (
    SELECT count(*) AS n FROM evaluation_suggestions, bounds
    WHERE decision = 'accepted' AND created_at >= lo AND created_at < hi
),
downloaded AS (
    SELECT DISTINCT workspace_id FROM download_records, bounds
    WHERE downloaded_at >= lo AND downloaded_at < hi
)

SELECT * FROM (
    VALUES
    (1, '輸入意圖後查看至少一個 Skill 詳情',
     (SELECT count(*) FROM viewed WHERE session_id IN (SELECT session_id FROM searched)),
     (SELECT count(*) FROM searched),
     'analytics; per session. One person on two devices, or who cleared the cookie, ' ||
     'counts twice, so this denominator is systematically high. Read as a magnitude.'),

    (2, '查看詳情後 Fork 或啟動試跑',
     (SELECT count(*) FROM viewing_workspaces w
      WHERE w.workspace_id IN (SELECT workspace_id FROM forked)
         OR w.workspace_id IN (SELECT workspace_id FROM ran)),
     (SELECT count(*) FROM viewing_workspaces),
     'analytics denominator, domain numerator; per workspace. Anonymous detail ' ||
     'views are excluded because nothing they did next is visible — so this ' ||
     'denominator is lower than segment 1''s and the two do not chain arithmetically.'),

    (3, '建立 Run 後成功完成',
     (SELECT n FROM runs_succeeded), (SELECT n FROM runs_created),
     'domain only. `succeeded` is EXECUTION, never a task verdict (ADR-025): a run ' ||
     'that finished and produced nothing useful is counted here as a success.'),

    (4, '完成 Run 後認為結果有幫助',
     (SELECT n FROM feedback_helpful), (SELECT n FROM feedback_given),
     'domain only; denominator is evaluations that got any answer, not all ' ||
     'evaluations. Volunteered feedback skews to the two extremes.'),

    (5, '改善建議被採用',
     (SELECT n FROM suggestions_accepted), (SELECT n FROM suggestions_decided),
     'domain only; denominator excludes `pending`, so a user who ignored every ' ||
     'suggestion is absent rather than counted as a rejection.'),

    (6, '完成試跑後打包下載',
     (SELECT count(*) FROM downloaded), (SELECT count(*) FROM ran),
     'domain numerator, domain denominator; per workspace. `download_started` is ' ||
     'NOT used here — it records that somebody pressed the button, and the four ' ||
     'locks may still have refused. Compare the two to see refusals: ' ||
     (SELECT count(*) FROM download_intents)::text || ' workspaces started a download.'),

    (7, '首次使用後再次回來',
     (SELECT count(*) FROM returning_workspaces),
     (SELECT count(DISTINCT sw.workspace_id) FROM analytics_events e
      JOIN session_workspaces sw ON sw.session_id = e.session_id
        AND sw.visit_day = date_trunc('day', e.occurred_at AT TIME ZONE 'UTC'), bounds
      WHERE e.event_name = 'session_started'
        AND e.occurred_at >= lo AND e.occurred_at < hi),
     'analytics; per workspace, "came back" = sessions on two distinct days. A ' ||
     'cleared cookie makes one returning visitor look like two new ones, so this ' ||
     'is a floor, not an estimate.')
) AS f(segment, description, numerator, denominator, note);
