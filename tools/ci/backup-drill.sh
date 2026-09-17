#!/usr/bin/env bash
set -euo pipefail

POSTGRES_IMAGE="${POSTGRES_IMAGE:?set POSTGRES_IMAGE to the skillhub-postgres image under test}"
SEAWEEDFS_IMAGE="chrislusf/seaweedfs:4.46@sha256:08d516132314207d10c8e37cbffc1f32b147d870169688734cc61c6231625b62"
ROOT="$(cd "$(dirname "$0")/../.." && (pwd -W 2>/dev/null || pwd))"
NET="backup-drill-$$"
S3="backup-drill-s3-$$"
PG="backup-drill-pg-$$"

cleanup() {
  docker rm -f "$S3" "$PG" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

fail() {
  echo "backup drill: $*" >&2
  docker logs --tail 40 "$PG" >&2 || true
  exit 1
}

walg_env=(
  -e WALG_S3_PREFIX=s3://skillhub-backups/postgres
  -e AWS_ACCESS_KEY_ID=skillhubdev
  -e AWS_SECRET_ACCESS_KEY=skillhubdevsecret
  -e "AWS_ENDPOINT=http://$S3:8333"
  -e AWS_S3_FORCE_PATH_STYLE=true
  -e AWS_REGION=us-east-1
  -e POSTGRES_USER=skillhub
)

psql_exec() {
  docker exec -i "$PG" psql -v ON_ERROR_STOP=1 -U skillhub -d skillhub -Atq "$@"
}

docker network create "$NET" >/dev/null
docker create --name "$S3" --network "$NET" "$SEAWEEDFS_IMAGE" \
  server -s3 -s3.config=/etc/seaweedfs/s3.json -dir=/data >/dev/null
docker cp "$ROOT/infra/compose/seaweedfs-s3.json" "$S3:/etc/seaweedfs/s3.json"
docker start "$S3" >/dev/null

for _ in $(seq 1 60); do
  if docker exec "$S3" sh -c 'echo "s3.bucket.create -name skillhub-backups" | weed shell' >/dev/null 2>&1 &&
     docker exec "$S3" sh -c 'echo "s3.bucket.list" | weed shell' 2>/dev/null | grep -q skillhub-backups; then
    break
  fi
  sleep 2
done
docker exec "$S3" sh -c 'echo "s3.bucket.list" | weed shell' 2>/dev/null | grep -q skillhub-backups ||
  fail "the object store never accepted the backup bucket"

docker run -d --name "$PG" --network "$NET" "${walg_env[@]}" -e POSTGRES_PASSWORD=skillhub -e POSTGRES_DB=skillhub \
  "$POSTGRES_IMAGE" postgres -c archive_mode=on -c "archive_command=wal-g wal-push %p" -c archive_timeout=60 >/dev/null
for _ in $(seq 1 60); do
  [ "$(psql_exec -c 'SELECT 1' 2>/dev/null)" = 1 ] && break
  sleep 2
done
[ "$(psql_exec -c 'SELECT 1' 2>/dev/null)" = 1 ] || fail "postgres did not start"

for migration in "$ROOT"/db/migrations/*.sql; do
  psql_exec --single-transaction <"$migration" >/dev/null || fail "migration $(basename "$migration") failed"
done

psql_exec -c "INSERT INTO users (email, display_name) SELECT 'before-' || g || '@drill', 'before' FROM generate_series(1, 3) g" >/dev/null
docker exec "$PG" skillhub-backup >/dev/null 2>&1 || fail "skillhub-backup failed"

psql_exec -c "INSERT INTO users (email, display_name) SELECT 'after-' || g || '@drill', 'after' FROM generate_series(1, 4) g" >/dev/null
segment="$(psql_exec -c 'SELECT pg_walfile_name(pg_current_wal_lsn())')"
psql_exec -c 'SELECT pg_switch_wal()' >/dev/null
for _ in $(seq 1 60); do
  [ "$(psql_exec -c "SELECT coalesce(last_archived_wal >= '$segment', false) FROM pg_stat_archiver")" = t ] && break
  sleep 2
done
[ "$(psql_exec -c "SELECT coalesce(last_archived_wal >= '$segment', false) FROM pg_stat_archiver")" = t ] ||
  fail "WAL segment $segment was never archived"

want="$(psql_exec -c 'SELECT count(*) FROM users')"
report="$(docker run --rm --network "$NET" "${walg_env[@]}" "$POSTGRES_IMAGE" skillhub-restore-drill 2>/dev/null)" ||
  fail "skillhub-restore-drill failed"
got="$(awk '$1 == "users" { print $2 }' <<<"$report")"
[ "$got" = "$want" ] ||
  fail "the restored database has $got users, the source has $want: the WAL written after the base backup was not replayed"
echo "backup drill: restored $got users, including the 4 written after the base backup"

if docker run --rm --network "$NET" "${walg_env[@]}" -e WALG_S3_PREFIX=s3://skillhub-backups/nothing-here \
  "$POSTGRES_IMAGE" skillhub-restore-drill >/dev/null 2>&1; then
  fail "a restore drill with no backup to fetch exited 0"
fi
echo "backup drill: a drill with no backup fails, as it must"
