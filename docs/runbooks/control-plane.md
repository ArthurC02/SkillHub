# Runbook：控制平面節點

**讀者是要建、換版、還原或重建控制平面那台機器的人。** 照節次做；每一步都寫了「驗什麼」，對不上就停。

控制平面是一台 Ubuntu 24.04 主機，用 [`infra/compose/control-plane.yml`](../../infra/compose/control-plane.yml) 跑：Caddy（TLS）→ web → platform-api；platform-worker；apps/llm；Postgres（含 WAL-G 連續封存）；postgres-exporter、Prometheus、Alertmanager。排程工作由 systemd timer 觸發，不在容器裡。模型閘道與沙箱節點是另外的機器，不在本檔。

| 路徑 | 內容 | 誰寫 |
| --- | --- | --- |
| `/etc/skillhub/release.env` | 角色、commit、網域、四個釘 digest 的映像。**不含秘密** | cloud-init（由 `tools/deploy/render.py` 產生） |
| `/opt/skillhub` | 該 commit 的 repo checkout（compose、Caddyfile、systemd、migration） | cloud-init，並驗 `git rev-parse HEAD` 等於 release |
| `/etc/skillhub/secrets/*` | 秘密，目錄 700、檔案 600 | 人，手動放 |
| `/etc/skillhub/alertmanager.yml` | 由範本代入 SMTP 設定 | `skillhub-bootstrap` |
| `/var/lib/skillhub/deployed-migration` | 這個資料庫已套用到的最後一份 migration 編號 | `skillhub-migrate` |

---

## 1. 建一台新的

### 1.1 產生 user-data

只能部署**main 的 CI 已經推過映像**的 commit：

```bash
go -C tools/devctl run . ci-status <40 碼 sha>          # 必須是 green
python tools/deploy/render.py control-plane --release <40 碼 sha> --settings control-plane.settings > user-data.yaml
```

`control-plane.settings` 是 `KEY=VALUE`（不進 repo）：`SKILLHUB_DOMAIN`、`SKILLHUB_ACME_EMAIL`、`SKILLHUB_PRIVATE_IP`（私有網路上這台的位址）、`SKILLHUB_ALERT_EMAIL`、`SKILLHUB_SMTP_SMARTHOST`（`host:587`）、`SKILLHUB_SMTP_FROM`、`SKILLHUB_SMTP_USERNAME`。少一個、多一個拼錯的、或值帶 shell 會重新解讀的字元，render 都會拒絕；某個映像沒有被那個 commit 推出來也會拒絕。

### 1.2 開機

- 用 `user-data.yaml` 建主機，接上私有網路。
- 防火牆：公網只開 22、80、443（含 443/udp）；5432 只對模型閘道的私有位址開。Prometheus（9095）與 Alertmanager（9093）只綁 127.0.0.1，用 SSH tunnel 看。
- DNS 只設 A 記錄指到這台。compose 網路沒有開 IPv6，IPv6 連線會經 Docker 的轉發程式進來，來源位址變成容器網段的閘道，所有 IPv6 使用者會共用一個速率限制桶。**Caddy 第一次啟動就會去申請憑證**，DNS 沒生效前啟動只會一直重試。

驗：`cloud-init status --wait` 是 `done`；`/var/log/cloud-init-output.log` 最後一行是 `skillhub-bootstrap: control plane installed; …`。

### 1.3 放秘密

```bash
sudo install -m 600 /dev/stdin /etc/skillhub/secrets/<檔名> <<'EOF'
…
EOF
```

| 檔 | 內容 |
| --- | --- |
| `postgres.env` | `POSTGRES_USER`、`POSTGRES_PASSWORD`、`POSTGRES_DB=skillhub`；WAL-G：`WALG_S3_PREFIX=s3://<備份 bucket>/postgres`、`AWS_ACCESS_KEY_ID`、`AWS_SECRET_ACCESS_KEY`、`AWS_ENDPOINT`、`AWS_REGION`、`AWS_S3_FORCE_PATH_STYLE=true`；可選 `WALG_RETAIN_FULL`（預設保留 7 份完整備份） |
| `platform.env` | api、worker、maintenance、reindex 共用。`DATABASE_URL=postgres://<user>:<password>@postgres:5432/skillhub?sslmode=disable`；其餘照 [release-checklist](../plans/mvp/m4/release-checklist.md) §2.3 的表，完整清單是 `.env.example` |
| `llm.env` | apps/llm 的設定；`LITELLM_API_KEY` 是 Virtual Key，不是 master key（§2.3） |
| `postgres-exporter.env` | `DATA_SOURCE_NAME=postgresql://<user>:<password>@postgres:5432/skillhub?sslmode=disable` |
| `smtp-password` | SMTP 密碼，單獨一行 |

**備份 bucket 與應用程式的物件儲存分開**，金鑰也分開：應用程式的金鑰被拿走時，備份不能跟著被刪。這套設定沒有做用戶端加密，bucket 必須私有並開供應商端加密。

驗：`sudo /opt/skillhub/infra/deploy/control-plane/bin/skillhub-preflight` 沒有輸出、exit 0（要先 `set -a; . /etc/skillhub/release.env; set +a`）。

### 1.4 建 schema，啟動

```bash
sudo /opt/skillhub/infra/deploy/control-plane/bin/skillhub-migrate 0000
sudo systemctl start skillhub
sudo /opt/skillhub/infra/deploy/control-plane/bin/skillhub-enable-timers
```

`skillhub-migrate` 會先只起 Postgres，逐份以單一交易套用，每成功一份就把編號寫進 ledger；中途失敗就停在那一份，修好後不帶參數重跑即可接續。

驗：
- `docker compose --env-file /etc/skillhub/release.env -f /opt/skillhub/infra/compose/control-plane.yml ps` 每個服務是 `running`，postgres 是 `healthy`。
- `curl -sI https://<網域>/healthz` 是 200，憑證由公開 CA 簽發。
- `systemctl list-timers 'skillhub-*'` 列出 `maintenance-schedule` 的每一行，加上 `skillhub-backup.timer` 與 `skillhub-restore-drill.timer`。
- 立刻做第一次備份與一次演練（§3），不要等排程。

之後接 release-checklist §2.4 的種入與回填。

## 2. 換版

換版就是換 `release.env` 與 `/opt/skillhub` 到另一個 commit。在自己的電腦上：

```bash
python tools/deploy/render.py control-plane --release <新 sha> --settings control-plane.settings --release-env > release.env
```

把 `release.env` 傳到節點上，然後：

```bash
sudo systemctl start skillhub-backup
sudo install -m 0644 release.env /etc/skillhub/release.env
set -a; . /etc/skillhub/release.env; set +a
cd /opt/skillhub && sudo git fetch -q --depth 1 "$SKILLHUB_REPOSITORY" "$SKILLHUB_RELEASE" && sudo git checkout -q FETCH_HEAD && test "$(git rev-parse HEAD)" = "$SKILLHUB_RELEASE"
sudo /opt/skillhub/infra/deploy/control-plane/bin/skillhub-bootstrap
sudo /opt/skillhub/infra/deploy/control-plane/bin/skillhub-migrate
sudo systemctl restart skillhub
sudo /opt/skillhub/infra/deploy/control-plane/bin/skillhub-enable-timers
```

- **先備份再 migrate**：migration 是單向的，回到舊版的方法是從備份還原（§4），不是倒著跑 SQL。
- `skillhub-bootstrap` 可以重跑：它重新安裝新版的 systemd unit，並用新的 `release.env` 重新產生 `alertmanager.yml`。**改了告警信箱或 SMTP 設定，也是走這一節**，不重跑它設定不會生效。

## 3. 備份與還原演練

- **連續封存**：Postgres 每寫滿一段 WAL，或有寫入後滿 60 秒，就由 `wal-g wal-push` 送到備份 bucket。失敗時 WAL 堆在資料卷上並重試。
- **完整備份**：`skillhub-backup.timer` 每天 03:00，保留最近 `WALG_RETAIN_FULL` 份，更舊的連同所屬 WAL 一起刪。
- **還原演練**：`skillhub-restore-drill.timer` 每月 7 日 05:00，在一個**沒有掛資料卷**的一次性容器裡抓最新完整備份、重播 WAL 到最新、印出幾張核心表的列數與最後寫入時間。不碰正在服務的資料庫。

手動：

```bash
sudo systemctl start skillhub-backup
sudo systemctl start skillhub-restore-drill
journalctl -u skillhub-restore-drill -n 20
```

驗演練：最後一行是 `restore drill: recovered and readable`，`runs`／`audit_events` 最後寫入時間距離現在不超過封存延遲（約一分鐘）。**數字要跟線上資料庫對一次**，不是只看它有沒有成功。

## 4. 從備份還原到線上

用在資料庫毀損、誤刪或換版後要退回。**會丟掉還原點之後的寫入。**

```bash
C="sudo docker compose --env-file /etc/skillhub/release.env -f /opt/skillhub/infra/compose/control-plane.yml"
sudo systemctl stop skillhub-backup.timer
$C stop caddy web platform-api platform-worker postgres-exporter postgres
$C run --rm --no-deps -v skillhub_postgres-restore-hold:/hold postgres cp -a /var/lib/postgresql/. /hold/
$C run --rm --no-deps postgres sh -euc '
  rm -rf "$PGDATA" && install -d -o postgres -g postgres -m 700 "$PGDATA"
  gosu postgres wal-g backup-fetch "$PGDATA" LATEST
  gosu postgres touch "$PGDATA/recovery.signal"
  echo "restore_command = '\''wal-g wal-fetch %f %p'\''" >>"$PGDATA/postgresql.auto.conf"'
$C up -d --wait postgres
$C exec -T postgres sh -c 'until psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "SELECT NOT pg_is_in_recovery()" | grep -q t; do sleep 5; done'
```

- 第一個 `run` 把現在的資料目錄整份複製到 `skillhub_postgres-restore-hold`，還原失敗時還有東西可以退。
- 要退到某個時間點而不是最新：在 `echo` 那一行後面再加一行 `echo "recovery_target_time = '\''<UTC 時間>'\''" >>"$PGDATA/postgresql.auto.conf"`。

最後一行回來之後資料庫已經升為主庫。接著：

```bash
sudo systemctl start skillhub skillhub-backup.timer
sudo systemctl start skillhub-backup
```

還原後的資料庫在新的 timeline 上，**立刻做一次完整備份**，之後的 WAL 才有起點。確認資料無誤後 `sudo docker volume rm skillhub_postgres-restore-hold`。

## 5. 告警

Alertmanager 把所有告警寄到 `SKILLHUB_ALERT_EMAIL`；`severity=critical` 每小時重寄，其餘四小時。

| 來源 | 什麼時候響 |
| --- | --- |
| `infra/observability/alerts.yml` | 平台自報指標，以及 `skillhub-database` 群組：`PostgresDown`、`WalArchiveFailing`、`WalArchiveStalled` |
| `ScheduledJobFailed` | 任何 `skillhub-*` 排程工作（備份、演練、保存期清理）失敗，由 systemd `OnFailure` 直接送進 Alertmanager；`unit` 標籤是失敗的那個 unit，`journalctl -u <unit>` 看原因 |

驗送達：

```bash
sudo systemctl start skillhub-alert@test.service
```

幾分鐘內信箱要收到 `test failed on <主機名>`。**沒收到就等於沒有告警**，先修這個再做別的。

## 6. 重建整台

主機遺失或被入侵時，不修，重建：

1. §1.1～1.3 建新的，`--release` 用舊主機最後部署的那個 commit。秘密換新；被入侵時所有秘密都換，**但 `postgres.env` 的 WAL-G 設定指向同一個備份位置**（金鑰可以換）。
2. §1.4 **不跑 migrate**，照 §4 從備份還原。
3. 補 migration ledger，讓下一次換版知道從哪裡接：`sudo install -d /var/lib/skillhub && ls /opt/skillhub/db/migrations | tail -1 | cut -c1-4 | sudo tee /var/lib/skillhub/deployed-migration`（前提是第 1 步的 commit 與備份來自同一個版本）。
4. `sudo systemctl start skillhub`、`skillhub-enable-timers`，最後把 DNS 指過去。Caddy 會重新申請憑證。
