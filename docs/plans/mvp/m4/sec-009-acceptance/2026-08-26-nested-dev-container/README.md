# SEC-009 Suite 1 部分執行 — 巢狀開發容器，2026-08-26

## ⛔ 這份文件不是 SEC-009 的驗收，而且不可能是

`02:SEC-009` 的判準是[基線全數 `pass`、0 項 `unknown`](../../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md)（ADR-022 §3），且明文「部分通過**不得**以其餘項目風險低為由放行」。

**本次 46 項裡有 17 項有機器證據、4 項部分、25 項沒有任何證據。** 這份目錄記錄的是「哪幾項第一次有了可查的東西」，不是驗收表。ADR-022 §5 要求的正式證據目錄命名為 `YYYY-MM-DD-<node-id>/`；這裡的名字刻意不是 node-id，因為**沒有節點**。

而且受測物不對。ADR-022 把 Suite 2 的受測物定義成**即將加入池的那台節點**，本機是 Windows → Docker Desktop 的 WSL2 VM → privileged 容器 → gVisor 沙箱。在裡面跑逃逸測試，量的是「沙箱」與「一個被刻意賦予全部 capability 的容器」之間的邊界，核心也不是生產那個。

## 執行環境

見 [`versions.txt`](versions.txt)。摘要：`runsc release-20260817.0`、沙箱核心 `4.19.0-gvisor`、宿主 `6.6.87.2-microsoft-standard-WSL2`、`systrap` 平台（不需巢狀虛擬化）。

**證據有兩個來源，強度不同**：

| 來源 | 是什麼 | 強度 |
| --- | --- | --- |
| **CI 的 gVisor leg** | `.github/workflows/ci.yml` 的 `sandbox` job：ubuntu-latest 上 `sudo runsc install`，然後以 `SKILLHUB_SANDBOX_TEST_RUNTIME=runsc` 跑 `apps/sandbox/internal/dockerdrv` 全套 | **較強**。真 Linux、真 runsc、真 Docker runtime，而且測的是 `dockerdrv` 生產那份 `HostConfig`，不是手抄的一份。每次 `apps/sandbox/**` 變更都跑（ADR-022 §4 就是這樣指派 Suite 1 的） |
| **本機巢狀容器** | `tools/sec009/` 的 T1／T2 | **較弱**。上面那三段巢狀。它證明的是「腳本跑得完、而且會紅」，不是「邊界守得住」 |
| **GHCR 稽核** | `tools/sec009/t8-image-audit.py` | 查的是 registry 上此刻還成不成立，不需要節點 |

## 本次執行的測項

| 測項 | 跑了嗎 | 結果 |
| --- | --- | --- |
| **T1** 逃逸（通用嘗試八項＋節點側兩項觀察） | ✅ 兩臂都跑 | 沙箱臂全部 REFUSED；負對照臂三項 ESCAPED（`core_pattern` 可寫、`/dev/mem` 可讀、掛上來的 proc 報宿主核心 `6.6.87.2`），節點側植檔如期 FAIL。[sandboxed.txt](T1/sandboxed.txt)、[negative-control.txt](T1/negative-control.txt) |
| **T2** syscall fuzz 4 × 1800s | ✅ 跑完全程 | **ADR-022 的三條判準全 PASS，腳本自己更嚴的一條 FAIL**。見下節與 [fuzz-4x1800s.txt](T2/fuzz-4x1800s.txt)；第一次嘗試卡死於 48 分鐘，記錄於 [hung-run-aborted.txt](T2/hung-run-aborted.txt) |
| **T3** 資源耗盡 | ✅ 但在 CI 上，不在這裡 | C-11／C-12／C-14 的行為測試本次新增至 `docker_test.go`，隨 CI 的 gVisor leg 在真 runsc 上通過；C-13／C-15 早已在 |
| **T4** Runtime 相容性（45 個精選 Skill 重跑） | ❌ | 見下節 |
| **T5～T7、T9、T10** | ❌ | Suite 2，或被測物尚未實作 |
| **T8 映像半** | ✅ | 五項全 PASS。[image-audit.txt](T8/image-audit.txt)、[image-audit.json](T8/image-audit.json) |
| **T8 節點半** | ❌ | 要節點 |

### T1 這次抓到的東西，是它自己的

`no file planted on the node` 這一列在這台機器上**從寫出來到今天為止都是假綠**。`MARKER` 以 `docker run -e` 傳入，而 Git Bash 會把 `/tmp/...` 改寫成 `C:/Users/.../Temp/...`；沙箱裡的 `touch` 因此失敗。兩個後果疊在一起才被發現：評分段還繼承著 `set -e`，於是那次失敗直接殺掉腳本，**節點側兩列觀察一行都沒跑**，rc=1 被報成「setup failed」。若只發生其中一個，`[ -e "$MARKER" ]` 會印 **PASS**——一個從沒執行過的動作拿到綠燈。負對照同樣是死的：無沙箱時同一個 `touch` 也失敗，**整個套件裡沒有任何輸入能讓那一列變紅**。

修法是把 MARKER 定義移進容器內（它是常數，本來就不必跨主機邊界）並補上 `set +e`。重跑後負對照那一列如期變紅。

### T2:三條判準全過,而腳本仍然判 FAIL,兩者都要照實說

最終一次 4 × 1800s([fuzz-4x1800s.txt](T2/fuzz-4x1800s.txt)):

| ADR-022 給 T2 的判準 | 結果 |
| --- | --- |
| Sentry 不 crash | **PASS**——沙箱撐完 1801s |
| 主機核心無 oops | **PASS**——節點側 `dmesg` 前後比對無新增 |
| 容器內程序未取得任何非預期權限 | **PASS**——每個回報的 worker uid／gid 未變，**且 T1 的三個不變量在 30 分鐘壓力之後重驗仍成立**(`core_pattern`、`/dev/mem`、`docker.sock`) |

| 腳本自己的負對照 | 結果 |
| --- | --- |
| 真的 fuzz 了 | **PASS**——**6,115,799 次 syscall**(下限 720,000)、31 個相異 errno、6,714 個切片 |
| 每個 worker 都活到留下最終紀錄 | **FAIL**——**3／4**。worker 2 最後出現在 1,517,014 calls／1,680 slices |

**所以 T2 不記為通過。** ADR-022 §3 把 `unknown` 當 fail，而「第四個 worker 怎麼了」目前正是 unknown。

#### 為什麼不能就說「worker 2 死了」

兩個解釋都還站著,而**這次的輸出同時支持第二個**:

- **它被殺了**——沙箱側記憶體峰值 **10,278 MiB**,四路並行的壓力下有 guest task 被收掉。支持它的是:worker 2 的 progress 行在末尾**連續**消失(1680 之後應該還有四次),不是隨機掉。
- **它最後幾行被吃掉了**——每個 worker 外面包了一層 `( python3 …; echo "EXIT $i $?" )`,而**四行 `EXIT` 只印出兩行**(worker 1 與 3)。**worker 4 有完整的最終 JSON 紀錄卻沒有 EXIT 行**,所以它的 supervisor 跑完了、`echo` 卻沒有到達檔案。**輸出在沙箱收尾時會掉,這一點是這次直接量到的。**

worker 2 三次都是同一個編號,曾經看起來像種子問題(種子是 `100000 * worker + slices`,決定性的)。**不是**:worker 2 單獨跑到 318 slices 乾淨結束,只有在四個並行時才出事。

#### 第一次嘗試卡死了 48 分鐘,而腳本原本會說它 PASS

切片的時間上界靠 0.25s 的 `SIGALRM`,而 fuzzer 打得到武裝它的 syscall——一次隨機 `rt_sigprocmask(SIG_BLOCK, …)` 或 `rt_sigaction(SIGALRM, SIG_IGN)` 之後計時器就不再到達,下一個阻塞式 syscall 永不返回,監督者的 `os.waitpid(pid, 0)` 就永遠等下去。

**刻意不用把那兩個 syscall 加進 DENY 的方式修**:blocklist 只涵蓋有人預料到的掛法,而每一條都是永久讓出的 fuzz 表面。`SIGKILL` 擋不掉、忽略不了、也接不住,所以改由監督者持有時間預算。**最終這次有 43 個切片因此被收掉**,證明那個機制反覆發生——舊碼下第一個就會讓那個 worker 停住。

**更要緊的是第二半**:elapsed 檢查原本**只找提早返回**,所以那個 run 若最終返回,每一行都會印 PASS。遲到現在也是 finding。

**當時用來判定「卡住」的兩個讀數是量錯的東西**:「所有 sentry 執行緒在 S」與「`runsc` 十秒內 0 個 CPU tick」在一次**健康**的執行裡同樣成立(`runsc` 父程序只是監督者,工作在 sentry 執行緒裡;同時 `docker stats` 是 45% CPU、跨執行緒累計 112,278 ticks)。**要看的是累計 ticks 或容器 CPU%。**

#### 兩個檢查本身的弱點,記著不改

- **`no privileges gained (uid/gid)`**:一次 4 × 300s 的執行裡它紅過一次。而 `runsc do` 以 **uid 0** 跑工作負載,所以它唯一觀測得到的變化是**權限下降**(最可能是隨機 `unshare(CLONE_NEWUSER)`),不是取得。**這條檢查在這個環境裡的框架是錯的**,但在一次長工作階段的結尾去改一條安全檢查的判準,正是本專案反覆寫規則在防的事,所以記著等裁定,不動尺。
- **檔案式 checkpoint 不可能生效**:`runsc do` 給沙箱 overlay,`/tmp` 裡寫的東西外面看不到。第一版的 checkpoint 對**包含三個正常結束的 worker 在內**全部報「no checkpoint」——**一個讀起來完全像 finding 的假象**。現在走 stdout,並以單次 `os.write` 寫整行(四個 supervisor 共用那個 stdout,`print()` 會把內文與換行拆成兩次寫)。

## 46 項逐列

判定欄的語意是**「本次是否產生了機器可查的證據」**，不是 SEC-009 的 pass／fail。檢查內容不在此複製——唯一來源是[威脅模型 §4](../../m0/threat-model-and-sandbox-baseline.md)。

| 檢查 | ADR-022 測項 | 本次證據 | 判定 |
| --- | --- | --- | --- |
| `C-01` | T8 節點半 | — | — 無證據 |
| `C-02` | T1 | 本機 T1：`no-new-privileges` 下 `core_pattern` 寫入 REFUSED；CI runsc leg：`User=65532:65532`、`SecurityOpt` 含 `no-new-privileges:true` | ✅ 有證據 |
| `C-03` | T1 | CI runsc leg：`HostConfig.Privileged` 為 false | ✅ 有證據 |
| `C-04` | T1 | 本機 T1：`mount(2)` 掛到的 proc 報 `4.19.0-gvisor` 而非宿主核心；CI runsc leg：pid／ipc／uts／network 皆非 `host` 或 `container:` | ✅ 有證據 |
| `C-05` | T1 | 本機 T1：`/var/run/docker.sock` 不存在；CI runsc leg：`Binds`／`Mounts` 皆空 | ✅ 有證據 |
| `C-06` | T1 | 本機 T1：`/dev/mem` 讀取 REFUSED；CI runsc leg：`ReadonlyRootfs` 為 true、`/` 不可寫、`/work`／`/out` 可寫 | ✅ 有證據 |
| `C-07` | T1 | 本機 T1：看不到任何宿主程序；CI runsc leg：`insp.Mounts` 為空 | ✅ 有證據 |
| `C-08` | T1＋T2 | CI runsc leg：`CapDrop=ALL`；本次 T2 在 30 分鐘壓力後重驗 T1 三個不變量仍成立 | ✅ 有證據 |
| `C-09` | T1＋T2＋T4 | CI runsc leg：`TestRequestedRuntimeIsTheOneTheContainerGot` 對 `runsc` 斷言。**T4 未跑** | ◐ 部分 |
| `C-10` | T3 | CI runsc leg：`NanoCPUs` 等於宣告值（宣告面）。**行為面未量** | ◐ 部分 |
| `C-11` | T3 | CI runsc leg：`TestMemoryCeilingStopsAWorkloadThatExceedsIt`（本次新增，真 Linux ＋ 真 runsc） | ✅ 有證據 |
| `C-12` | T3 | CI runsc leg：`TestScratchQuotaRefusesAWriteWithoutKillingTheRun`（本次新增） | ✅ 有證據 |
| `C-13` | T3 | CI runsc leg：`TestPidsLimitStopsAForkBomb` | ✅ 有證據 |
| `C-14` | T3 | CI runsc leg：`TestOpenFileCeilingReachesGuestTasks`（本次新增） | ✅ 有證據 |
| `C-15` | T3 | CI runsc leg：`TestWallClockStopsALiveSandboxAndDestroyReleasesIt` | ✅ 有證據 |
| `C-16` | T1＋T7 | 本機 T1：找不到任何 unix socket。**T7 未跑** | ◐ 部分 |
| `P-01` | T1＋T8 | 本機 T1：docker socket 不可達、節點上無檔案被放置。**T8 節點半未跑** | ◐ 部分 |
| `P-02` | T10 | —。**P-02 要求的常駐探針不存在**（`04` 丙-26 已查證） | — 無證據 |
| `P-03` | T8 節點半 | — | — 無證據 |
| `P-04` | T8 節點半 | —。`infra/nodes/gvisor-baseline.txt` 仍為 `unset` | — 無證據 |
| `P-05` | T8 節點半 | — | — 無證據 |
| `N-01` | T5 | —。生產強制層（nftables）尚未實作（`04` 甲-3） | — 無證據 |
| `N-02` | T5 | — | — 無證據 |
| `N-03` | T5 | — | — 無證據 |
| `N-04` | T5 | — | — 無證據 |
| `N-05` | T5 | — | — 無證據 |
| `N-06` | T5 | — | — 無證據 |
| `N-07` | T5 | — | — 無證據 |
| `N-08` | T5-9（本次補指派） | —。2026-08-25 加入基線，在此之前沒有任何測項指到它 | — 無證據 |
| `D-01` | T6 | — | — 無證據 |
| `D-02` | T6 | — | — 無證據 |
| `D-03` | T6 | — | — 無證據 |
| `D-04` | T6 | — | — 無證據 |
| `D-05` | T6 | — | — 無證據 |
| `D-06` | T6 | — | — 無證據 |
| `D-07` | T7 | — | — 無證據 |
| `I-01` | T8 映像半 | 本次 T8：`2026.08-3` → `sha256:740f71e2…` | ✅ 有證據 |
| `I-02` | T8 映像半 | 本次 T8：1 條 FROM，全部以 digest 釘選 | ✅ 有證據 |
| `I-03` | T8 映像半 | 本次 T8：SPDX 2.3 bundle `sha256:d177708081b9` | ✅ 有證據 |
| `I-04` | T8 映像半 | 本次 T8：`scanned_at=2026-08-25T13:55:29Z`，0 天，2026-09-24 到期 | ✅ 有證據 |
| `I-05` | T9 | — | — 無證據 |
| `I-06` | T8 映像半 | 本次 T8：可修的 Critical／High = 0（322 筆 finding） | ✅ 有證據 |
| `X-01` | T7＋T9 | — | — 無證據 |
| `X-02` | T7 | — | — 無證據 |
| `X-03` | T7 | — | — 無證據 |
| `X-04` | T7 | —。注入子項是 X-04 唯一的驗證方式 | — 無證據 |

**17 有證據、4 部分、25 無證據。**

## T4 為什麼沒跑

T4 的判準是「終態與 runc 基準**逐筆一致**」，基準是 M2 那 45 個 Run（[content-baseline-report.md](../../m2/content-baseline-report.md)），實測成本約 $3.4。

要在這台機器上跑，得先讓 `sandboxd` 連到一個註冊了 `runsc` 的 Docker daemon——本機 Docker Desktop 沒有，得起 DinD。那會**同時換掉 runtime 與 daemon 環境**，而 ADR-022 明文「任何不一致必須逐筆歸因，且**不得**歸因於 gVisor syscall 不相容」。一次乾淨的全過仍會是證據；但一旦不一致，在巢狀環境裡多半歸因不到，正是那條判準禁止的結局。

而 ADR-022 §4 本來就把 T4 指派給 **CI ＋ 人工確認**，觸發條件是「Runtime Image 或 gVisor 版本變更」，不是隨時跑。**它的落點是 CI 那條已經裝好 runsc 的 leg，缺的是模型憑證，不是一台 Linux。**

## 剩下的 25 項為什麼不是「還沒排時間」

分兩類，第二類不是排程問題：

1. **要真實節點**（T8 節點半的 C-01／P-01／P-03～P-05、T6 的 D-01～D-06、T7 的 D-07／X-01～X-04、T9 的 I-05）——受測物就是那台機器，換一台就換了受測物。
2. **被測的東西還沒有被實作**：
   - **N-01～N-08**：沙箱層 nftables default-deny ＋固定 DNS 的**生產強制層不存在**（`04` 甲-3／SBX-007）。dev 現有的是「那張 Docker 網路上只有閘道」這個結構事實，不是強制層。
   - **P-02（T10）**：ADR-022 要求的**常駐**探針不存在（`04` 丙-26 已查證 `tools/sec009/` 只有 T1／T2／T8）。

第 2 類跑不了不是因為沒安排，是因為要驗的控制還沒寫出來。

## 三個批次前置條件，兩個仍然 FAIL

ADR-022 把三個前置寫成「缺一即 T8 判 `unknown` ＝ fail」，也就是**缺一即整批 fail**：

| # | 現況 |
| --- | --- |
| ① 映像已發佈至 GHCR 且附 SBOM 與掃描 attestation | **PASS**（本次實測） |
| ② `infra/nodes/gvisor-baseline.txt` 已填實際版本 | **FAIL**，仍是 `unset` |
| ③ `infra/egress/allowlist.yaml` 的 `pinned_ip` 已填 | **FAIL**，仍是 `unset` |

②③ 都是那台節點上的值。**SEC-009 目前缺的不是十個測項，是一台機器**——本次同時證明了第①項不在那台機器的等待清單上。
