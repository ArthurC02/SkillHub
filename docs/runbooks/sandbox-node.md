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
3. **選 commit**：要包含上一步的合併，且已有受控、釘定的節點映像。此專案目前由 main 的 CI 發布映像，可用 `go -C tools/devctl run . ci-status <40 碼 sha>` 確認 green；若部署管線不同，使用它選定的 provenance 證據。節點上的規則一律從這個 commit 的允許清單渲染，不能拿另一個 commit 的規則配映像。

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

驗：從 console 登入，`cloud-init status --wait` 是 `done`，`/var/log/cloud-init-output.log` 最後一行是 `skillhub-bootstrap: sandbox node installed; …`；`runsc --version` 等於 `infra/nodes/gvisor-baseline.txt`；`sudo nft list table ip skillhub` 有一條指向閘道 `IP 4000` 的 `accept`；`systemctl is-active skillhub-egress-flows skillhub-egress-drops systemd-journal-upload` 三個都是 `active`。

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

`skillhub-serving` 先啟動 `sandboxd`，等它以 token 回答 `/capability` 且隔離強度是 `strong`，才把 `node.json` 改成 `serving`。`sandboxd` 每次啟動前的檢查任一不過就不啟動：token 檔不是 600 或短於 32 字元、映像沒有釘 digest、egress 准入清單不存在、`skillhub` nftables 表沒有載入、bridge 流量沒有經過 netfilter、連線記錄不帶位元組數（`nf_conntrack_acct` 不是 1）、dockerd 沒有註冊 `runsc`。寫出口記錄的 `skillhub-egress-flows`（放行的連線）與 `skillhub-egress-drops`（被擋的封包）任一個沒在跑，`sandboxd` 都不啟動；兩者都把觀察到的東西轉成[出口記錄契約](../../contracts/events/egress-record.schema.json)的格式才寫進 journal，查法見[控制平面 runbook](control-plane.md) §7。原因在 `journalctl -u skillhub-sandboxd`。

驗：`systemctl is-active skillhub-sandboxd` 是 `active`；`sudo cat /etc/skillhub/node.json` 的 `build_phase` 是 `serving`。

## 4. 准入：在這台節點上驗

此時控制平面還不知道這台節點，沒有 Run 會進來。四件事都在**這一台**上做，換一台機器就換了受測物：

1. **節點准入探針**：`sudo python3 /opt/skillhub/tools/sec009/t8-node-probe.py`，exit 0 才繼續。它讀 `node.json`、容器清單、`runsc` 版本、`docker info`，並掃描行程環境與 `/etc/skillhub`、`/opt/skillhub` 裡有沒有資料庫憑證。
2. **這台跑的 Runtime Image 有有效的 SBOM 與掃描**：管理者先準備可由 root 執行的 GitHub CLI，以 `sudo gh attestation verify --help` 確認支援驗證，並在管理者環境設定具有該 GHCR 映像唯讀權限的 `GH_TOKEN`；bootstrap 不會安裝 CLI 或供應這份憑證。執行 `sudo --preserve-env=GH_TOKEN sh -c 'set -a; . /etc/skillhub/release.env; set +a; python3 /opt/skillhub/tools/sec009/t8-image-audit.py'`，exit 0 才繼續。token 只供這次管理者稽核，不放入 `release.env`、`sandboxd.env` 或工作負載。帶著 `SKILLHUB_SANDBOX_IMAGE` 時它查的是這台釘的 digest；無法驗證簽章、掃描已達 30 天、時間在未來、有可修的 Critical／High、或 `pinned_ip` 還是 `unset` 都不通過。
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

**目前缺少安全的 maintenance／drain 操作，不能把移除 Provider 當成停止新派送。** 控制平面以設定中的 Provider 名稱解析既有 attempt；先拿掉舊節點並重啟，會使輪詢失敗，也讓清理找不到拆除端點。舊 Run 與資源清完前，保留原名稱、端點與 token，不把同名 Provider 改指另一台主機。

例行換新需要以下順序與證據；其中 maintenance 屏障及撤下判定尚未實作，待審查的行為見[節點維護提案](../domain-memory/changes/sandbox-node-drain/draft-pr.md)。目前不能依此段宣稱已有可執行的滾動換新自動化。

1. 照 §1～§4 建立並驗收新節點，依 §5 以另一個 Provider 名稱接入，保留舊節點路由。新舊並存須符合部署授權及預算。
2. 先有可確認、跨 Worker 生效的舊節點停派送屏障，才開始等待既有 attempt 收尾。現有 `/admin/dispatch/halt` 宣告的是 P1，會同時保留現場、阻擋清理；`orphan_threshold` 又會自動解除，兩者不能替代例行 maintenance。
3. 確認無未結束或建立中的 attempt、相關 Run 已完成資源清理，並核對節點上**包含停止容器**的受管資源清單、模型金鑰及物件授權。`sudo docker ps -a --filter label=skillhub.sandbox.managed` 只能協助查看容器，不能單獨證明已 drain；讀取失敗或資料不完整都不是空集合。資源已清但價格仍待補報時，既有延後查費不依賴舊節點端點，不必只為價格保留主機。
4. 停派送屏障仍有效且資源已清，才停止舊節點 `skillhub-sandboxd`；在控制平面確認該節點的必要記錄已送達並保存。最後三行 journal 或一筆停止訊息不足以證明記錄完整送達。
5. 再從 `SKILLHUB_SANDBOX_PROVIDERS` 移除舊節點及其 token，確認設定生效與目標主機身分後，由具授權的部署負責人撤下該主機。尚有 P1 保留、資源未清或證據未知時不執行撤機。

有逃逸疑慮時先照 [P1 停派送 runbook](p1-dispatch-halt.md) 停派送並保留現場；調查、證據保存與負責人確認完成前，不用例行換新程序清除或刪掉事故節點。
