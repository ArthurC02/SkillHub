# Runbook：模型閘道節點

**讀者是要建、換版或重建模型閘道那台機器的人。** 所有模型呼叫都經過這台（apps/llm、每個 Run 的沙箱、平台簽發與撤銷 Virtual Key），供應商金鑰只放在這台。

閘道是一台 Ubuntu 24.04 小主機，用 [`infra/compose/gateway.yml`](../../infra/compose/gateway.yml) 跑 LiteLLM，模型清單是 [`infra/compose/litellm-config.yaml`](../../infra/compose/litellm-config.yaml)（與本機開發同一份）。它的資料（Virtual Key、花費）放在**控制平面 Postgres 的 `litellm` 資料庫**，經私有網路連過去。LiteLLM 只綁私有網路位址的 4000 埠。

先建控制平面（[控制平面 runbook](control-plane.md)），再建這台。

## 1. 在控制平面上準備資料庫

```bash
C="sudo docker compose --env-file /etc/skillhub/release.env -f /opt/skillhub/infra/compose/control-plane.yml"
$C exec postgres sh -c 'createuser -U "$POSTGRES_USER" --pwprompt litellm'
$C exec postgres sh -c 'createdb -U "$POSTGRES_USER" -O litellm litellm'
```

密碼在提示時輸入，不進 shell 歷史。控制平面的防火牆讓 5432 只對閘道的私有位址開。

驗：`$C exec postgres psql -U "$POSTGRES_USER" -d postgres -Atc "SELECT datname FROM pg_database WHERE datname = 'litellm'"` 印出 `litellm`。

## 2. 建閘道主機

```bash
printf 'SKILLHUB_PRIVATE_IP=<閘道的私有位址>\n' > gateway.settings
python tools/deploy/render.py gateway --release <40 碼 sha> --settings gateway.settings > gateway-user-data.yaml
```

- 用 `gateway-user-data.yaml` 建主機，接上與控制平面同一個私有網路。
- 防火牆：公網只開 22；4000 只對控制平面與沙箱節點的私有位址開。**私有網路上的流量沒有 TLS**，Virtual Key 以明文經過，所以 4000 不能對任何其他來源開。

驗：`cloud-init status --wait` 是 `done`，`/var/log/cloud-init-output.log` 最後一行是 `skillhub-bootstrap: model gateway installed; …`。

## 3. 放秘密，啟動

```bash
sudo install -m 600 /dev/stdin /etc/skillhub/secrets/litellm.env <<'EOF'
LITELLM_MASTER_KEY=sk-…
OPENAI_API_KEY=…
DATABASE_URL=postgresql://litellm:<密碼>@<控制平面私有位址>:5432/litellm
EOF
sudo systemctl start skillhub
```

`LITELLM_MASTER_KEY` 是閘道的管理員憑證：只放在這台，以及平台簽發每個 Run 的短效 key 時用的 `SKILLHUB_MODEL_GATEWAY_KEY`。apps/llm 拿的是由它簽發、帶 `max_budget` 的 Virtual Key（release-checklist §2.3）。

`skillhub-preflight` 在每次啟動前檢查 `litellm.env` 是 600 且三個變數都有值。

驗：
- `docker compose --env-file /etc/skillhub/release.env -f /opt/skillhub/infra/compose/gateway.yml ps` 的 litellm 是 `healthy`。
- 從控制平面：`curl -s http://<閘道私有位址>:4000/health/liveliness` 有回應。
- 從任何其他位址連 4000 失敗。

## 4. 接回控制平面

| 在哪裡 | 設什麼 |
| --- | --- |
| 控制平面 `control-plane.settings` | `SKILLHUB_GATEWAY_URL=http://<閘道私有位址>:4000`，照控制平面 runbook §2 換版一次，讓 Prometheus 開始探測它 |
| 控制平面 `llm.env` | `LITELLM_BASE_URL=http://<閘道私有位址>:4000`、`LITELLM_API_KEY=<Virtual Key>` |
| 控制平面 `platform.env` | `SKILLHUB_MODEL_GATEWAY_URL`（沙箱節點連得到的位址）、`SKILLHUB_MODEL_GATEWAY_ADMIN_URL`、`SKILLHUB_MODEL_GATEWAY_KEY` |

驗：控制平面的 Prometheus 上 `probe_success{job="gateway"}` 是 1。把閘道的 `skillhub` 停掉五分鐘，`ModelGatewayDown` 要寄到信箱；再啟動。

## 5. 換版與重建

- **換版**：`render.py gateway … --release-env`，其餘照控制平面 runbook §2，但沒有備份與 migrate 兩步。LiteLLM 自己的資料表由它啟動時建立與升級。
- **重建**：閘道沒有本機狀態，資料都在控制平面的 `litellm` 資料庫。照 §2～3 建新的，換掉私有位址時一併改 §4 三處。
