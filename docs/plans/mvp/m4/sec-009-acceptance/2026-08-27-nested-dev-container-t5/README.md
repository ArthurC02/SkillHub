# SEC-009 T5 網路外洩 — 巢狀開發容器，2026-08-27

## ⛔ 這不是驗收，而且比 08-26 那一份離驗收更遠

[ADR-022 §2](../../../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 把 **T5 歸在 Suite 2**，而 Suite 2 的受測物有一個定義：**即將加入池的那一台節點**，由生產同一份 IaC 建置、已套用生產的 nftables 與 dnsmasq。本次的「節點」是 Windows → Docker Desktop 的 WSL2 VM → 一個 privileged 容器，「沙箱」是三個 network namespace，「目的地」是一條假上行後面的第四個 namespace。

**所以本目錄不是 `YYYY-MM-DD-<node-id>/`**（ADR-022 §5 給正式證據的命名），名字裡刻意寫著 `nested-dev-container`：沒有節點。

**T5 的 46 項覆蓋一格都不能改。** N-01～N-08 的判定仍然是 `unknown`，理由不是「沒跑」，是**跑的不是那個東西**。

## 那為什麼要跑

因為在 2026-08-27 之前，`infra/egress/rendered/nftables.conf` **從來沒有被任何一個核心載入過**。

CI 有的是 `render.py --check`，它證明的是「這個檔案與 allow-list 一致」。**它證明不了 nft 會接受這個檔案**，而一個載入失敗的 ruleset 留給節點的是**一條規則都沒有**——那是失效開放的方向。同理，T5 此前沒有任何可執行的形式：部署批會在一台沒人除錯過的機器上、在節點已經在佈建的時間壓力下，第一次把它寫出來。

## 四次執行

| 檔案 | 是什麼 | 結果 |
| --- | --- | --- |
| [`T5/ruled.txt`](T5/ruled.txt) | 載入渲染出的 ruleset，跑 T5-1～T5-9 ＋兩個反向驗證 | **12 項全部 refused、兩個反向驗證通過、NXDOMAIN 成立** |
| [`T5/negative-control.txt`](T5/negative-control.txt) | 同一組探針，**不載入任何規則** | **11 項到達目的地** ⇒ 這些探針看得見失敗 |
| [`T5/mutation-metadata.txt`](T5/mutation-metadata.txt) | 載入前拿掉 `skillhub-drop-metadata` 那一條 | **T5-3 轉 FAIL** ⇒ 這個評分器會紅 |
| [`T5/ci-ubuntu-latest-first-run.txt`](T5/ci-ubuntu-latest-first-run.txt) | **同一支腳本第一次在 CI 的 `ubuntu-latest` 上跑**（核心 `6.17.0-1022-azure`） | **T5-4 `the attempt succeeded`** ⇒ 見下方 ⑤ |

執行環境見 [`versions.txt`](versions.txt)。腳本：[`tools/sec009/t5-network-egress.sh`](../../../../../../tools/sec009/t5-network-egress.sh)，自 2026-08-27 起由 `egress-allowlist.yml` 每次觸發時在 CI 上跑（正向與負對照各一次）。

## 這一次量到而此前沒有人知道的五件事

### ① 委任出去的那個 ruleset，nft 收得下

`infra/egress/rendered/nftables.conf` 載入成功，**零個 accept 規則**，也就是失效關閉的那個方向。這是它第一次被證明「載得起來」而不只是「與來源一致」。

### ② T5-4 的東西向阻擋依賴一個 repo 裡沒有任何地方寫著的核心模組

同一個 bridge、同一個網段上的兩個沙箱，彼此之間走的是二層；**`ip` 的 forward hook 根本看不到那些 frame**，除非 `net.bridge.bridge-nf-call-iptables=1`。

規則本身讀起來像是它自己就擋得住：

```
iifname $SANDBOX_IFACE oifname $SANDBOX_IFACE counter log prefix "skillhub-drop-eastwest " drop
```

**它擋得住，但前提在別的地方。** 本機核心預設就載著 `br_netfilter` 且該 sysctl 是 `1`，所以本機那一輪是綠的。

**這不是一個假設性的風險，見下方 ⑤：同一支腳本在 CI 的 stock Ubuntu 核心上，這一項是紅的。**

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

### ⑤ 在 CI 的 stock Ubuntu 核心上，東西向**沒有被擋住**——而這支腳本原本正在幫它遮掩

把這支腳本接上 CI（`egress-allowlist.yml`）之後的第一次執行，`ubuntu-latest`（核心 `6.17.0-1022-azure`）：

```
  T5-4 east-west to another run          FAIL    the attempt succeeded
  ...
  bridge-nf-call-iptables:  absent
```

**一個沙箱連到了同一台節點上的另一個沙箱。** 那正是 ADR-022 Q2 條件 2 要關掉的、**不需要任何逃逸**的跨 Run 路徑。stock 的 Ubuntu 節點**預設不載 `br_netfilter`**，所以那條 `iifname X oifname X drop` 從來沒有看到過那些 frame，計數器停在零，而規則讀起來完全正常。

**而且同一個機制擋不住的不只這條規則**：ADR-022 Q2 條件 2 指定的另一個手段 `--icc=false`，是 Docker 在 FORWARD 鏈插的 DROP，**依賴的是同一個模組**。兩個手段一起失效，失效的方式是安靜的。

**要記的是這一段的順序，而不只是結論。** 這支腳本的第一版在 setup 階段就把那個 sysctl 寫成 `1`，而且沒有印出來：

- 在本機，模組本來就載著 ⇒ T5-4 綠 ⇒ **這個依賴從頭到尾沒有出現在任何一行輸出裡**。
- **setup 步驟在動手修補它自己等一下要去找的那個洞。**
- 是 CI 上那個沒有模組的核心讓它露出來的——**換一台機器，是唯一會讓這種缺陷現形的事**。

修法**不是**把那一行拿掉（拿掉只會讓 CI 永遠紅，而紅的原因是節點設定，不是規則寫錯），而是：

1. 腳本改成**先嘗試 `modprobe br_netfilter`**——也就是節點的 IaC 必須做的那件事——並且**把「這是實驗室自己載/開的」印在結果裡**。今天本機那一行是 `1  (loaded or enabled by the lab: no)`。
2. 腳本輸出多一段 **`NODE REQUIREMENT (T5-4)`**，把它從註腳升成一列結果：**沒有它，那條規則永遠計零，兩個 Run 互通。**
3. 同一句話寫進 **`nftables.conf` 的規則旁邊**（由 `render.py` 產生），因為在節點上讀那個檔的人，不會來讀這份報告。
4. **閘門 A 的節點准入探針要把它列為一項檢查**——`infra/egress/rendered/` 裡沒有任何東西能自己保證這件事，而節點准入是唯一有機會擋下一台沒設好的節點的地方。

**T5-4 這一列的綠，從今天起只代表「節點設成這樣時，規則有效」，永遠不代表「節點是安全的」。**

## 仍然做不到、也沒有假裝做到的

- **T5 的節點版**：真節點、生產 IaC、真閘道位址、`pinned_ip` 不是 `unset`（ADR-022 §2 的三個前置條件）。
- **N-06 的另一半**：規則有 `log prefix` 與 counter，**但沒有任何東西把它們收去保存 90 天**。本次量的是「有沒有留下計數」，不是「紀錄有沒有被保存」。
- **每 Run 一個 netns**：`dockerdrv` 今天不指定 namespace（`HostConfig` 的欄位刻意留零值），dev 仍是共用一張 Docker network。本次的 netns 是實驗室自己建的，**不是產品程式建的**。
- **T5-2 的 1000 個 port**：本次一個 port。每個 port 都要吃滿 drop 的逾時，而這一項問的是那個網段可不可達。節點版要跑完整掃描。
- **T1 點名的四個 CVE PoC**、T6、T7、T10：與本批無關，仍在部署批。
