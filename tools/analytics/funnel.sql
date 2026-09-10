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

searched AS (
    SELECT DISTINCT session_id FROM analytics_events, bounds
    WHERE event_name = 'search_performed' AND occurred_at >= lo AND occurred_at < hi
),
-- Ordered by occurred_at via the EXISTS subquery, not a plain intersection: a
-- session must have searched before viewing, not merely done both.
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
viewing_workspaces AS (
    SELECT DISTINCT workspace_id FROM analytics_events, bounds
    WHERE event_name = 'skill_detail_viewed' AND workspace_id IS NOT NULL
      AND occurred_at >= lo AND occurred_at < hi
),
-- Same ordering requirement as above: the fork or Run must come after the
-- detail view, not merely exist alongside it.
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

succeeded_workspaces AS (
    SELECT DISTINCT workspace_id FROM runs, bounds
    WHERE status = 'succeeded' AND created_at >= lo AND created_at < hi
),
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
-- Only the current revision of each evaluation, so a superseded row does not
-- count the same run twice.
evaluations_current AS (
    SELECT e.* FROM evaluations e, bounds
    WHERE e.superseded_at IS NULL AND e.created_at >= lo AND e.created_at < hi
),
feedback_given AS (SELECT count(*) AS n FROM evaluations_current WHERE feedback_helpful IS NOT NULL),
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
downloaded_after_success AS (
    SELECT DISTINCT r.workspace_id FROM runs r, bounds
    WHERE r.status = 'succeeded' AND r.created_at >= lo AND r.created_at < hi
      AND EXISTS (
          SELECT 1 FROM download_records d
          WHERE d.workspace_id = r.workspace_id
            -- finished_at is nullable with no CHECK tying it to status, so a
            -- succeeded run with no finish time falls back to created_at.
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
