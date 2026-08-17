# tools/qa

`03` §17 的品質保證需求裡，**需要一份可重跑的語料才判定得了**的那些。目前只有一項。

| 目錄 | 需求 | 一句話 |
| --- | --- | --- |
| [`skillpkg-corpus/`](skillpkg-corpus/) | `QA-002` | Agent Skills 規格驗證的**刻意破壞變體**與其期望 finding；合法樣本那一半是既有的 45 個 pin commit 套件 |

同 `tools/content`：**驗證工具，不是產品程式碼**——不進 CI、不被服務引用。差別在期望清單是給 Go 測試讀的資料檔，不是留給人看的快照。

## `skillpkg-corpus`

`02:SKILL-002`／`03:QA-002` 的資料集缺的從來不是好樣本。45 個 pin commit 套件證明的是「不誤擋」；**證明「擋得住」要靠會被擋的樣本，而那些一個都沒有**。這裡補的就是那一半。

| 檔案 | 內容 |
| --- | --- |
| [`generate.py`](skillpkg-corpus/generate.py) | 生成腳本。抓 3 個 pin commit 基底套件（重用 `tools/content/import_seed.py` 的重打包），逐一套上 21 個破壞變體 |
| [`expected-findings.json`](skillpkg-corpus/expected-findings.json) | 每個變體的期望 finding（code × severity）、`blocked`、以及**真缺陷的 `gap` 欄** |
| `services/platform/internal/ingest/qa002_corpus_test.go` | 對照用的 harness。走的是匯入路徑的同兩支呼叫：`PackageFS` → `skillpkg.Validate` |

**入庫的是腳本不是 45 份二進位**：變體是一個函式一行，讀得出哪裡壞了；committed zip 是不透明的，而且會對著 `skillpkg` 悄悄腐爛。

### 怎麼跑

```bash
python tools/qa/skillpkg-corpus/generate.py --list          # 變體表
python tools/qa/skillpkg-corpus/generate.py --selftest      # 離線，不需網路
python tools/qa/skillpkg-corpus/generate.py --out /tmp/corpus

cd services/platform
QA002_CORPUS=/tmp/corpus go test ./internal/ingest -run QA002 -v
```

**沒有 `QA002_CORPUS` 就跳過**，形式比照 `SKILLHUB_TEST_DATABASE_URL` 的既有前例——生成要抓三個 pin commit repo 壓縮檔，CI job 沒有那個網路預算。生成一次可重複使用；`--cache` 與 `import_seed.py` 共用。

### 覆蓋的破壞類型

`03:QA-002` 點名的九類全部有變體，另加 archive 層四項：

| 類型 | 變體 |
| --- | --- |
| 缺 `SKILL.md` | `skill-md-missing` |
| frontmatter 壞 | `frontmatter-missing`／`-unterminated`／`-invalid-yaml`／`-unknown-field` |
| 必要欄位缺 | `name-missing`／`description-missing` |
| 名稱違規 | `name-invalid`／`name-too-long`／`description-too-long` |
| 檔案引用逃逸 | `file-ref-escapes`／`file-ref-missing` |
| 內嵌可執行程式碼 | `embedded-script`／`binary-file` |
| 疑似 Secret | `secret-in-script`／`secret-in-skill-md` |
| zip 炸彈 | `zip-bomb` |
| 路徑逃逸的 entry | `zip-path-traversal`／`zip-absolute-path`／`zip-symlink-escape` |
| （另加）掃描上限 | `oversize-file` |

### 兩條紀律

1. **期望清單是審過的，不是錄下來的。** 觀測行為若弱於該失敗模式應有的強度，那一列寫 `gap` 欄如實記著，**不改期望去遷就**、也不寫成 pass。目前有三列帶 `gap`（`zip-path-traversal`／`zip-absolute-path`／`zip-symlink-escape`），逐項理由見 [`m4/README.md` §14.1](../../docs/plans/mvp/m4/README.md)。
2. **假憑證在執行時才組出來**，原始碼裡沒有任何一段完整的憑證字串——會被自己的 pre-push 掃描擋下的語料是沒人 commit 得進來的語料。`--selftest` 有一條斷言直接檢查這件事。
