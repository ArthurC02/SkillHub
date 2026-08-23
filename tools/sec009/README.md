# SEC-009 的可執行化（不是 SEC-009 的驗收）

## 這裡是什麼

`02:SEC-009` 要求 SelfHostedProvider 驗收的 45 項全數通過，可執行的測項清單、通過判準與證據要求由 [ADR-022](../../docs/adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第三部分給出（10 個測項、45 項覆蓋核對、全數 pass 且 0 unknown 才放行，證據存 `docs/plans/mvp/m4/sec-009-acceptance/`）。

在 2026-08-23 之前，那 10 個測項**一行可執行的東西都沒有**，而擋住它的一直被記成「要一台 Linux 機器」。

## 已經查明的事實（2026-08-23）

**gVisor 在這台開發機上跑得起來，而且是真的 gVisor。** 不需要 WSL 發行版——`wsl --list` 上只有 Docker Desktop 自用的 `docker-desktop`，沒有 Ubuntu。走的是 Docker：

| 位置 | 核心 |
| --- | --- |
| 宿主容器 | `6.6.87.2-microsoft-standard-WSL2` |
| 沙箱內 | **`4.19.0-gvisor`** |

`dmesg` 是 gVisor 自己的開機訊息、`--network=none` 下零個網路介面、沙箱內 PID 1 是被沙箱的那個 process。`runsc release-20260817.0`，下載後對過 sha512。

跑法：`tools/sec009/gvisor-smoke.sh`

## 這裡**不是**什麼

**不是 SEC-009 的驗收，而且不可能是。** 兩個理由，第二個比第一個重要：

1. ADR-022 要的是**獨立 VM 池**上的 10 個測項與 45 項覆蓋，證據落檔。這支腳本只跑一個 smoke test。
2. **它是巢狀的**：Windows 宿主 → Docker Desktop 的 WSL2 VM → privileged 容器 → gVisor 沙箱。在這裡跑逃逸測試，量的是「沙箱」與「一個被刻意賦予全部 capability 的容器」之間的邊界，而且核心不是生產跑的那個。**這裡過了只代表流程跑得完，不代表邊界守得住。**

AGENTS.md 第 7 條同一件事的另一種說法：Dev Container 不取代真實 Linux／gVisor 部署驗收。

## 一個要進部署 runbook 的發現

privileged 容器裡第一次跑 runsc 會失敗在：

```
cannot set up cgroup for root: configuring cgroup: write /sys/fs/cgroup/cgroup.subtree_control: device or resource busy
```

**顯而易見的修法是 `--ignore-cgroups`，它會過——而它關掉的正是 SEC-009 資源耗盡測項要量的那個東西。** 綠燈會出現，測項會變成空的。

腳本走的是另一條：把 root cgroup 裡的 process 移進 leaf，讓 `subtree_control` 可寫（cgroup v2 的 no-internal-process 規則）。**保住 `+cpu +memory +pids` 才有東西可以量。**

這一條要寫進部署 runbook：任何在容器裡跑 runsc 的環境都會撞到它，而兩個修法的差別在告警面上看不出來。

## 下一步

把 ADR-022 第三部分的 10 個測項逐條寫成腳本，在這裡 dry run。**跑得過的記成「程序可執行」，不記成測項通過**；真的驗收要一台獨立 Linux 節點，而 `infra/nodes/gvisor-baseline.txt` 現在還是 `unset`（＝還沒有任何沙箱節點被開出來）。

---

## T1 的通用嘗試(2026-08-23 新增)

`tools/sec009/t1-escape-attempts.sh`——ADR-022 第三部分 T1 裡**通用嘗試**那一半,八項:寫 `core_pattern`、`mount(2)`、載入核心模組、讀 `/dev/mem`、看見宿主程序、找 unix socket、碰 docker socket、在節點上放一個檔案;外加 T1 判準要求的兩項節點側觀察(沒有檔案被放上來、`dmesg` 沒有 taint)。

**T1 點名的四個 CVE PoC 不在裡面**,而且刻意不放:它們需要真的 exploit 程式碼打真的宿主,在一個巢狀的開發機容器裡跑別人的 PoC,對一台它從沒碰過的節點證明不了任何事。那四項屬部署批。

### 它有一個負對照,而且那不是裝飾

```bash
tools/sec009/t1-escape-attempts.sh                    # 沙箱內,期望全部 REFUSED
SEC009_NO_SANDBOX=1 tools/sec009/t1-escape-attempts.sh  # 無沙箱,期望至少一項 ESCAPED
```

**一個不管有沒有沙箱都會綠的探針組,量的是零。** 負對照把期望反過來:沒有沙箱時至少要有一項成功,否則腳本自己判失敗。實跑結果——無沙箱下三項逃得出去:`core_pattern` 寫得進去、`/dev/mem` 讀得到、而 `mount` 掛上來的 proc **報的是宿主核心 `6.6.87.2-microsoft-standard-WSL2`**。加上沙箱之後這三項全部 REFUSED。

### 第一次跑抓到的三件事,全部是這支腳本自己的問題

| # | 症狀 | 真相 |
| --- | --- | --- |
| 1 | `mount(2)` 判為 ESCAPED | **gVisor 有實作 mount**,給的是它自己的 procfs。**syscall 成功不等於逃逸**;探針改成把掛上來的 proc 讀回來,只有在它報出非 gVisor 核心時才算逃逸 |
| 2 | 找到 unix socket | 那是 `runsc do` 自己的控制通道 `/tmp/runsc-do*/runsc-*.sock`。**生產不走 `do`**(sandboxd 走 Docker runtime),所以那是**探針量到自己**;現以路徑窄範圍排除 |
| 3 | 全部通過而且**什麼都沒跑** | `bash -s` 從 stdin 讀腳本,而 `docker run` 少了 `-i` 時 stdin 什麼都沒有——容器跑了一個空腳本,**exit 0**。一個從沒執行過的測試給出綠燈,正是這個目錄存在的理由 |

**這三件全都會在部署日當天發生**,差別是那天有時間壓力、有一台剛建好沒人除錯過的機器,而且第 3 件會讓人以為驗收過了。

### 它仍然不是 SEC-009 的驗收

一格都不是。巢狀環境量的是「沙箱」與「一個被刻意給了全部 capability 的容器」之間的邊界,核心也不是生產那個。ADR-022 把 Suite 2 的受測物定義成**即將加入池的那台節點**——換一台機器就換了受測物。

**T3(資源耗盡)刻意沒有寫成腳本**:它要量的是「限制由 runtime 強制生效」與「同節點其他 Run 劣化 < 20%」,而限制是 `sandboxd` 經 Docker runtime 套上去的,不是 `runsc do` 套的。用 `ulimit` 在沙箱裡自己設一個上限再驗證它生效,測到的是 `ulimit`。**那一項要一台跑著 sandboxd 的節點,不是一支 shell 腳本。**
