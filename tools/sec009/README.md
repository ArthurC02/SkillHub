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
