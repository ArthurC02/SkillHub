# SEC-009 T5 網路外洩 — 巢狀開發容器，2026-08-27

## ⛔ 這不是驗收，而且比 08-26 那一份離驗收更遠

[ADR-022 §2](../../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 把 **T5 歸在 Suite 2**，而 Suite 2 的受測物有一個定義：**即將加入池的那一台節點**，由生產同一份 IaC 建置、已套用生產的 nftables 與 dnsmasq。本次的「節點」是 Windows → Docker Desktop 的 WSL2 VM → 一個 privileged 容器，「沙箱」是三個 network namespace，「目的地」是一條假上行後面的第四個 namespace。

**所以本目錄不是 `YYYY-MM-DD-<node-id>/`**（ADR-022 §5 給正式證據的命名），名字裡刻意寫著 `nested-dev-container`：沒有節點。

**T5 的 46 項覆蓋一格都不能改。** N-01～N-08 的判定仍然是 `unknown`，理由不是「沒跑」，是**跑的不是那個東西**。

## 那為什麼要跑

因為在 2026-08-27 之前，`infra/egress/rendered/nftables.conf` **從來沒有被任何一個核心載入過**。

CI 有的是 `render.py --check`，它證明的是「這個檔案與 allow-list 一致」。**它證明不了 nft 會接受這個檔案**，而一個載入失敗的 ruleset 留給節點的是**一條規則都沒有**——那是失效開放的方向。同理，T5 此前沒有任何可執行的形式：部署批會在一台沒人除錯過的機器上、在節點已經在佈建的時間壓力下，第一次把它寫出來。

## 三次執行

| 檔案 | 是什麼 | 結果 |
| --- | --- | --- |
| [`T5/ruled.txt`](T5/ruled.txt) | 載入渲染出的 ruleset，跑 T5-1～T5-9 ＋兩個反向驗證 | **12 項全部 refused、兩個反向驗證通過、NXDOMAIN 成立** |
| [`T5/negative-control.txt`](T5/negative-control.txt) | 同一組探針，**不載入任何規則** | **11 項到達目的地** ⇒ 這些探針看得見失敗 |
| [`T5/mutation-metadata.txt`](T5/mutation-metadata.txt) | 載入前拿掉 `skillhub-drop-metadata` 那一條 | **T5-3 轉 FAIL** ⇒ 這個評分器會紅 |

執行環境見 [`versions.txt`](versions.txt)。腳本：[`tools/sec009/t5-network-egress.sh`](../../../../../tools/sec009/t5-network-egress.sh)，自 2026-08-27 起由 `egress-allowlist.yml` 每次觸發時在 CI 上跑（正向與負對照各一次）。

## 這一次量到而此前沒有人知道的四件事

### ① 委任出去的那個 ruleset，nft 收得下

`infra/egress/rendered/nftables.conf` 載入成功，**零個 accept 規則**，也就是失效關閉的那個方向。這是它第一次被證明「載得起來」而不只是「與來源一致」。

### ② T5-4 的東西向阻擋依賴一個 repo 裡沒有任何地方寫著的 sysctl

同一個 bridge、同一個網段上的兩個沙箱，彼此之間走的是二層；**`ip` 的 forward hook 根本看不到那些 frame**，除非 `net.bridge.bridge-nf-call-iptables=1`。

規則本身讀起來像是它自己就擋得住：

```
iifname $SANDBOX_IFACE oifname $SANDBOX_IFACE counter log prefix "skillhub-drop-eastwest " drop
```

**它擋得住，但前提在別的地方。** 本次是本機核心預設為 `1` 才成立的；一台把它設成 `0` 的節點，這條規則計數永遠是零，而兩個 Run 之間**不需要任何逃逸**就互通。這一條要進部署 runbook 與 T5 的節點版檢查清單。

### ③ T5-5 的三個半邊，是被三個不同的地方擋下來的

ADR-022 的 T5-5 寫「`sandboxd:9000`、bridge gateway、節點 loopback」。實測：

| 目標 | 實際擋它的是 | nftables 有沒有留下紀錄 |
| --- | --- | --- |
| 閘道位址的 `:9000`（**被轉送**的流量） | forward 鏈的 `tcp dport 9000` | 有，`skillhub-drop-sandboxd` |
| bridge gateway `10.77.0.1:9000`（節點**自己的**位址） | **input 鏈的 `policy drop`** | 有，`skillhub-drop-inbound` |
| 節點 loopback `127.0.0.1:9000` | **核心的路由層，在 nftables 之前** | **沒有，一筆都沒有** |

forward 鏈裡那條 `ip daddr 127.0.0.0/8 drop` **對這個方向永遠不會觸發**——送往 loopback 的封包不會被轉送。實測到這個程度：在沙箱側加一條 `127.0.0.1 via` 的路由、兩側都把 `route_localnet` 設成 1，再在 `prerouting` 掛一條 raw 優先權的計數規則，**計到的仍然是 0**：封包連 nftables 都沒進到。

**這對 N-06 是一個結論而不是一個 bug**：T5 的判準寫著「每一次嘗試都要在 nftables 記錄中留下一筆 drop，沒有記錄 ＝ N-06 未成立」，而**節點 loopback 這一項在結構上留不下紀錄**。它不是規則寫錯，是那個封包不存在。**節點版的判定表需要為這一項寫一個具名例外，否則它會永遠卡在 `unknown`。**

腳本把這件事寫成一個可被推翻的斷言而不是一句註解：該探針的評分是 `kernel` 級——**要求「被拒絕」且「沒有任何 nftables 計數器動過」**。哪一天有東西計到了，這一列就會紅。

### ④ 負對照抓到三個「看起來在測、其實什麼都沒測」的探針

第一次跑負對照，三列是 `FAIL unreachable even with no rules`：DNS 那一列打的 `198.51.100.53:53` 沒有人在聽、東西向那一列打的 `run2:22` 沒有人在聽、入站那一列打的 `198.51.100.1:9000` 沒有人在聽。**三者在有規則的那一輪全部是綠的**——因為「被 DROP」與「沒有人在聽」對 `nc -z` 而言是同一件事。

補上三個 listener 之後負對照才由 11 項到達。**如果只跑正向那一輪，這三列會一直是綠的，而它們一格都沒在量。**

## 仍然做不到、也沒有假裝做到的

- **T5 的節點版**：真節點、生產 IaC、真閘道位址、`pinned_ip` 不是 `unset`（ADR-022 §2 的三個前置條件）。
- **N-06 的另一半**：規則有 `log prefix` 與 counter，**但沒有任何東西把它們收去保存 90 天**。本次量的是「有沒有留下計數」，不是「紀錄有沒有被保存」。
- **每 Run 一個 netns**：`dockerdrv` 今天不指定 namespace（`HostConfig` 的欄位刻意留零值），dev 仍是共用一張 Docker network。本次的 netns 是實驗室自己建的，**不是產品程式建的**。
- **T5-2 的 1000 個 port**：本次一個 port。每個 port 都要吃滿 drop 的逾時，而這一項問的是那個網段可不可達。節點版要跑完整掃描。
- **T1 點名的四個 CVE PoC**、T6、T7、T10：與本批無關，仍在部署批。
