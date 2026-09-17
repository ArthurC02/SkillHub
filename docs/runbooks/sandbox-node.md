# Runbook：沙箱節點

**讀者是要建、驗收、換新或撤下沙箱節點的人。** 受測者的 Skill 在這台機器上執行，所以這份的每個「驗」都不能跳。

沙箱節點是一台 Ubuntu 24.04 主機，只做一件事：`sandboxd` 收控制平面派來的 Run，在 gVisor（`runsc`）容器裡執行。它不跑 compose，`sandboxd` 是 systemd 管的主機程序。先建好[控制平面](control-plane.md)與[模型閘道](gateway.md)，再建這台。

| 路徑 | 內容 | 誰寫 |
| --- | --- | --- |
| `/etc/skillhub/release.env` | 角色、commit、兩個釘 digest 的映像（`sandboxd`、Runtime Image）、Runtime 版本、位址、slot 數。**不含秘密** | cloud-init（由 `tools/deploy/render.py` 產生） |
| `/opt/skillhub` | 該 commit 的部分 checkout：只有 [`checkout-paths`](../../infra/deploy/sandbox/checkout-paths) 列的檔（部署腳本、egress 允許清單與渲染器、節點准入探針）。**不含應用程式原始碼** | `/usr/local/sbin/skillhub-checkout` |
| `/etc/skillhub/egress/` | 由允許清單渲染的 nftables 規則與 `sandboxd` 讀的准入清單；規則同時裝成 `/etc/nftables.conf` | `skillhub-bootstrap` |
| `/etc/skillhub/sandboxd.env` | `sandboxd` 的非秘密設定：`runsc`、預設 bridge、監聽私有位址 9000、P-02 探針目標 | `skillhub-bootstrap` |
| `/etc/skillhub/secrets/sandboxd.env` | `SKILLHUB_SANDBOX_TOKEN`，600 | 人，手動放 |
| `/etc/systemd/journal-upload.conf.d/skillhub.conf` | 把整份 journal（含出口記錄）推到控制平面 19532 | `skillhub-bootstrap` |
| `/etc/skillhub/node.json` | 節點准入要讀的建置事實：`node_id`、`role`、`node_created_at`、`iac_commit`、`build_phase` | `skillhub-bootstrap` 寫 `provision`，`skillhub-mark-serving` 改成 `serving` |

## 1. 開節點之前

1. **釘閘道位址**：獨立 PR 只改 `infra/egress/allowlist.yaml` 的 `model_gateway.pinned_ip`，填閘道的私有位址（**不是控制平面的位址**），同一個 PR 更新威脅模型文件。合併前 `python tools/egress/render.py --check` 會要求一併重新產生 `infra/egress/rendered/`。**沒有這一步，節點會正常起來，而每個 Run 都到不了閘道。**
2. **控制平面收得到記錄**：控制平面的供應商防火牆要讓這台的私有位址連 19532（[控制平面 runbook](control-plane.md) §1.2）。沒有這一步節點照樣服務，出口記錄只留在本機，換新時跟著主機一起刪掉。
3. **選 commit**：要包含上一步的合併，而且 main 的 CI 已經推過那個 commit 的映像（`go -C tools/devctl run . ci-status <40 碼 sha>` 是 green）。節點上的規則是從這個 commit 的允許清單渲染的。

## 2. 建節點

```bash
cat > sandbox.settings <<'EOF'
SKILLHUB_PRIVATE_IP=<這台的私有位址>
SKILLHUB_CONTROL_PLANE_IP=<控制平面的私有位址>
SKILLHUB_SANDBOX_SLOTS=2
EOF
python tools/deploy/render.py sandbox --release <40 碼 sha> --settings sandbox.settings > sandbox-user-data.yaml
```

render 從那個 commit 的 Runtime Image Dockerfile 讀 `IMAGE_VERSION` 與 Agent SDK 版本，並向 GHCR 查兩個映像的 digest；位址不是 IPv4、slot 不是正整數、或任一映像沒有發佈都會拒絕。

- 用 `sandbox-user-data.yaml` 建主機，接上與控制平面、閘道同一個私有網路。
- 供應商防火牆：入站只開 9000，來源只有控制平面的私有位址；**不開 22**，要進機器用供應商的 console。節點自己的 nftables 也只收這一條，兩層任一層設錯另一層還擋著。

驗：從 console 登入，`cloud-init status --wait` 是 `done`，`/var/log/cloud-init-output.log` 最後一行是 `skillhub-bootstrap: sandbox node installed; …`；`runsc --version` 等於 `infra/nodes/gvisor-baseline.txt`；`sudo nft list table ip skillhub` 有一條指向閘道 `IP 4000` 的 `accept`；`systemctl is-active skillhub-egress-flows systemd-journal-upload` 兩個都是 `active`。

建置腳本只能跑一次：`/etc/skillhub/node.json` 存在時它拒絕執行。建壞了就刪掉重建，不在原機上修。

## 3. 放 token，啟動

在自己的電腦產生 token，同一個值要放兩處（節點與控制平面 §5）：

```bash
openssl rand -hex 32
```

在節點 console：

```bash
sudo install -m 600 /dev/stdin /etc/skillhub/secrets/sandboxd.env <<'EOF'
SKILLHUB_SANDBOX_TOKEN=<上面那個值>
EOF
sudo systemctl start skillhub-serving
```

`skillhub-serving` 先啟動 `sandboxd`，等它以 token 回答 `/capability` 且隔離等級是 `gvisor`，才把 `node.json` 改成 `serving`。`sandboxd` 每次啟動前的檢查任一不過就不啟動：token 檔不是 600 或短於 32 字元、映像沒有釘 digest、egress 准入清單不存在、`skillhub` nftables 表沒有載入、bridge 流量沒有經過 netfilter、連線記錄不帶位元組數（`nf_conntrack_acct` 不是 1）、dockerd 沒有註冊 `runsc`。記錄出口連線的 `skillhub-egress-flows` 沒在跑時 `sandboxd` 也不啟動。原因在 `journalctl -u skillhub-sandboxd`。

驗：`systemctl is-active skillhub-sandboxd` 是 `active`；`sudo cat /etc/skillhub/node.json` 的 `build_phase` 是 `serving`。

## 4. 准入：在這台節點上驗

此時控制平面還不知道這台節點，沒有 Run 會進來。四件事都在**這一台**上做，換一台機器就換了受測物：

1. **節點准入探針**：`sudo python3 /opt/skillhub/tools/sec009/t8-node-probe.py`，exit 0 才繼續。它讀 `node.json`、容器清單、`runsc` 版本、`docker info`，並掃描行程環境與 `/etc/skillhub`、`/opt/skillhub` 裡有沒有資料庫憑證。
2. **這台跑的 Runtime Image 有有效的 SBOM 與掃描**：`sudo sh -c 'set -a; . /etc/skillhub/release.env; set +a; python3 /opt/skillhub/tools/sec009/t8-image-audit.py'`，exit 0 才繼續。帶著 `SKILLHUB_SANDBOX_IMAGE` 時它查的是這台釘的 digest，不是標籤今天指向的那一個；掃描超過 30 天、有可修的 Critical／High、或 `pinned_ip` 還是 `unset` 都會失敗。
3. **SEC-009 Suite 1 與 Suite 2**：照 [`tools/sec009/README.md`](../../tools/sec009/README.md) 的測項，在暫存目錄另取一份完整 checkout（同一個 commit）來跑，證據落 `docs/plans/mvp/m4/sec-009-acceptance/<日期>-<節點>/`。跑完刪掉暫存 checkout 與測試用的映像。
4. **再跑一次第 1 步**：測試留下的容器或檔案會讓它失敗，失敗就先清乾淨。

四件都過才接 §5。任一測項失敗、或結果是 unknown，這台不接入。

## 5. 接回控制平面

在控制平面的 `platform.env`：

| 變數 | 值 |
| --- | --- |
| `SKILLHUB_SANDBOX_PROVIDERS` | `<名稱>=http://<節點私有位址>:9000`，多台以逗號分隔 |
| `SKILLHUB_SANDBOX_TOKEN_<名稱大寫>` | §3 的 token |
| `SKILLHUB_MODEL_GATEWAY_URL` | `http://<閘道私有位址>:4000`，**必須是 IP**，與允許清單的 `pinned_ip` 同一個值。沙箱沒有 DNS，寫成允許清單裡的名字時節點照樣接受 Run，Run 卻解析不到閘道 |
| `SKILLHUB_TRACE_INGEST_URL` | `https://<網域>`。回推 Trace 的是 `sandboxd` 這個主機程序，不是沙箱 |

然後 `sudo systemctl restart skillhub`。**私有網路上的 9000 沒有 TLS**，token 以明文經過，所以 9000 不能對控制平面以外的來源開。

驗：送一個 Run，完成；節點上 `sudo nft list chain ip skillhub forward` 裡指向閘道那條 `accept` 的計數在增加；在控制平面照[控制平面 runbook](control-plane.md) §7 查得到這個 Run 往閘道 4000 的連線，而且帶位元組數。

## 6. 七天換新

節點不升級、不修，到期或有下列事件就換一台新的：`gvisor-baseline.txt` 變了、允許清單變了、有逃逸疑慮、清理連續失敗。

1. 照 §1～§4 建一台新的（`--release` 用最新的綠色 commit），接入 §5，兩台並存。
2. 從 `SKILLHUB_SANDBOX_PROVIDERS` 拿掉舊的那台，重啟控制平面。
3. 等舊節點上沒有執行中的 Run：`sudo docker ps --filter label=skillhub.sandbox.managed` 是空的。
4. 在舊節點 `sudo systemctl stop skillhub-sandboxd`，然後在控制平面確認這台的記錄推完了：`sudo journalctl --directory=/var/log/journal/remote -u skillhub-sandboxd.service _HOSTNAME=<舊節點主機名> -n 3` 裡有剛才的 `Stopped Skill Hub sandbox provider`。
5. 刪掉舊主機。

有逃逸疑慮時不走這個順序：先照 [P1 停派送 runbook](p1-dispatch-halt.md) 停掉全部派送，再換整個池。
