-- name: RegisterMaintenanceJobs :exec
INSERT INTO maintenance_job_runs (job, period_seconds)
SELECT unnest(@jobs::text[]), unnest(@period_seconds::int[])
ON CONFLICT (job) DO UPDATE SET period_seconds = EXCLUDED.period_seconds;

-- name: ForgetUnscheduledMaintenanceJobs :exec
DELETE FROM maintenance_job_runs WHERE job <> ALL(@jobs::text[]);

-- name: TryLockMaintenanceJob :one
SELECT pg_try_advisory_lock(hashtextextended('skillhub:maintenance:' || @job::text, 0));

-- name: UnlockMaintenanceJob :exec
SELECT pg_advisory_unlock(hashtextextended('skillhub:maintenance:' || @job::text, 0));

-- name: RecordMaintenanceJobSuccess :execrows
UPDATE maintenance_job_runs SET succeeded_at = now() WHERE job = @job;

-- name: ListMaintenanceJobRuns :many
SELECT job, period_seconds, registered_at, succeeded_at
FROM maintenance_job_runs
ORDER BY job;
