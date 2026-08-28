# M6 前期量測：物件儲存的替身，以及它證明不了的那一半

- 日期：2026-08-28
- 決策落點：[ADR-058](../../../adr/ADR-058-the-clean-test-mode-is-real-postgres-behind-the-api-seam.md)、允收見 [`02` §4.10](../../02-specifications-and-acceptance-criteria.md) `PORT-009`
- 姊妹報告：[report-inmemory-postgres.md](report-inmemory-postgres.md)（資料庫那一半）

## 0. 這份報告最重要的一句話

**一個 in-process 的假 S3 可以讓那七個方法全部通過，而它通過的原因是「沒有東西能拒絕它」。**

本報告的第一版差點以相反的結論收尾——「換掉測試替身就讓 `SBX-008` 第一次有真的測試」。那句話是錯的，而抓到它的方式是**對自己的實驗做突變**：把簽章改掉、把簽章整個拿掉、讓它過期、拿「讀」的票去「寫」。四種都應該被拒絕，四種都回 HTTP 200。

**這是本 repo 反覆記載的那種缺陷，而它差一點在一份宣稱要修補它的報告裡被製造出來。**

## 1. 為什麼物件儲存是與資料庫分開的一半

[report-inmemory-postgres.md](report-inmemory-postgres.md) §2 已經查證：**那 287 筆靜默跳過的測試不需要物件儲存**——它們用的是行程內的 `map[string][]byte`，碰 `OBJSTORE_*` 的只有兩支另行 gate 的付費 E2E。

所以物件儲存不是「測試跑不起來」的原因。它是**系統跑起來**的需求：真實的 API server 要存取套件與產出物，而受限環境不能跑 SeaweedFS。兩件事的形狀不同，不要混為一談。

存取表面**只有七個方法**（`objstore.Client`）：`EnsureBucket`、`Get`、`Put`、`Remove`、`Exists`、`PresignGet`、`PresignPut`。

## 2. 實驗一：in-process 的假 S3，七個方法全過

`johannesboyne/gofakes3` ＋ `s3mem` backend，以 `httptest.NewServer` 起在**測試行程內**——沒有外部程序、沒有二進位、沒有 Docker。對面是專案實際使用的 minio-go client。

```
PASS  EnsureBucket (MakeBucket)
PASS  Put
PASS  Get (位元組相同)                36 bytes
PASS  Exists (StatObject)
PASS  PresignGet (產生)
PASS  PresignGet (真的抓得到)          HTTP 200, 36 bytes
PASS  PresignPut (產生)
PASS  PresignPut (真的上傳得了)         HTTP 200，讀回 12 bytes
PASS  Remove
PASS  Remove 之後 Exists 應為否         不存在，正確
```

**看起來是完整的十項通過。** 而其中兩項的意義與其餘八項完全不同。

## 3. 實驗二：對自己的結論做突變——四個都沒有把關

同一個 fake，同一個 client，四種應該被拒絕的請求：

| 突變 | 期望 | 實際 |
| --- | --- | --- |
| ① 正常的簽名 URL | 通過 | HTTP 200 ✅ |
| ② **簽章被竄改**（`X-Amz-Signature=deadbeef…`） | 拒絕 | **HTTP 200** ❌ |
| ③ **完全拿掉 `X-Amz-Signature` 與 `X-Amz-Credential`** | 拒絕 | **HTTP 200** ❌ |
| ④ **已過期 3 秒**（簽 1 秒效期後 sleep 3 秒） | 拒絕 | **HTTP 200** ❌ |
| ⑤ **拿 presigned GET 的 URL 去做 PUT** | 拒絕 | **HTTP 200，而且物件真的被覆寫**：`"overwritten by a GET-only token"` ❌ |

**gofakes3 沒有任何一行程式碼在檢查簽章**（repo 根層無 SigV4／auth 處理；`chunk.go` 連 streaming chunk signature 都是直接丟棄）。所以 §2 那兩項 `PASS`，測到的是「URL 打得通」，**不是「授權成立」**。

**這不是 gofakes3 的缺陷，是它的定位。** 同一種取捨在同類工具裡是普遍的，而且有些寫在自己的文件裡：`fsouza/fake-gcs-server` 明說「可以用 signed URL，但不對 signature 與 expiration 做任何驗證」；`adobe/S3Mock` 明說「接受 presigned URL 但忽略所有參數」。

## 4. 所以 `SBX-008` 的證據該從哪來

`02:SBX-008` 要保證的是**短效授權**。這四個字裡：

| 性質 | 假的 S3 能證明嗎 |
| --- | --- |
| URL 產生得出來、參數組得對 | ✅ 能 |
| 「短效」——過期後失效 | ❌ **不能** |
| 「授權」——簽章被驗、不可竄改 | ❌ **不能** |
| 單一路徑——GET 的票不能用來 PUT | ❌ **不能** |

**正確的分工是把測試切成兩半**：

1. **URL 生成與解析**：純單元測試對字串斷言（`X-Amz-Expires` 的值、method、endpoint 覆寫、key 前綴），**不需要任何 server**。
2. **簽章真的被強制**：只能靠一次**對真 SeaweedFS 的執行**，在能跑 Docker 的 CI 上，不在受限機器上。

而第 2 項有一個必須一起做的規矩，來自業界的實作：**那個測試要被明文禁止 skip**。`restic` 的 `RESTIC_TEST_DISALLOW_SKIP` 把 MinIO 與 S3 兩支測試列進不准跳過的清單，理由正是——**這一類測試最常見的死法不是變紅，是在某次 CI 調整裡被安靜地跳過，而沒有人發現**。

> **這一條與本 repo 的現況直接相關**：今天平台測試裡**沒有任何一行 fetch 過 presigned URL**（`grep objects.test` 只有產生它的兩行），所以那三個性質今天是零覆蓋。**引入一個不驗簽的假 S3，會把「零覆蓋」變成「看起來有覆蓋」——那比零覆蓋更糟。**

## 5. 唯一會驗簽的純 Go 選項，以及為什麼不建議

`mojatter/s2`（v0.12.7，2026-08-18）README 明說會驗證 query string 裡的 SigV4，是本次盤點中**唯一**真的檢查 presigned URL 的純 Go 實作。

**不建議採用**：35 顆星、159 commits、2026 年才出生、無 versioning、無 ACL/IAM、單一帳號。把它放進 CI 的關鍵路徑，等於用「簽章有被檢查」換來一個新的相依。**與其如此，不如老實承認假的那一側不驗簽，把驗簽的證據放在真環境那一次執行上**——後者是可查證的事實，前者是一個賭注。

## 6. 瀏覽器那一半：`blob:` 只是形狀像

離線、以 `file://` 開啟的頁面裡，「物件儲存」能是什麼：

| 機制 | `file://` 下可用 | 二進位友善 | 與 presigned URL 的距離 |
| --- | --- | --- | --- |
| 記憶體 `Map<string, Blob>` | ✅ 一定可用 | ✅ | 不是儲存語意，配 `blob:` 補上「取得檔案」 |
| **`blob:` URL** | ✅ 可用（`blob:null/<uuid>`） | ✅ | **最接近，但只有形狀像**——見下 |
| IndexedDB | ⚠️ Chrome 可、Firefox 預設可、**Safari 預設不可**（開本機檔案存取 storage 拋 SecurityError） | ✅ | 只有存取，沒有 URL 也沒有能力傳遞 |
| **OPFS** | ❌ **不要碰** | ✅ | OPFS 需要 secure context，而 `file://` 是 opaque origin、一律判定 not trustworthy。**未查到能直接引用的官方說明，所以這是規格推導不是實測**——這種不確定性本身就不該進 demo 的關鍵路徑 |
| Cache API / Service Worker | ❌ SW 不能在 `file://` 註冊 | — | — |
| localStorage | ⚠️ Safari 同上 | ❌ 只能存字串，二進位要 base64（+33%） | — |

**`blob:` 與 presigned URL 的三個差別，每一個都會讓人誤會**：

1. **沒有到期時間**——只在 `revokeObjectURL()` 或 document unload 時失效。
2. **綁在建立它的 document**，換頁即死，**遞不出這個分頁**。
3. **沒有簽章，也沒有方法區分**——不存在「presigned PUT」這回事。

**所以 demo 模式能演示的是「按下去有檔案出來」，演示不了「短效授權」。這句話必須寫在 demo 旁邊**（`02:PORT-003` 的揭露義務涵蓋它），否則遲早有人指著 demo 說沙箱授權已經驗過了。

**還有一個更省的角度**：前端只經由 `apiFetch` 一個函式對外，demo 模式的攔截點就在那裡——**根本不需要在瀏覽器內重建任何 S3 語意**。回一個帶 `blob:` URL 的假回應即可；去實作 bucket、key、presign 這些概念，只會複製一套永遠不會被驗證的假語意。

## 7. 業界怎麼避免「假的與真的行為分歧」——三種手法，都不靠 code review

- **一套測試兩邊跑**：CNCF `distribution` 的 `storagedriver/testsuites` 用 `RegisterSuite` 讓 `inmemory`／`filesystem`／`s3-aws` 跑同一組斷言；`thanos-io/objstore` 的 `objtesting.ForeachStore` ＋ `AcceptanceTest` 同理。**分歧不是被討論出來的，是被同一組斷言在假的那邊照出來的。**
- **錄真放假**：`gocloud.dev/blob` 的 `drivertest.RunConformanceTests` 把 `SignedURL` 列為 conformance 的一步，並以 `testdata/**/*.replay` 重放對真實雲端錄下的互動——**假的行為的來源是真服務的錄影，不是實作者的想像。**
- **禁止 skip**：`restic` 的 `RESTIC_TEST_DISALLOW_SKIP`（見 §4）。

## 8. 沒有量到的、以及本報告不主張的

- **未量**：`gofakes3` 對 Range request、multipart upload、`Content-Type` 保存、以及 minio-go 的 chunked upload 路徑的行為。本報告只打了那七個方法的主路徑。
- **未量**：瀏覽器端的任何東西。§6 全部來自文件與規格推導，**一項都沒有實測**——尤其 OPFS 在 `file://` 下的行為是規格推導。
- **不主張**：本報告不主張採用 gofakes3。它主張的是**如果採用，要清楚它證明什麼**，以及 `SBX-008` 的證據必須另尋來源。
- **一個順帶查證**：`minio/minio`（server）於 2026-04-25 封存唯讀，但本專案的 server 是 SeaweedFS，client 是 `minio/minio-go`（Apache-2.0，**未封存**），**所以那次封存不直接影響本專案**。這一句寫在這裡是因為兩者名字太像，值得先講清楚以免日後被誤引。

## 9. 可重跑

腳本置於 scratchpad 未進 repo。兩支的形狀：

1. **七方法**：`httptest.NewServer(gofakes3.New(s3mem.New()).Server())`，minio-go 以 `credentials.NewStaticV4` 指過去，逐一呼叫並**實際 fetch presigned URL**（不要只斷言字串產生得出來——那正是本報告差點犯的錯）。
2. **突變**：對同一個 presigned URL 做四種破壞（竄改簽章、刪除簽章、等待過期、以 PUT 使用 GET 的票），**四種都應該被拒絕**。任何一種回 2xx，就代表那個替身對 `SBX-008` 沒有證明力。
