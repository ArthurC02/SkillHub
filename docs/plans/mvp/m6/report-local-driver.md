# M6 前期量測：本機行程 Driver 有沒有現成的輪子

- 日期：2026-08-28
- 決策落點：[ADR-059](../../../adr/ADR-059-the-clean-mode-execution-driver-is-honest-about-not-being-a-sandbox.md)
- 姊妹報告：[report-sandbox-options.md](report-sandbox-options.md)（為什麼沒有沙箱）、[report-inmemory-postgres.md](report-inmemory-postgres.md)、[report-object-storage.md](report-object-storage.md)

## 0. 這份報告最重要的一句話

**沒有現成的輪子，而且輪子本身只有約 120 行——但最像輪子的那一個，正好是本 repo 反覆記載的那種缺陷。**

`go-cmd/cmd` 有 1027 顆星，README 寫著「It works on Linux, macOS, and Windows」，並宣稱 `Stop` 實作了殺行程群組所需的 "low-level magic"。它的 `cmd_windows.go` 全文是：

```go
func terminateProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil { return err }
	return p.Kill()                              // 只殺父行程
}
func setProcessGroupID(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{}     // 空結構，什麼都沒設
}
```

對照 `cmd_linux.go` 確實有 `Setpgid: true` 與 `kill(-pid)`。**Linux 那半是對的，Windows 那半是空殼，而同一份 README 把兩者描述成同一件事。**

**信了它會怎樣，本報告量過（§2）**：孫行程存活。而測試多半是綠的——因為父行程確實死了，**而測試通常只檢查這一點**。

## 1. 需求：不是隔離，是生命週期

`apps/sandbox/internal/sandbox` 的 `Driver` 是 11 個方法，工作負載是**單一個 `node /opt/skillhub/run.mjs`**。第二個實作 spawn 的是完全相同的東西，只是 `/work`、`/out` 換成主機目錄。

| 方法 | 本機做法 | 難度 |
| --- | --- | --- |
| `Start` | `exec.Command` ＋ per-run 目錄 ＋ 掛進 Job Object／process group | **唯一的難點** |
| `Wait` | `cmd.Wait()` → `Outcome{ExitCode, Output}` | 低 |
| `Stop` | grace 後終止（§4：grace 兩邊都不是合作式窗口） | 低 |
| `Remove` | 關 job handle ＋刪目錄，冪等 | 低 |
| `ReadTrace` | `os.File` ＋ `Seek`／`ReadAt` ＋ `io.LimitReader` | 低 |
| `ReadArtifacts` | `filepath.WalkDir` ＋ `archive/tar`，約 40 行 | 低 |
| `WorkloadDone`／`ReleaseWorkload` | marker 檔（`run.mjs` 已經在等一個 `collected` 檔，最多 60 秒） | 低 |
| `Adopt` | 回空（§5 決策 1 的後果） | 低 |
| `Healthy` | `exec.LookPath("node")` ＋一次 `node --version` | 低 |

## 2. 實測：回收整棵行程樹，兩種做法

在本機（Windows 11、一般使用者、未安裝任何新東西）跑一支探針：讓子行程再生一個 detached 的孫行程，兩者都長睡，然後比較兩種收法。

```
plain  base=5  peak=7 (+2 spawned)  after=6  leaked=1   ← 只 kill 父行程
job    base=6  peak=8 (+2 spawned)  after=6  leaked=0   ← 關掉 job handle
```

**`leaked=1` 就是 `go-cmd/cmd` 在 Windows 上會給你的東西。**

探針用到的全部符號都來自 `golang.org/x/sys/windows`，一次編譯就過，**不需要 CGO、不需要管理員**：

```
CreateJobObject              JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
SetInformationJobObject      JOBOBJECT_EXTENDED_LIMIT_INFORMATION
AssignProcessToJobObject     JobObjectExtendedLimitInformation
OpenProcess / CloseHandle    PROCESS_SET_QUOTA / PROCESS_TERMINATE
```

## 3. 逐一裁決

| 候選 | 星數／最後動靜 | 裁決 |
| --- | --- | --- |
| **`go-cmd/cmd`** | 1027★／release 2024-06-23 | **不用**——Windows 端是空殼（§0），README 與原始碼不符 |
| `shirou/gopsutil` | 11904★／今天仍更新 | **不用（就此用途）**——只殺單一 PID，樹狀終止要自己遞迴 Toolhelp32 快照，**本質競態**；無 Job Object。（做行程指標觀測則是好東西） |
| `kolesnikovae/go-winjob` | 21★／release **2020-06-30** | **不用**——`go.mod` 停在 go 1.14；且**只有 Windows 半邊**，Unix 半與平台切分照樣要寫，換來約 60 行 |
| `hashicorp/go-reap` | 68★／無 release | **不用**——README 明文不支援 Windows；解的是容器 PID 1 收殭屍，與本題無關 |
| `mitchellh/go-ps` | 1490★／**已封存** | **不用**——只列舉不終止 |
| `creack/pty` | 2082★ | **不用**——Windows 上直接回 `ErrUnsupported`，且本工作負載不需要 PTY |
| `criyle/go-sandbox` | 270★／無 release tag | **不用**——README 明文「Only works on Linux」 |
| `nektos/act`／`dagger` | — | **不用**——執行模型本體就是容器，**等於用更大的 Docker 取代 Docker** |
| `go-task/task` | — | **不用**——`internal/` 不可 import，且做的是跨平台 shell 直譯 |
| `drone-runner-exec` | 63★／**已封存**，停在 2022 beta | **不用** |
| `mholt/archives` | 441★ | **不用**——為 stdlib 40 行的事換 **17+ 個相依**與用不到的多格式偵測 |
| **`golang.org/x/sys`** | 官方／已在相依樹 | **用**——Job Object 全套已出好；缺 `OpenJobObject`、`IsProcessInJob` 與 CPU rate 結構（後者自己宣告約 5 行） |
| **`buildkite/agent` `internal/process`** | 1049★／release **2026-08-28**，MIT | **用，但用抄的**——`internal/` 不可 import，授權允許複製。約 120 行含平台切分與降級路徑 |
| **`archive/tar`** | stdlib | **用** |

## 4. 三件在動工前該知道的

**① 我原本的一個前提是錯的：Go 的 `SysProcAttr` 沒有 RLimit 欄位。** 標準庫不能直接對子行程 setrlimit。Linux 無 root 時可用的是 `UseCgroupFD`／`CgroupFD`（**Go 1.20 起**），而那要 systemd 有委派 memory controller——裸容器、非 systemd 發行版、舊 systemd 都不成立，**必須 runtime 偵測後降級，不能假設**。跨平台的共同底線是 `node --max-old-space-size`（不是 OS 強制的硬牆，native 記憶體不受管）。

**② `KILL_ON_JOB_CLOSE` 表示服務重啟會殺掉跑到一半的 Run。** 那個旗標的語義就是「最後一個 handle 關閉即殺光」，而 handle 隨行程死亡。所以 `Adopt()` 誠實地只能回空。替代做法是具名 job（`OpenJobObject` 依名稱重取）並放棄該旗標，代價是被硬殺時留下孤兒樹。**這是取捨不是 bug。**

**③ `Stop(grace)` 的 grace 在兩個平台都不是合作式窗口，而這是既有行為。** 研究擔心「Windows 沒有 SIGTERM，所以 grace 會失效」——查證本 repo 之後，這個顧慮比看起來小：**`run.mjs` 一個訊號 handler 都沒有**（`grep process.on` 零筆），而 `dockerdrv.Stop` 走的 `ContainerStop` 送的正是 SIGTERM。**今天就沒有人在聽。** 真正的收集窗口是 `WorkloadDone`／marker 檔那個機制。

**一個沒有函式庫能解的競態，要知道但不必解**：`CreateProcess` 與 `AssignProcessToJobObject` 之間有窗口——Go 的 `os/exec` 回傳時子行程已 resume。乾淨解法要 `CREATE_SUSPENDED` 或 `PROC_THREAD_ATTRIBUTE_JOB_LIST`，而 **Go 兩者都沒開放**（golang/go#32404、#44005 仍 Open）。Buildkite 的做法是接受它。

## 5. 沒有量到的、以及本報告不主張的

- **只實測了第 2 節那一項**，而且只在 Windows 上。Linux 側的 process group 收法**沒有跑過**。
- **未量**：Job Object 的記憶體／CPU 上限對 Node 的實際效果，以及 `CreateProcess`→assign 競態窗口的實際寬度。
- **未查證**：「Job Object 不需要 admin」在微軟文件裡是**負面證據**（文件沒說要）＋本報告第 2 節那次一般使用者身分的成功執行。**沒有找到明文的「不需要」。**
- **不主張**：本報告不主張現在就動工。它主張的是**這件事沒有值得加的相依**，以及第 4 節那三件要先知道。

## 6. 訂正（2026-08-29，實作時查出）：§2 那個實測是對的，但它證明的東西比我寫的窄

`PORT-010b` 動工時照鐵律 9 做突變——把 `terminate()` 還原成只殺父行程，預期測試變紅。**它沒有紅。**

追下去的原因是：**Windows 上 Node 會把自己非 detached 的子行程放進 Node 自己的 Job Object**，所以 `node.exe` 被殺掉時（不論是誰殺的），它自己的 job handle 隨行程結束一起關閉，**連帶把子孫殺光**。這與本套件的 Job Object 完全無關——純 PowerShell `Start-Process` 加一個**不帶 `/T`** 的 `taskkill` 就能重現，Go 沒有介入。

**也就是說，那支測試量到的是 Node 的保證，不是我們的。**

把孫行程改成 `detached: true`（脫離 Node 自己那張網）之後，同一個突變才真的紅：

```
grandchild survived Stop: heartbeat grew from 42 to 140 bytes after the driver
reported the sandbox stopped — only the direct child was reaped, not the whole tree
--- FAIL: TestReapsWholeProcessTree
```

並且活下來的孫行程鎖住工作目錄，讓 `t.TempDir()` 的清理也一起失敗——**額外一條佐證，說明洩漏是真的**。

**所以 §2 的敘事要收窄。** 這個 Job Object 買到的**不是**「Windows 上父行程死了子行程不會死」——一般情況下 Node 自己就處理掉了。它買到的是：**一個刻意 detach 或背景化自己的行程逃得掉 Node 那張網**，而那正是有敵意或粗心的工作負載會產生的東西。§2 的 `leaked=1` 讀數本身沒有錯（那支探針的孫行程就是 detached 的），錯的是我從它推出的那句話的範圍。

**`go-cmd/cmd` 的裁決不變**：它的 Windows 端仍然是空殼，只是它會壞在哪一種情況，比 §0 寫的更精確。

## 7. 同時查出一個不在本報告範圍、但擋住 `PORT-010` 端到端的東西

`infra/images/runtime-agent-sdk/run.mjs` 有**三個寫死的 POSIX 外部程式**：`/bin/sleep`（`:145`、`:176`）與 `unzip`（`:199`）。**Windows 主機上三者都不存在**，`execFileSync` 直接丟未捕捉例外；而本機 Driver 的 `pushInputs` 在行程啟動**之後**才寫 `ready` marker，所以工作負載一定會先進到那個等待迴圈。

**§1 那句「第二個實作 spawn 的是完全相同的東西，只是把 `/work`、`/out` 換成主機目錄」對 Linux 主機成立，對 Windows 主機不成立**——而 M6 的目標機器是後者。

已記為 [`04` 丙-82](../../04-backlog-and-handoffs.md)。**本次的測試 fixture 用 `Atomics.wait` 繞開它**，而那個繞開正是問題本身：**在容器裡永遠不會發現這件事，因為映像裡三個都在。**
