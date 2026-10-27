# tools/domain-memory-engine

Domain Memory 外掛的命令引擎，以 Rust 重寫。終點是外掛的 Python 只剩一層啟動器：找到對應平台的執行檔，原樣轉交 argv、stdin、stdout、stderr 與 exit code；MCP 也是同一個執行檔的 `mcp-serve`，經 stdio 說 JSON-RPC。規則、計算、檔案與 git 操作全部在 Rust。

## 隔離

- 這個 crate 不在外掛目錄裡，外掛的程式、文件與發行摘要都不引用它；外掛使用者在遷移之前只會跑到 Python。
- 外掛仍是行為的唯一來源。Rust 追的是外掛現在的輸出，不是自己的版本；外掛改了行為，這裡的一致性比對會紅，由下一批補上。
- 尚未移植的命令在解析完參數後以 `ERROR: <command> is not available in the Rust engine yet` 結束，exit 1。

## 契約

`contract/cli.json` 由 `parity/extract_cli_contract.py` 從外掛的 argparse 擷取：每個命令的選項、型別、選擇值、預設值、互斥群組，以及已排版的 usage 與 help 文字。擷取固定用 Python 3.13、程式名稱 `registry_tools.py`、80 欄寬，因為 argparse 的錯誤措辭隨 Python 版本不同、help 的換行隨終端寬度不同，引擎只能對齊其中一種。`--check` 只比對不寫入，CI 用它擋契約漂移。

引擎照這份契約解析參數，重現 argparse 的行為：長選項的唯一前綴、`--option=value`、負數當值、`-h` 後接字母仍印 help、型別與選擇值的錯誤、必填與互斥群組；錯誤印該命令的 usage 加 `<prog>: error: <訊息>`，exit 2；`counterfactual` 的用法錯誤改以 JSON 回覆、exit 3。

已知差異：help 的換行固定在 80 欄，Python 版會跟著終端寬度變；JSON 裡單獨的 surrogate 跳脫在引擎裡變成 U+FFFD。

## 一致性比對

`parity/parity.py` 以同一組參數分別跑外掛（`registry_tools.py`）與引擎，逐位元比對 stdout、stderr 與 exit code，必須用 Python 3.13 執行。案例有兩種：每個命令自動產生的 `-h` 與未知選項，以及 `parity/cases.json` 手寫的案例；後者可以用 `files` 在暫存目錄造出資料，參數裡的 `{fixture}` 換成那個目錄，`{repo}` 換成 repo 根目錄。

一個命令算移植完成，條件是它的每一條輸出分支（成功、每一種拒絕、每一種讀取失敗）都有案例而且一致。新案例要先對外掛跑過，確認它走到想測的分支，再對引擎跑。

目前移植完成的命令：`list-proposals`、`export`、`get-record`、`get-context`、`validate-policy`、`lookup`、`resolve-terms`、`dependents`、`analyze-boundary`、`coverage`。案例的 `git: true` 會把 fixture 提交成 git 倉庫，`env` 設定環境變數；git 逾時的訊息只有單元測試，因為逾時與否取決於機器速度。

Python 的 `\d` 依 Python 3.13 的 Unicode 資料判斷，表格在 `src/digits.rs`。搜尋用的 NFKC、casefold 與字詞字元表在 `src/unicode_data.rs`，由 `parity/generate_unicode.py` 以 Python 3.13 產生，不手改；換 Python 版本時重新產生。

## 常用指令

從這個目錄執行，Rust 版本由 `rust-toolchain.toml` 釘住：

```sh
cargo fmt --check
cargo clippy --all-targets --locked -- -D warnings
cargo test --locked
cargo build --release --locked
python3.13 parity/extract_cli_contract.py --check
python3.13 parity/parity.py
```

## 遷移

全部命令一致之後才規劃遷移。屆時要決定的是：各平台執行檔怎麼隨外掛發行（外掛以 `git archive` 被複製，執行檔必須在那份內容裡或能被可驗證地取得）、啟動器在找不到執行檔時怎麼辦，以及外掛的 Python 實作與測試何時刪除。
