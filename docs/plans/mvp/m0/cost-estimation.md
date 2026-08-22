# M0：部署平台成本試算

- 狀態：**試算完成（v2，經複核修正；§6.2 已依模型供應商定案增補 v3 重估），ADR-014／ADR-015 定案待負責人確認**
- 日期：2026-08-13（v1）／2026-08-13 修訂（v2）／**2026-08-14 §6.2 增補 v3**（模型供應商定案為 OpenAI，見 §6.2.3 與 §9.4）
- 用途：解除 [ADR-014](../../../adr/ADR-014-core-infrastructure-selection.md)（「部署平台確認與成本試算」）與 [ADR-015](../../../adr/ADR-015-sandbox-isolation-technology.md)（「部署平台支援驗證」）的定案前置條件；同時回應 [01-goals-and-plan.md](../../01-goals-and-plan.md) 第 12 節風險「Sandbox 成本不可持續」。
- 幣別：USD。所有價格為 2026-08-13 查詢之公開牌價（On-Demand／隨用隨付，未套用任何承諾折扣或新創補助）。
- 匯率假設：1 EUR = 1.1538 USD（[Federal Reserve H.10, 2026-08-12](https://www.federalreserve.gov/releases/h10/hist/dat00_eu.htm)）。

> **v2 重要變更**：(1) Hetzner 單價全面更新為 **2026-06-15 官方調價後**牌價（v1 誤用調價前數字，CCX 線低估達 2.9 倍）；(2) 情境 (b) 補上遺漏的 N+1 節點冗餘；(3) Sandbox slot 記憶體由 2 GiB 修正為 PDM-005 的 **4 GiB**，連帶更換各平台機型系列；(4) Neon 依 §2.2 規格計 CU；(5) 依負責人 2026-08-13 指示新增 **E. 雲原生容器化自架**組合並改寫推薦框架。完整清單見 §9。

---

## 0. 本文件回答什麼問題（v2 框架）

負責人於 2026-08-13 指示：**Postgres、物件儲存等服務應以容器替代受管服務，走雲原生架構概念。** 因此本試算的問題已從「選哪家受管服務」改為：

1. **跑在哪一家的 VM／受管 Kubernetes 上最划算？**
2. **哪些元件容器化不划算，應維持受管作為例外？**
3. **E1（單／少節點 compose 或 k3s）→ E2（受管 Kubernetes + CloudNativePG）的切換觸發條件是什麼？**

受管服務組合（AWS／GCP／DO 全家桶／Hetzner+Neon）**降為對照組**，仍在表中並修正到正確數字，用途是量化「容器化到底省多少、值不值營運工時」。

> ⚠️ **與 ADR-014 的取向衝突**：ADR-014（Proposed）選型原則 1 為「受管服務優先，不自架可以買到的東西」，決策表明列「受管 PostgreSQL」。本文件的方向與該原則相反（**物件儲存維持受管，故該列不衝突；衝突僅在 PostgreSQL 一列**）。定案時須由負責人決定：**(A)** 趁 ADR-014 仍為 Proposed 直接修訂其選型原則與決策表，或 **(B)** 另立 ADR-018 取代並將 ADR-014 標為 Superseded。**本文件不修改任何 ADR。**

---

## 1. 候選平台如何產生

ADR-014 與 ADR-015 **未列出具名候選平台**——ADR-014 只指定能力，ADR-015 只指定執行節點形態。候選集合由 ADR 內文的硬性條件推導：

| ADR 條件 | 出處 | 對平台的篩選效果 |
| --- | --- | --- |
| PostgreSQL 須能跑 FTS + pgvector | ADR-014 決策表、ADR-013 | 容器化後不篩選平台（自帶 extension 的映像）；受管對照組需供應商支援 pgvector |
| S3 相容物件儲存（presigned URL） | ADR-014 決策表 | 需 S3 API 相容 |
| gVisor（runsc）**在一般雲端 VM 即可執行，不需裸機或巢狀虛擬化** | ADR-015 選項 B | **不篩選任何平台**；僅要求可自訂 OS 映像的 IaaS VM |
| 執行節點為**獨立 VM 池**，不與應用工作負載混排 | ADR-015 決策 | **Sandbox 節點不進 Kubernetes**，E1／E2 皆維持獨立 VM 池 |
| Egress 全部經專用 Proxy，default-deny | ADR-015、ADR-005 | 需自建 Proxy 節點；刻意不用受管 NAT Gateway |
| 控制平面採模組化單體 + 少量 Worker，依壓力拆分 | ADR-010 | E1 的節點合併符合此原則；E2 只在壓力觸發後啟用 |
| LiteLLM Proxy 為獨立部署單元 | ADR-017 | 容器化，與控制平面共節點 |
| Langfuse **Cloud**（自架 v3 需 ClickHouse+Redis+S3） | ADR-017 | 固定 SaaS 訂閱，所有組合相同；**明確列為不容器化的例外** |

**關鍵推論**：gVisor 不需 KVM／nested virtualization，因此 Sandbox 節點只需一般雲端 VM，平台選擇不被隔離技術限縮。這使得低價 IaaS 供應商成為合法候選。

據此的候選組合：

| 代號 | 組合 | 角色 |
| --- | --- | --- |
| **E1** | 雲原生容器化，單／少節點（docker compose 或 k3s 單機） | **主要候選**，MVP 形態 |
| **E2** | 雲原生容器化，受管 Kubernetes（DO DOKS，控制平面免費）+ CloudNativePG operator | **主要候選**，成長期形態 |
| A. AWS | RDS PostgreSQL + S3 + EC2 (Graviton) | 對照組，成本上界基準 |
| B. GCP | Cloud SQL + Cloud Storage + Compute Engine | 對照組 |
| C. DigitalOcean | Managed PostgreSQL + Spaces + Droplets | 對照組，單一供應商 |
| D. Hetzner + Neon | Hetzner Cloud VM + Hetzner Object Storage + Neon Postgres | 對照組，受管組合成本下界 |

E1／E2 各在 **Hetzner** 與 **DigitalOcean** 兩家節點供應商上分別試算（AWS／GCP 的 VM 單價已由對照組證明不具競爭力，不重複展開）。

註：Fly.io、Render 等 PaaS 未列入——ADR-015 要求 Sandbox 為自管的獨立 VM 池並可自訂 runsc runtime，PaaS 抽象層阻擋此需求。

---

## 2. 用量假設

### 2.1 每 Run 資源假設

| 假設項 | 值 | 依據 |
| --- | --- | --- |
| Sandbox slot 規格 | **2 vCPU / 4 GiB** | **PDM-005 §5.2 單次 Run 資源上限**（v1 誤用 2 GiB） |
| 單 Run 佔用時間 | **6 分鐘** = 4 分執行 + 1 分 provisioning/gVisor 冷啟 + 1 分清理 | ADR-015「分鐘級」；介於 PDM-010 的中位 2 分鐘與 PDM-005 的 10 分鐘軟上限之間 |
| gVisor 效能損失 | 已內含於上述 6 分鐘（相對 runc 約 +20% wall time） | ADR-015「性能損失需納入 Run 時間與成本估算」 |
| 尖峰集中窗口 | 每日 Run 集中於 8 小時 | 個人創作者使用時段 |
| 突發係數 | 2× 平均併發 | 佇列等待可接受（Run 為非同步） |
| 尖峰併發 slot 數 | `每日Run × 6min ÷ 8h × 2` | — |
| 每節點 slot 數 | `min(節點 vCPU ÷ 2, 節點 GiB ÷ 4)`，且須留宿主 OS 與 gVisor Sentry 餘裕 | **4 GiB/slot 使記憶體成為約束**，見 §2.3 |
| 節點冗餘 | **N+1**（兩個情境一致套用） | ADR-015「節點以 IaC 建置、無狀態、定期重建」需滾動汰換餘裕 |
| 單 Run egress | 10 MB | 文字負載；ADR-017 呼叫路徑 |
| 單 Run 產出物件 | 20 MB | ADR-014「大型 Payload 進物件儲存」 |
| 物件保存期 | 90 天（Trace）／30 天（Artifact） | PDM-006 分級表 |
| 單 Run Langfuse units | 30 | ADR-017 埋點範圍 |
| 每月天數 | 30 | — |

### 2.2 三組用量情境

| 項目 | (a) 封閉測試 | (b) 早期成長 | (a′)(b′) PDM-010 定案值參照 |
| --- | --- | --- | --- |
| 使用者數 | 20 | 200 | 20 ／ 200 |
| 每日 Run | 50 | 500 | 20 ／ 200 |
| 每月 Run | 1,500 | 15,000 | 600 ／ 6,000 |
| 每人每月 Run | **75** | **75** | **30** |
| 尖峰併發 slot | `50×6÷480×2` = 1.25 → **2** | `500×6÷480×2` = 12.5 → **13** | 0.5 → **1** ／ 5 → **5** |
| Sandbox 節點（含 N+1） | **1 + 1 = 2 台**（4 vCPU/16 GiB） | **4 + 1 = 5 台**（8 vCPU/32 GiB） | **1 + 1 = 2 台** ／ **2 + 1 = 3 台** |
| 控制平面工作負載 | 12 vCPU / 24 GiB（Go API、Worker、Python LLM、LiteLLM） | 16 vCPU / 32 GiB + LB | 同 (a) ／ 8 vCPU / 16 GiB |
| Egress Proxy | 1 台小型 | 2 台小型（HA） | 1 台 ／ 2 台 |
| PostgreSQL | 2 vCPU / 8 GiB，100 GB | 4 vCPU / 16 GiB，300 GB | 2 vCPU / 8 GiB ／ 2 vCPU / 8 GiB，150 GB |
| 物件儲存 | 150 GB | 1 TB（+ 備份約 300 GB） | 150 GB ／ 420 GB |
| 月 egress | 50 GB | 400 GB | 20 GB ／ 160 GB |
| Langfuse units／月 | 45,000 | 450,000 | 18,000 ／ 180,000 |

> ⚠️ **(a)、(b) 與 PDM-010 免費額度不一致**：兩個情境都隱含 **75 Run/人/月**，但 [PDM-010](pdm-proposals.md) §8.1 建議的每月滾動上限是 **30 Run/人**。若 PDM-010 照案定案，實際 Run 量只有 (a)/(b) 的 **40%**，Sandbox 池與總成本應以 (a′)/(b′) 為準。(a)、(b) 因此應理解為「額度放寬或引入付費後」的規劃情境，不是 MVP 定案值。§4.4 提供 (a′)/(b′) 的數字。

情境 (b) 的部署形態仍在 [ADR-010](../../../adr/ADR-010-mvp-deployment-and-evolution-path.md) 的「模組化單體 + 少量 Worker」範圍內，未觸發任何服務拆分條件。

### 2.3 Sandbox 節點型號推導（4 GiB/slot 的後果）

slot = 2 vCPU / 4 GiB，即 **4 GiB per vCPU**。這淘汰了所有 2 GiB/vCPU 的運算最佳化機型系列：

| 平台 | v1 用的機型 | 為何不可用 | v2 改用 |
| --- | --- | --- | --- |
| AWS | c7g.xlarge / c7g.2xlarge（8／16 GiB） | 2 slot×4 GiB = 8 GiB、4 slot×4 GiB = 16 GiB，**宿主 OS 與 gVisor Sentry 完全無餘裕** | **m7g.xlarge**（4 vCPU/16 GiB）／**m7g.2xlarge**（8 vCPU/32 GiB） |
| GCP | e2-standard-4／-8 | 記憶體足夠，但 e2 為共享租戶，**與「Sandbox 需可預期 CPU 時間」的要求矛盾**（該要求在 v1 只施加於 DO） | **n2-standard-4／-8**（專用 vCPU） |
| DigitalOcean | CPU-Optimized（2 GiB/vCPU） | 同 AWS，記憶體不足 | **General Purpose**（4 GiB/vCPU，仍為專用 vCPU） |
| Hetzner | CCX23／CCX33 | **本來就是 4 GiB/vCPU 專用 vCPU，無需更換** | CCX23／CCX33（不變） |

單一 8 vCPU / 32 GiB 節點承載 4 slot（用掉 8 vCPU / 16 GiB），留 16 GiB 給宿主與 Sentry，餘裕充足。

### 2.4 明確排除項

- **模型 token 費用**：走 LiteLLM 至供應商，屬變動成本、完全由 PDM-003 模型選擇決定，所有平台無差異，故不納入平台比較。**但這是總帳單的最大單項**——見 §6.2。
- **LiteLLM 授權費**：核心 Proxy 為 MIT 開源自架，$0；僅計節點成本。
- 網域、TLS 憑證（Let's Encrypt $0）、CI/CD 分鐘數、O11y 受管後端（ADR-014 待決策，各平台皆有免費額度）。
- **人力與維運工時不換算為金額**，但在 §5.4 以「每月營運小時」明列——這是容器化方案的主要隱藏成本。

---

## 3. 單價表（2026-08-13 查核）

### 3.D Hetzner Cloud + Neon（**v2 全面更新**）

> **v1 的錯誤**：v1 引用一篇談 **2026-04** 調價的第三方部落格，漏掉 Hetzner 在 **2026-06-15 08:00 CEST** 生效的第二次調價。該次調價 CCX（專用 vCPU）線漲幅 **113%～173%**，CPX 線漲幅約 **154%**。調價適用於**新訂單與 rescale**，新客戶無法取得舊價。下表為官方調價公告值，查核日 2026-08-13。

| 項目 | 規格 | v1 誤用值 | **v2 官方值** | USD |
| --- | --- | ---: | ---: | ---: |
| Hetzner CX33 | 4 vCPU / 8 GiB（共享），20 TB | €6.49 | **€8.49** | **$9.80** |
| Hetzner CX43 | 8 vCPU / 16 GiB（共享），20 TB | — | **€15.99** | **$18.45** |
| Hetzner CX53 | 16 vCPU / 32 GiB（共享），20 TB | — | **€29.49** | **$34.02** |
| Hetzner CPX32 | 4 vCPU / 8 GiB（共享 AMD） | €13.49 | **€35.49** | **$40.95** |
| Hetzner CCX13 | 2 vCPU / 8 GiB（**專用**） | — | **€42.99** | **$49.60** |
| Hetzner CCX23 | 4 vCPU / 16 GiB（**專用**），20 TB | €31.49 | **€85.99** | **$99.21** |
| Hetzner CCX33 | 8 vCPU / 32 GiB（**專用**），30 TB | €48.49 | **€138.49** | **$159.77** |
| Hetzner IPv4 | — | €0.50 | €0.50 | $0.58 |
| Hetzner 流量超額 | — | €1.00/TB | €1.00/TB | $1.15/TB |
| Hetzner Object Storage | 含 1 TB 儲存 + 1 TB egress；超額儲存 €0.0067/TB-h、egress €1.00/TB | €4.99 | €4.99 | **$5.76** |
| Neon Launch 運算 | 1 CU = 1 vCPU + 4 GiB | $0.106/CU-h | $0.106/CU-h ✔ | 同左 |
| Neon Scale 運算 | — | $0.222/CU-h | $0.222/CU-h ✔ | 同左 |
| Neon 儲存 | — | $0.35/GB-月 | $0.35/GB-月 ✔ | 同左 |

- Hetzner 價格來源：[Hetzner Price Adjustment 15 June 2026（官方）](https://docs.hetzner.com/general/infrastructure-and-availability/price-adjustment/)、[hetzner.com/cloud](https://www.hetzner.com/cloud/)、[hetzner.com/storage/object-storage](https://www.hetzner.com/storage/object-storage/)。價格為德國機房、未稅、不含 IPv4。
- **Neon 三項單價經複核完全正確，且 Launch／Scale 均無月租底價**（[neon.com/pricing](https://neon.com/pricing)）。
- **CCX33 在 v1 記為 €48.49，連調價前的 €62.49 都對不上**——該值錯了兩層。
- **調價後 CX 線全面優於 CPX 線**：CX33 與 CPX32 規格相同（4 vCPU/8 GiB），價格卻是 $9.80 vs $40.95。v2 控制平面一律改用 CX 線。

### 3.C DigitalOcean（**經複核，全部正確**）

| 項目 | 單價 | 備註 |
| --- | --- | --- |
| Managed PostgreSQL 8 GiB / 4 vCPU（單節點） | $122.10/月 | **含 60–120 GiB 儲存**，情境 (a) 的 100 GB 免額外計費 |
| Managed PostgreSQL 16 GiB / 6 vCPU（單節點） | $244.35/月 | **含 290–580 GiB 儲存**，情境 (b) 的 300 GB 免額外計費 |
| Managed PG 額外儲存 | $0.215/GiB-月 | 10 GiB 級距 |
| Spaces 物件儲存 | $5/月含 250 GiB + 1,024 GiB egress；超額 $0.02/GiB-月、egress $0.01/GiB | — |
| Droplet Basic 8 GiB / 4 vCPU | $48/月（含 5 TB 傳輸） | 控制平面 |
| Droplet Basic 2 GiB / 1 vCPU | $12/月（含 2 TB） | Egress Proxy |
| Droplet CPU-Optimized 8 GiB / 4 vCPU | $84/月（含 5 TB） | 2 GiB/vCPU，**不適用 Sandbox** |
| Droplet CPU-Optimized 16 GiB / 8 vCPU | $168/月（含 6 TB） | 同上；E2 worker 可用 |
| **Droplet General Purpose 16 GiB / 4 vCPU** | **$136/月** | 4 GiB/vCPU 專用 vCPU，**v2 Sandbox (a) 用** |
| **Droplet General Purpose 32 GiB / 8 vCPU** | **$272/月** | 同上，**v2 Sandbox (b) 用** |
| Load Balancer | $12/月 | — |
| **DOKS 受管 Kubernetes 控制平面** | **$0**（標準）／$40（HA 控制平面） | 只付 worker 節點 |
| Block Storage Volume | $0.10/GiB-月 | ⚠️ 牌價，本次未逐項查證，見 §9 |

來源：[digitalocean.com/pricing/managed-databases](https://www.digitalocean.com/pricing/managed-databases)、[digitalocean.com/pricing/droplets](https://www.digitalocean.com/pricing/droplets)。DO 自 2026-01-01 起改為**每秒計費**（最低 60 秒），對短命 Sandbox 節點有利。

### 3.A AWS（**抽查通過**）

| 項目 | 單價 | 備註 |
| --- | --- | --- |
| RDS db.m7g.large (2 vCPU/8 GiB) | $0.168/h | — |
| RDS db.m7g.xlarge (4 vCPU/16 GiB) | $0.336/h | 線性倍數 |
| RDS gp3 儲存 | $0.115/GB-月 | — |
| S3 Standard | $0.023/GB-月（首 50 TB） | — |
| EC2 c7g.xlarge (4 vCPU/8 GiB) | $0.145/h ≈ $105.85/月 | 控制平面 |
| **EC2 m7g.xlarge (4 vCPU/16 GiB)** | $0.163/h ≈ **$119.14/月** | v2 Sandbox (a) |
| **EC2 m7g.2xlarge (8 vCPU/32 GiB)** | **$0.326/h ≈ $237.98/月** | v2 Sandbox (b)，**已複核** |
| EC2 t4g.small (2 vCPU/2 GiB) | $0.0168/h ≈ $12.26/月 | Egress Proxy（v1 用 $12 但未列 SKU） |
| Data Transfer Out | $0.09/GB，**每月首 100 GB 免費** | **已複核** |

抽查：[instances.vantage.sh/aws/ec2/m7g.2xlarge](https://instances.vantage.sh/aws/ec2/m7g.2xlarge) 確認 m7g.2xlarge = $0.326/h、8 vCPU / 32 GiB；[aws.amazon.com/s3/pricing](https://aws.amazon.com/s3/pricing/) 確認首 100 GB egress 免費。

### 3.B GCP（**抽查通過**）

| 項目 | 單價 | 備註 |
| --- | --- | --- |
| Cloud SQL PostgreSQL Enterprise vCPU | $0.0413/vCPU-h | — |
| Cloud SQL 記憶體 | $0.007/GB-h | — |
| Cloud SQL SSD 儲存 | $0.222/GB-月 | — |
| Cloud Storage Standard | $0.020/GB-月 | — |
| Compute Engine e2-standard-4 (4 vCPU/16 GiB) | **$97.84/月**（**已複核**，us-central1） | 控制平面 |
| Compute Engine e2-small | ≈ $12/月 | Egress Proxy |
| **Compute Engine n2-standard-4 (4 vCPU/16 GiB)** | **$141.79/月** | v2 Sandbox (a)，專用 vCPU |
| **Compute Engine n2-standard-8 (8 vCPU/32 GiB)** | **$283.58/月** | v2 Sandbox (b)，專用 vCPU |
| 網際網路 egress（Premium Tier） | $0.12/GB（首 1 TB） | — |

### 3.共用項

| 項目 | 單價 |
| --- | --- |
| LiteLLM Proxy（OSS, MIT） | **$0** 授權費，僅計節點 |
| Langfuse Cloud Core | **$29/月**，含 100k units |
| Langfuse units 超額 | $8/100k units（1M 以上降至 $7） |
| CloudNativePG operator | $0（Apache-2.0） |
| PostgreSQL + pgvector 容器映像 | $0 |

---

## 4. 試算結果

Langfuse 計費：(a) 45,000 units → Core **$29**；(b) 450,000 units → $29 + 350k × $8/100k = **$57**；(b′) 180,000 units → $29 + 80k × $8/100k = **$35**。

### 4.1 情境 (a)：封閉測試（20 使用者，50 Run/日，1,500 Run/月）

| 分項 | **E1 Hetzner** | **E1 DO** | **E2 DOKS** | D. Hetzner+Neon | C. DO 受管 | A. AWS | B. GCP |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PostgreSQL | 內含於資料節點 | 內含於資料節點 | $20（PVC） | $190 | $122 | $134 | $123 |
| 物件儲存（150 GB，含備份） | $6 | $5 | $5 | $6 | $5 | $3 | $3 |
| 資料節點／K8s worker | $19 | $136 | $168 | — | — | — | — |
| 控制平面節點 | $19 | $48 | 同上 | $31 | $144 | $325 | $302 |
| Load Balancer | — | — | $12 | — | — | — | — |
| Egress Proxy（1 台） | $10 | $12 | $12 | $10 | $12 | $12 | $12 |
| **Sandbox VM 池（2 × 4 vCPU/16 GiB）** | **$200** | **$272** | **$272** | **$200** | **$272** | **$238** | **$284** |
| Egress（50 GB） | $0 | $0 | $0 | $0 | $0 | $0 | $6 |
| Langfuse Cloud | $29 | $29 | $29 | $29 | $29 | $29 | $29 |
| **月費合計** | **$283** | **$502** | **$518** | **$466** | **$584** | **$741** | **$759** |
| 平均單 Run 平台成本 | **$0.19** | $0.33 | $0.35 | $0.31 | $0.39 | $0.49 | $0.51 |

計算註記：

- **E1 Hetzner**：資料節點 CX43（8 vCPU/16 GiB，$18.45+IPv4 $0.58）跑 Postgres+pgvector 容器與 LiteLLM；控制平面節點 CX43 跑 Go API/Worker/Python LLM（容器化後 3 個部署單元共節點，取代 v1 的 3 台獨立 VM）；Egress Proxy CX33 $10.38；Sandbox 2 × CCX23（$99.21+$0.58）= $199.58。
- **E1 DO**：資料節點用 General Purpose 16 GiB/4 vCPU $136（Postgres 需專用 vCPU）；控制平面 Basic 8 GiB/4 vCPU $48。
- **E2 DOKS**：控制平面 $0；worker 2 × CPU-Optimized 8 GiB/4 vCPU $84 = $168；CNPG PVC 2×100 GiB × $0.10 = $20（primary + standby）。
- **D. Hetzner+Neon**：Neon **2 CU**（= 2 vCPU/8 GiB，對齊 §2.2 規格）× 730h × $0.106 = $154.76 + 100 GB × $0.35 = $35 → $190。控制平面 3 × CX33 = $31.14。
- **C. DO**：100 GB 落在 Managed PG 8 GiB/4 vCPU 的 60–120 GiB 含量內，不另計儲存。
- **A. AWS**：控制平面 3 × c7g.xlarge（4 vCPU/8 GiB，對齊 §2.2 規格）$317.55 + EBS ≈ $325（v1 誤用 2 vCPU 的 m7g.large）。
- **B. GCP**：控制平面 3 × e2-standard-4 $293.52 + PD ≈ $302（v1 同樣誤用 2 vCPU 的 e2-standard-2）。

> **配置修訂註記（2026-08-16，[ADR-022](../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 強制條件 6）——總額不變，原文不改寫。**
>
> 本目錄文件自 2026-08-14 起視為已定案依據、不再修訂結論（[README.md](README.md)），故上方計算註記維持原文；以下為後續 ADR 對**節點配置**（非金額）的修訂，適用於 §4.1 與 §4.2 的 **E1 Hetzner** 兩列。
>
> - **原配置**：資料節點跑「Postgres + pgvector 容器與 LiteLLM」；另編列「Egress Proxy 1 台 CX33 $10.38」（(b) 為 2 台 HA $21）。
> - **修訂後**：**LiteLLM 移出資料節點，改跑在沙箱面專屬節點上**——即原本編列給 Egress Proxy 的那台 CX33（(b) 為 2 台）。ADR-022 Q3 決定不部署 Squid／Envoy（沙箱層允許清單收斂為一項平台自有目的地），該預算行因此空出來，正好承接這個需求。
> - **為什麼必須移出**：ADR-022 Q2 強制條件 6——沙箱層允許清單的目的地**不得是控制平面／資料節點的位址**。LiteLLM 若留在資料節點上，沙箱「唯一到得了的位址」就是那台跑著 PostgreSQL 的機器，ADR-001 的平面隔離會被一條防火牆規則抵銷（威脅模型 TM-CTL-01）。
> - **金額影響**：**$0**。兩情境的月費合計、單 Run 平台成本與所有比較結論皆不變；改變的只是同一筆錢買到的東西從「Egress Proxy」變成「沙箱面 LiteLLM 節點」。
> - 驗證：ADR-022 SEC-009 測項 **T5-7**（對允許清單 `pinned_ip` 的 5432／平台 API／9000 必須 DROP）是這項配置被違反時唯一會叫的檢查。

### 4.2 情境 (b)：早期成長（200 使用者，500 Run/日，15,000 Run/月）

| 分項 | **E1 Hetzner** | **E2 Hetzner（自管 k8s）** | **E1 DO** | **E2 DOKS** | D. Hetzner+Neon | C. DO 受管 | A. AWS | B. GCP |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PostgreSQL | 內含 | 內含（+Volume 待確認） | 內含 | $60（PVC） | $380 | $244 | $280 | $269 |
| 物件儲存（1 TB + 備份 300 GB） | $8 | $8 | $27 | $27 | $6 | $20 | $24 | $20 |
| 資料節點／K8s worker | $35 | $76 | $272 | $672 | — | — | — | — |
| K8s 控制平面 | — | $31（自管 3 台） | — | $0（DOKS） | — | — | — | — |
| 控制平面節點 | $38 | 同 worker | $96 | 同 worker | $48 | $204 | $452 | $421 |
| Load Balancer | $6 | $6 | $12 | $12 | 內含 | 內含 | 內含 | 內含 |
| Egress Proxy（2 台 HA） | $21 | $21 | $24 | $24 | $21 | $24 | $25 | $25 |
| **Sandbox VM 池（5 × 8 vCPU/32 GiB）** | **$802** | **$802** | **$1,360** | **$1,360** | **$802** | **$1,360** | **$1,190** | **$1,418** |
| Egress（400 GB） | $0 | $0 | $0 | $0 | $0 | $0 | $27 | $48 |
| Langfuse Cloud | $57 | $57 | $57 | $57 | $57 | $57 | $57 | $57 |
| **月費合計** | **$967** | **$1,001＋** | **$1,848** | **$2,212** | **$1,314** | **$1,909** | **$2,055** | **$2,258** |
| 平均單 Run 平台成本 | **$0.064** | $0.067 | $0.123 | $0.147 | $0.088 | $0.127 | $0.137 | $0.151 |

計算註記：

- **Sandbox 池是所有組合的最大單項**（E1 Hetzner **83%**、E1 DO 74%、C. DO 71%、A. AWS 58%）。**容器化完全不觸及這一項**——它省的是控制平面與資料層，不是 Sandbox。
- **N+1 已套用**：13 slot ÷ 4 slot/節點 = ceil(3.25) = 4 台，+1 = **5 台**。v1 只列 4 台，等於未套 N+1（情境 (a) 卻有套），四個平台一致低估 25%。
- **E1 Hetzner**：資料節點 CX53（16 vCPU/32 GiB）$34.60 跑 Postgres 4 vCPU/16 GiB 容器 + LiteLLM；控制平面 2 × CX43 = $38.06（16 vCPU/32 GiB，對齊 §2.2 的 4 × 4 vCPU/8 GiB）；Sandbox 5 × CCX33（$159.77+$0.58）= $801.80。
- **E2 Hetzner**：Hetzner **無受管 Kubernetes**，控制平面須自管（3 × CX33 = $31）；worker 4 × CX43 = $76.12（32 vCPU/64 GiB）。CNPG 的 PVC 需 Hetzner Volume（600 GB），**單價本次未複核，未計入**——故此欄標 `＋`。
- **E2 DOKS**：worker 4 × CPU-Optimized 16 GiB/8 vCPU $168 = $672（32 vCPU/64 GiB，容納 CNPG primary+standby 各 16 GiB 與四個應用單元，餘裕偏緊）；PVC 600 GiB × $0.10 = $60。
- **D. Hetzner+Neon**：Neon **4 CU** × 730h × $0.106 = $309.52 + 200 GB × $0.35 = $70 → $380（假設大型 Payload 已依 ADR-014 下沉物件儲存；若 300 GB 全留 Neon 則 +$35）。
- **A. AWS**：Sandbox 改用 m7g.2xlarge（32 GiB）而非 c7g.2xlarge（16 GiB）——後者在 4 GiB/slot 下裝不下 4 個 slot。
- **B. GCP**：Sandbox 改用 n2-standard-8（專用 vCPU）而非 e2-standard-8（共享租戶）。這使 GCP 由 v1 的第二便宜變成最貴，但這是把「Sandbox 需專用 vCPU」的要求一致施加於所有平台的結果。

### 4.3 三個關鍵事實

1. **供應商差異 > 架構差異。** 受管的 D（Hetzner+Neon，$1,314）比容器化的 E1 DO（$1,848）便宜 29%。**節點單價才是主導變數**，不是容器化與否。
2. **容器化省的錢集中在非 Sandbox 部分。** E1 Hetzner (b) 的非 Sandbox 成本 $165，D 的非 Sandbox 成本 $512——省 **68%**（$347）。但因 Sandbox 佔 83%，總帳單只省 **26%**。
3. **E2 在 MVP 規模不省錢。** E2 DOKS ($2,212) 比 C. DO 受管 ($1,909) **貴 $303**；E2 Hetzner ($1,001+) 比 E1 Hetzner ($967) 貴。E2 買的是 **HA（CNPG 自動 failover）與宣告式編排**，不是成本。

### 4.4 PDM-010 定案值參照（30 Run/人/月）

若 PDM-010 照案定案，Run 量降至 (a)/(b) 的 40%：

| 組合 | (a′) 600 Run/月 | (b′) 6,000 Run/月 |
| --- | ---: | ---: |
| **E1 Hetzner** | **$283**（與 (a) 相同） | **$587** |
| D. Hetzner+Neon | $466 | $799 |
| C. DO 受管 | $584 | $1,206 |

- **(a′) 成本與 (a) 完全相同**：1 slot + N+1 = 2 節點，與 (a) 的 2 節點相同——**N+1 下限使封測規模的 Sandbox 成本對 Run 量不敏感**。想在封測階段省錢，只能省節點規格或放棄 N+1，不能靠減少 Run。
- **(b′) E1 Hetzner $587**：5 slot ÷ 4 = 2 台 +1 = **3 台** CCX33 = $481。相對 (b) 省 $380，其中 $321 來自 Sandbox 池縮減。
- **結論**：PDM-010 的 30 Run/人/月上限在 (b) 規模是**每月省約 $380 的成本控制機制**，這強化了 PDM-010 §8.2「把月上限視為成本上限的執行機制，而非體驗設計」的建議。

---

## 5. 自架的營運成本、風險與元件例外

### 5.1 哪些元件容器化、哪些是例外

| 元件 | 決定 | 理由 |
| --- | --- | --- |
| PostgreSQL + pgvector + FTS + River 佇列 + Trace 分割表 | **容器化** | 省最多（(b) 省 $380 vs Neon、$244 vs DO Managed）；同時**消除 Neon–Hetzner 跨供應商延遲風險**（v1 §7 列為「最需要先驗證的一項」，容器化後不存在） |
| LiteLLM Proxy | **容器化** | 本來就是 OSS 自架，v1 已計為節點成本，無變化 |
| Go API／River Worker／Python LLM 服務 | **容器化** | 本來就是自有程式碼；容器化的實質收益是**節點合併**（(a) 由 3 台 VM 併為 1 台，省 $132/月 vs C. DO） |
| **物件儲存** | **例外：維持受管** | 見 §5.2——自架反而更貴，且觸及授權風險 |
| **Langfuse** | **例外：維持 Cloud** | 自架 v3 需 ClickHouse + Redis + S3 三個新資料產品，違反 ADR-014 選型原則 2；$29–57/月遠低於三套備份監控升級的成本 |
| **Sandbox 執行節點** | **例外：獨立 VM + gVisor，不進 K8s** | ADR-015 明訂執行平面為獨立 VM 池、不與應用工作負載混排；把 Sandbox 放進 K8s 會讓 ADR-001 的平面隔離退化為命名空間隔離 |
| PostgreSQL 高可用（CNPG replica） | **E1 不做，E2 才做** | 見 §5.3 風險 |

### 5.2 物件儲存為什麼不容器化（含 MinIO 授權問題）

**成本上自架就輸了。** 情境 (b) 需 1 TB 資料 + 約 300 GB 備份，具備耐久性至少要 2 份副本 = 約 2.6 TB 持久化磁碟：

| 方案 | 情境 (b) 月成本 | 說明 |
| --- | ---: | --- |
| Hetzner Object Storage（受管） | **$8** | €4.99 含 1 TB 儲存 + 1 TB egress；超額 €0.0067/TB-h |
| DO Spaces（受管） | **$27** | $5 含 250 GiB + 1 TiB egress |
| 容器自架於 DO Block Storage | **≈ $266** | 2.6 TB × $0.10/GiB-月 = $266，**尚未含節點與營運工時** |
| 容器自架於節點內含 NVMe | 需 CX53 級節點多台 | 無跨節點耐久性，節點重建即資料遺失——與 ADR-015「節點定期重建」直接衝突 |

**受管物件儲存比自架便宜約 10–30 倍**，因為 S3 類服務賣的是攤提後的容量，而 block storage 賣的是保留容量。這是本次試算最乾脆的一個結論。

**MinIO 的授權與產品風險（即使不採用也應記錄）：**

- **授權**：MinIO 為 **AGPL-3.0**。AGPL 第 13 條的網路 copyleft 條款在「物件儲存以網路服務形式被存取」的情境下，其傳染範圍需法務判斷；即使我們只在內部網路使用、對外僅暴露自家 API，這仍是一個需要明確結論而非默認安全的問題。
- **社群版功能縮減**：2025 年後 MinIO 將開發重心移往商業版 AIStor，社群版的管理主控台功能被大幅精簡，社群版與商業版功能落差擴大且路線圖不明確。把 MVP 的資料耐久性押在一個正在縮減社群投入的專案上，風險不對稱。
- **替代方案**：
  - **SeaweedFS**（**Apache-2.0**）——三者中唯一授權乾淨的選項，S3 gateway 成熟，對大量小物件效率佳。若未來真要自架，這是首選。
  - **Garage**（AGPL-3.0）——社群治理、專為跨節點自架設計，無廠商功能縮減風險，但授權類別與 MinIO 相同。
  - **混合式（本文件建議）**——物件儲存直接用供應商的 S3 相容服務，**完全繞開授權問題**，且成本最低。

### 5.3 自架的風險（MVP 可接受但必須明示）

| 風險 | E1 | E2 | 對策 |
| --- | --- | --- | --- |
| **Postgres 單實例，無自動 failover** | **存在**——節點或容器故障 = 全平台停機至人工還原 | 已緩解（CNPG primary + standby 自動接管） | MVP 可接受：Run 為非同步、無金流、封測使用者數少。**但必須明示**，不可默認為「和受管一樣」 |
| 資料遺失（節點重建、磁碟故障） | pgBackRest／WAL-G 持續 WAL 歸檔至物件儲存 | 同左 + standby | **還原演練必須排入 M0 收尾**，未演練過的備份等於沒有備份（ADR-010 DR 要求） |
| Postgres 主版本升級 | 人工執行，需停機窗口 | CNPG 支援但仍需規劃 | 受管方案此項為 $0 工時 |
| 磁碟水位耗盡 | 無供應商告警，需自建監控 | 同左 | Trace 分割表 `DROP PARTITION`（PDM-006 90 天）是主要控制手段 |
| K8s 版本升級 | 不適用 | DOKS 每季／自管更頻繁 | E2 的持續稅 |
| **Hetzner 濫用政策**（執行不受信任程式碼） | **存在** | **存在** | 與容器化無關，見 §7 |

### 5.4 每月營運小時估計

不換算為金額（費率由負責人決定），但必須可見：

| 工作項 | E1 | E2 | 受管對照組 |
| --- | ---: | ---: | ---: |
| Postgres 備份還原演練 | 2–4 h | 2–4 h | 0.5 h |
| Postgres 小版本升級與重啟窗口（月攤提） | 1–2 h | 1–2 h | 0 h |
| Postgres 主版本升級（年度攤提） | 0.7–1.3 h | 0.7–1.3 h | 0.1 h |
| 節點 OS 修補與重建 | 2–3 h | 1–2 h | 1–2 h（僅 Sandbox 節點） |
| K8s 版本升級（季度攤提） | — | 1.3–2.7 h（DOKS）／2.7–5.3 h（自管） | — |
| 監控告警維護（PG、容器、磁碟水位） | 2–3 h | 2–3 h | 1 h |
| **月度合計（穩態）** | **8–13 h** | **9–16 h** | **2.6–3.6 h** |
| **相對受管對照組增量** | **+5～10 h/月** | **+6～13 h/月** | — |

**與省下的錢對照：**

| 情境 | 容器化省下（vs 最佳受管對照） | 營運工時增量 | 損益平衡的每小時費率 |
| --- | ---: | ---: | ---: |
| (a) 封測 | **$183**（E1 Hetzner $283 vs D $466） | +5～10 h | **$18–37/h** |
| (b) 早期成長 | **$347**（E1 Hetzner $967 vs D $1,314） | +5～10 h | **$35–69/h** |
| (b′) PDM-010 定案值 | **$212**（$587 vs $799） | +5～10 h | **$21–42/h** |

> ⚠️ **誠實結論**：以任何合理的工程師時薪計，**容器化 Postgres 在 MVP 規模不會靠省錢回本**。它的正當理由是**非金錢的**：消除跨供應商延遲風險、不受供應商計費模型變動影響（Hetzner 六月調價、Neon 改價都不會傷到自架的 Postgres）、資料主權、以及與「雲原生」架構方向的一致性。負責人若以成本為由推動容器化，這份數字不支持；若以架構自主與風險控制為由，數字不反對。

---

## 6. 敏感度：真正會咬人的三個變數

### 6.1 三變數

| 變數 | 假設值 | 若翻倍，(b) 情境影響 | 說明 |
| --- | --- | --- | --- |
| **單 Run 佔用時間** | 6 分鐘 | 尖峰 slot 13 → 25，節點 5 → **8 台**。E1 Hetzner Sandbox $802 → **$1,283**（總計 **$1,448**）；C. DO $1,360 → **$2,176** | ADR-015 待決策；PDM-005 的軟上限是 **10 分鐘**，加 provisioning/cleanup 約 12 分鐘——**翻倍即等於「使用者普遍跑到上限」**，不是極端假設。**唯一應優先量測的數字**（NFR-004 已要求量測 Sandbox 建立時間） |
| **Run 量** | 75 Run/人/月 | 若照 PDM-010 的 30 Run/人/月，(b) 降至 (b′)：E1 Hetzner $967 → **$587**（−39%） | 見 §4.4。這是唯一可由產品政策直接控制的變數 |
| **模型 token 費用** | **未計入平台比較** | 見 §6.2（**v3 重估見 §6.2.3**） | PDM-003 模型選擇對總成本的影響大於平台選擇。**v3 修正**：供應商定案為 OpenAI ＋ mini 級預設後，模型費由「所有情境的多數項」降為中位用量下的 21–44%，**平台選擇的槓桿回升**；真正的數倍級變數收斂為「模型分層是否被遵守」 |

物件保存期（90 天）與 egress 的敏感度極低（各組合 ±$8～$48），不列為關鍵變數。

### 6.2 模型費用：平台成本其實是配角

v1 此節假設「每 Run 平均 $0.05 token 費」。**該值與本專案自己的估算不符**——[PDM-010](pdm-proposals.md) §8.2 明列單 Run 模型成本**中位數 ≈ $0.30**、上限 ≈ $1.80（`claude-sonnet-5`，300K in / 60K out）。v1 低估 **6 倍**。v2 改用 $0.30：

| 情境 | Run/月 | 模型費（$0.30/Run） | E1 Hetzner 平台費 | 總計 | **模型費佔比** |
| --- | ---: | ---: | ---: | ---: | ---: |
| (a) | 1,500 | **$450** | $283 | $733 | **61%** |
| (b) | 15,000 | **$4,500** | $967 | $5,467 | **82%** |
| (b′) PDM-010 | 6,000 | **$1,800** | $587 | $2,387 | **75%** |
| (b) 最壞（上限 $1.80/Run） | 15,000 | **$27,000** | $967 | $27,967 | **97%** |

**重寫後的結論：**

1. **模型費用在所有情境都是總帳單的多數項（61%～97%）。** v1 說「一旦模型費用納入，AWS 與 Hetzner 的差距從 158% 壓縮到 60–70%」——用正確的 $0.30 重算，差距壓縮到 **約 20%**（AWS $2,055+$4,500 = $6,555 vs E1 Hetzner $967+$4,500 = $5,467）。**平台選型在總成本上的槓桿遠比 v1 呈現的小。**
2. **容器化省下的 $347/月，只佔 (b) 總帳單的 6%。** 而營運工時增量（§5.4）幾乎抵銷它。**在模型費用面前，基礎設施的架構辯論是次要問題。**
3. **真正決定成本可持續性的是三件事，全部是產品決策不是平台決策**：
   - **PDM-010 的免費額度上限**——30 vs 75 Run/人/月，直接決定 (b) 是 $2,387 還是 $5,467。
   - **PDM-005 的 Token 預算**（每 Run ≤ 300K in / 60K out，由 LiteLLM Virtual Key 強制）——這是唯一能硬性止血的機制，中位數與上限差 6 倍，**必須在 M1 用 20–30 次真實 Run 量測收窄**（PDM-010 §風險已要求）。
   - **PDM-003 的模型分層**——搜尋路徑用 haiku、試跑用 sonnet 的分層若失效（例如全用 opus-5），總帳單直接翻倍。
4. **對負責人的實務建議**：把預算決策的注意力放在 PDM-003／005／010，而非平台選型。平台選對可以省下每月數百美元；模型政策選錯可以多花每月數千美元。

### 6.2.3 v3 重估（2026-08-14）：供應商定案為 OpenAI ＋ mini 級預設 ＋ 87% 快取折扣

**觸發**：負責人於 2026-08-14 定案**模型供應商採 OpenAI API**（經 LiteLLM 閘道，ADR-017 架構不變），且 [pdm-proposals.md](pdm-proposals.md) §3 依實測把試跑預設定為 **`gpt-5.4-mini`**（$0.75 / $4.50 / 快取輸入 $0.075，每 MTok）。[pdm-003-litellm-spike-report.md](pdm-003-litellm-spike-report.md) §11.5 同時實測出 **87% 的實效快取折扣**——harness 前綴約 19.4K token 跨輪、**且跨 Run** 完全命中快取（OpenAI 前綴快取預設保留 24 小時）。**§6.2 上方的 v2 數字全部基於 `claude-sonnet-5`，已失效，以下取代之。**

**每 Run token 費重估**（沿用 v2 的「中位＝上限的 1/6」假設，只換單價；上限＝300K in / 60K out）：

| 檔位 | 每 Run 上限（熱快取） | 每 Run 上限（冷快取） | **每 Run 中位**（50K in / 10K out，熱快取） |
| --- | ---: | ---: | ---: |
| **`gpt-5.4-mini`（定案預設）** | **$0.30** | $0.50 | **$0.05** |
| `gpt-5.6-sol`（進階，選用） | $2.00 | $3.30 | $0.33 |
| （v2 舊值）`claude-sonnet-5` | $1.80 | — | $0.30 |

> **$0.30 的組成已翻轉**：300K 快取後 input 僅約 $0.03，60K output 為 $0.27——**output 佔上限成本的 90%**。v2 以 input:output ≈ 1:1 編列的直覺在此不再成立。

**各情境重估（模型費以 $0.05/Run 計，平台費為 E1 Hetzner）：**

| 情境 | Run/月 | 模型費 | E1 Hetzner 平台費 | 總計 | **模型費佔比** | （v2 舊佔比） |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| (a) | 1,500 | **$75** | $283 | **$358** | **21%** | 61% |
| **(b)** | **15,000** | **$750** | **$967** | **$1,717** | **44%** | 82% |
| (b′) PDM-010 | 6,000 | **$300** | $587 | **$887** | **34%** | 75% |
| (b) 最壞（上限 $0.30/Run） | 15,000 | **$4,500** | $967 | **$5,467** | **82%** | 97% |

**重寫後的結論（三點與 v2 相反或顯著改變）：**

1. **模型費在中位用量下已不是總帳單的多數項**——由 v2 的「所有情境 61%～97%」降為 **21%～44%**。**只有在使用者普遍打滿 token 上限時它才回到多數項（82%）。** v2 那句「在模型費用面前，基礎設施的架構辯論是次要問題」**在中位用量下不再成立**。
2. **平台選型的槓桿回升，而且回升得很明顯。** v2 說「AWS 與 Hetzner 的差距被模型費壓縮到約 20%」——用 $0.05/Run 重算，差距**回到 63%**（AWS $2,055 + $750 = **$2,805** vs E1 Hetzner $967 + $750 = **$1,717**）。**§7 的節點供應商推薦（Hetzner 主推、DO 退路）因此比 v2 時期更值得認真對待，不是次要問題。** 同理，容器化省下的 $347/月由佔 (b) 總帳單的 6% 升為 **20%**——但 §5.4 的誠實結論不變：以任何合理時薪計，它仍不靠省錢回本。
3. **「真正決定成本可持續性的三件事」順序改變。** 三項仍全部是產品決策，但權重不同了：
   - **PDM-003 的模型分層——升為第一位。** 分層失效（試跑全用旗艦 `gpt-5.6-sol`）會讓 (b) 模型費由 $750 變成 **$4,950**，總帳單 **$1,717 → $5,917，約 3.4 倍**。這是三者中唯一能單獨造成數倍差距的。v2 此項排第三。
   - **PDM-005 的 Token 預算**——仍是唯一能硬性止血的機制，中位與上限仍差 6 倍（$887 vs $5,467 於 (b′)／(b) 最壞）。**但 v5 有一項機制變更必須知道**：`max_budget`（金額）在 87% 折扣下**與 token 上限脫鉤約 7–8 倍**，已不能代理 token 上限；token 級強制改由 Go Worker 依 `input_tokens` 累計（pdm-proposals §5.2a）。**成本煞車與 token 煞車現在是兩件事，別當成一件。**
   - **PDM-010 的免費額度上限**——30 vs 75 Run/人/月，(b) 的差別由 v2 的 $2,387 vs $5,467 縮小為 **$887 vs $1,717**。仍然重要，但**不再是三者中最大的那一項**。
4. **對負責人的實務建議（改寫）**：v2 說「把注意力放在模型政策而非平台選型」——**v3 修正為：模型政策的注意力應集中在「分層是否被遵守」這一件事上**（它是唯一的數倍級變數），其餘的預算注意力可以合理地回到平台選型與 Sandbox 佔用時間（§6.1 仍是敏感度最高的單一變數，翻倍即 Sandbox 池 +60%）。

> ⚠️ **兩項未變的前提，不要因為數字變小而放鬆**：(i) **$0.05 中位數仍是假設值（上限的 1/6），不是量測值**——換供應商只換掉單價，沒有換掉這個假設，§8 的「M1 以 20–30 次真實 Run 校準」要求不變，且量測時須同時記錄每輪工具呼叫次數（input 用量幾乎完全由它決定）；(ii) **首次 Run 與後續 Run 的單位成本差約 8 倍**（冷／熱快取），成本模型不應只用單一均值，`02:TEST-005` 的預估成本必須顯示為區間。

---

## 7. 推薦

**前提**：雲原生容器化是負責人 2026-08-13 指示的既定架構方向。以下不重新辯論該方向，只回答「怎麼做最划算」與「哪裡該例外」。

### 7.1 節點供應商：Hetzner Cloud（主推），DigitalOcean（風險退路）

即使經 2026-06-15 調價（CCX 線漲 113–173%），Hetzner 在兩個情境仍是最低成本：

| | E1 Hetzner | E1 DO | 差額 |
| --- | ---: | ---: | ---: |
| (a) | **$283** | $502 | +$219（+77%） |
| (b) | **$967** | $1,848 | +$881（+91%） |

**⚠️ v1 的「3.8 倍 vCPU 價差」論證已刪除。** v1 稱「Hetzner CCX33 $56 vs AWS c7g.2xlarge $212 是 3.8 倍價差」——用正確價格（CCX33 $159.77）與正確機型（AWS 須用 32 GiB 的 m7g.2xlarge $237.98）重算，**實際是 1.49 倍**；對 DO General Purpose（$272）是 1.70 倍。Hetzner 仍然最便宜，但**「便宜到不需要思考」的那個論證不成立了**，它現在是一個需要與風險權衡的普通成本優勢。

**Hetzner 的兩個未解風險（容器化不但沒消除，其中一個還加重）：**

| 風險 | 嚴重度 | 說明與對策 |
| --- | --- | --- |
| **濫用政策**：Sandbox 執行不受信任程式碼，若被用於挖礦、掃描或濫用流量，Hetzner 處置偏向直接停機而非協商 | **高——選 Hetzner 必須付出的營運工作** | ADR-015 的 default-deny Egress Proxy 是主要對策；另需**事前與 Hetzner 溝通用途**、建立濫用偵測與自動封停 Run 的流程（對應 SEC 需求）。這一項與容器化無關，且是 Hetzner 相對 DO 的唯一實質劣勢 |
| **無受管 Kubernetes** | **中——只影響 E2** | E1 不受影響（compose／k3s 單機）。E2 在 Hetzner 需自管 K8s 控制平面（§5.4 多 1.3–2.7 h/月），或改用第三方 Cluster API 供應商。**若確定會走到 E2，DO 的 DOKS 免費控制平面會縮小兩家的差距** |
| ~~Neon–Hetzner 跨供應商延遲~~ | **已消除** | Postgres 容器化後與應用同節點／同內網，v1 列為「最需要先驗證的一項」的風險不再存在。**這是容器化最實在的一個收益** |

**選 DO 的條件**：若負責人不接受 Hetzner 的濫用政策風險，DO 是完整的退路——單一供應商、DOKS 控制平面免費、Sandbox 與資料同區內網、每秒計費對短命節點有利。代價是 (b) 每月多 **$881**。

### 7.2 部署形態：E1 起步，壓力觸發才進 E2

**MVP 採 E1**：docker compose 或 k3s 單機，控制平面全容器化。理由是 ADR-010 的「依壓力拆分，不預先建設」原則同樣適用於編排層——E2 在 MVP 規模**不省錢**（§4.3 事實 3），只增加 K8s 升級稅。

**E1 → E2 切換觸發（任一成立即評估，全部未成立就留在 E1）：**

1. **Postgres 單實例的計畫外停機已影響 Run 完成率**——這是 E2 唯一能買到而 E1 買不到的東西（CNPG 自動 failover）。這也是最可能的觸發項。
2. **控制平面節點數 > 4**，手動編排的部署、滾動更新與設定漂移成本超過 K8s 的學習與升級成本。
3. **ADR-010 服務拆分觸發表已觸發任一列**（Run Orchestrator、Ingestion Worker、Catalog & Search、Evaluation Worker 需獨立擴展或獨立部署）。
4. **團隊具備至少 1 名可負責 K8s 版本升級的成員**——這是**必要條件**，不是加分項。前三項成立但此項不成立時，正確的動作是先做 Postgres 備援（E1 + 手動 standby），不是上 K8s。

切換時的平台選擇：**若走 E2，DO DOKS（控制平面 $0）相對 Hetzner 自管的優勢會顯現**，但 (b) 的節點價差仍達 $881/月——屆時應重新試算，而非沿用本文件結論。

### 7.3 元件例外（不容器化）

1. **物件儲存 → 用供應商的 S3 相容服務**（Hetzner Object Storage $8／DO Spaces $27）。自架需約 2.6 TB block storage ≈ $266/月，**貴 10–30 倍**，且觸及 MinIO 的 AGPL 授權與社群版功能縮減問題。若未來確需自架，選 **SeaweedFS（Apache-2.0）**，不選 MinIO。詳見 §5.2。
2. **Langfuse → 維持 Cloud**（$29–57/月）。自架 v3 需引入 ClickHouse + Redis + S3 三個新資料產品。
3. **Sandbox 執行節點 → 獨立 VM 池 + gVisor，不進 Kubernetes**（ADR-015 不變）。這佔總平台成本的 **83%**，也是唯一真正值得優化的一項——但優化手段是**縮短單 Run 佔用時間**（§6.1）與**控制 Run 量**（PDM-010），不是換編排技術。

### 7.4 給負責人的決策問題

1. **接受 Hetzner 的濫用政策風險嗎？**（含事前溝通與偵測流程的工作量）若不接受，退路是 DO，(b) 每月多 $881。
2. **容器化 Postgres 的正當理由是什麼？** §5.4 顯示它**不靠省錢回本**（損益平衡費率僅 $18–69/h）。若理由是架構自主與消除跨供應商延遲，數字不反對；若理由是省錢，需重新檢視。
3. **PDM-010 的免費額度定 30 還是 75 Run/人/月？** 這一項對總成本的影響（$2,387 vs $5,467）**大於本文件所有平台與架構選擇的總和**。
4. **ADR 怎麼處理？** 本方向與 ADR-014（Proposed）的「受管服務優先」原則衝突（僅 PostgreSQL 一列）。修訂仍為 Proposed 的 ADR-014，或另立 ADR-018 取代？

---

## 8. 下一步

本文件解除 ADR-014 與 ADR-015 定案條件中的**成本試算**部分。定案前仍待：

- **ADR-014**：部署平台確認（＝ §7.4 決策問題 1）；**容器化方向的 ADR 處理方式**（＝決策問題 4）。
- **ADR-015**：PDM-003／004 Runtime 確認、部署平台支援驗證（在選定平台上實跑 runsc）、逃逸測試通過（SEC-009、SBX-010）。
- **§2.1 假設校準**（首次真實 Run 後），優先序：
  1. **單 Run 佔用時間**（NFR-004 量測項）——敏感度最高，翻倍即 Sandbox 池 +60%。
  2. **單 Run 模型 token 用量**——PDM-010 已要求以 20–30 次真實 Run 收窄中位數／上限的 6 倍差距。
  3. Sandbox slot 實際記憶體使用（確認 4 GiB 上限是否過寬）。
- **M0 收尾必做**：Postgres 備份**還原演練**（不是備份設定，是還原），這是 E1 單實例架構可被接受的前提。
- 若採 Hetzner：與 Hetzner 事前溝通「執行使用者提交程式碼」的用途與防濫用措施。
- 待補價格：**Hetzner Cloud Volume 單價**（E2 Hetzner 的 CNPG PVC 成本）、**DO Block Storage 單價**逐項查證。

---

## 9. v2 修正紀錄（2026-08-13）

### 9.1 複核發現的錯誤（v1 → v2）

| # | 位置 | v1 | v2 | 原因 |
| --- | --- | --- | --- | --- |
| 1 | §3.D Hetzner CCX23 | €31.49 = $36.34 | **€85.99 = $99.21** | **價格過時**：v1 引用談 2026-04 調價的第三方部落格，漏掉 **2026-06-15 官方調價**（CCX 線 +113~173%）。來源已改引官方調價公告頁 |
| 2 | §3.D Hetzner CCX33 | €48.49 = $55.96 | **€138.49 = $159.77** | 同上。且 v1 的 €48.49 **連調價前的 €62.49 都不符**，錯了兩層 |
| 3 | §3.D Hetzner CPX32 | €13.49 = $15.57 | **€35.49 = $40.95** | 同上（調價前實為 €13.99）。調價後 CX 線全面優於 CPX，控制平面已改用 CX33／CX43 |
| 4 | §3.D Hetzner CX33 | €6.49 = $7.49 | **€8.49 = $9.80** | 同上 |
| 5 | §2.2 情境 (b) Sandbox | 4 節點 | **5 節點** | **N+1 遺漏**：13 slot ÷ 4 slot/節點 = 4 台容量 + 1 = 5 台。情境 (a) 有套 N+1，(b) 沒有。四個平台一致低估 25% |
| 6 | §2.1 slot 記憶體 | 2 vCPU / **2 GiB** | 2 vCPU / **4 GiB** | **規格對齊**：PDM-005 §5.2 的 Run 記憶體上限是 4 GiB。連帶更換機型系列（AWS c7g→m7g、DO CPU-Optimized→General Purpose、GCP e2→n2），見 §2.3 |
| 7 | §4／§5 Neon CU | 1 CU (a)／2 CU (b) | **2 CU (a)／4 CU (b)** | **規格對齊**：§2.2 指定 2 vCPU/8 GiB 與 4 vCPU/16 GiB；1 CU = 1 vCPU + 4 GiB。v1 以「平均 CU」計，等於用一半規格與固定計費平台比較，非同基準 |
| 8 | §4／§5 AWS、GCP 控制平面 | m7g.large／e2-standard-2（**2 vCPU**） | c7g.xlarge／e2-standard-4（**4 vCPU**） | **規格對齊**：§2.2 指定 4 vCPU/8 GiB。v1 只對 AWS/GCP 用一半規格，DO 與 Hetzner 用足規格，計價不一致（此修正使 AWS/GCP 變貴） |
| 9 | §2.2 Run 量 | 未註明 | **加註與 PDM-010 落差 + (a′)(b′) 參照情境** | 兩情境隱含 75 Run/人/月，PDM-010 建議上限 30。新增 §4.4 提供 40% Run 量的數字 |
| 10 | §6.2 模型成本 | $0.05/Run | **$0.30/Run** | PDM-010 §8.2 自訂中位數為 $0.30，v1 低估 6 倍。該節結論已重寫（§6.2） |
| 11 | §7 推薦理由 | 「CCX33 vs c7g.2xlarge 是 **3.8 倍**價差」 | **已刪除** | 用正確價格與正確機型重算為 **1.49 倍**。此論證已失效，不可保留 |
| 12 | §3.B GCP Sandbox | e2-standard（共享租戶） | **n2-standard（專用 vCPU）** | v1 只對 DO 施加「Sandbox 需專用 vCPU」要求，對 GCP 未施加。一致化後 GCP 由第二便宜變最貴 |
| 13 | §3.A AWS Egress Proxy | $12（無對應 SKU） | $12（**標明 t4g.small**） | v1 的 $12 在 AWS 單價表中無來源，疑似沿用 DO Droplet 價 |

### 9.2 保留原樣的低嚴重度項（附原因）

| 項目 | v1 狀況 | 處理 |
| --- | --- | --- |
| **物件儲存分項的未說明加成** | v1 (a) AWS/GCP 皆記 $5（實為 $3.45／$3.00）；(b) AWS $31（實 $23.55）、GCP $26（實 $20.48）、Hetzner $11（1 TB 正好在含量內，實 $5.76） | **已改為按單價實算**（§4.1、§4.2）。單項差額 $2–8，對排序無影響，但既然是可精確計算的值就不留加成 |
| **Egress Proxy 單價在兩情境間不一致** | v1 AWS/GCP 由 (a) $12/台變成 (b) $24.5/台，無說明；DO／Hetzner 則線性 | **已改為線性**（AWS 2 × t4g.small = $25、GCP 2 × e2-small = $25）。原差額約 $25/月，佔 (b) 總額 1%，**影響可忽略**，修正只為內部一致性 |
| **§6.2「壓縮到約 60–70%」** | 依 v1 自身假設實為 70.5% | 該節已整節重寫，此表述不再存在 |
| **Hetzner Cloud Volume 單價** | v1 未使用 | v2 的 E2 Hetzner 需要，但**本次未複核到可信來源**，故 §4.2 該欄標 `＋` 並列入 §8 待補，**不以未經查證的數字入表** |
| **DO Block Storage $0.10/GiB-月** | v1 未使用 | v2 的 E2 DOKS 使用，為 DO 牌價但本次未逐項查證，已於 §3.C 標註 |

### 9.3 方向變更（負責人 2026-08-13 指示）

**指示內容**：Postgres、物件儲存（MinIO 類）這些服務應以容器替代受管服務，走雲原生架構概念。

**本文件據此的變更**：

1. 新增 **E1（單／少節點 compose 或 k3s）** 與 **E2（受管 K8s + CloudNativePG）** 兩個容器化組合為主要候選，各在 Hetzner 與 DO 上試算（§4.1、§4.2）。
2. 受管服務組合（A/B/C/D）**降為對照組**，但仍修正到正確數字，用途是量化容器化的實際節省。
3. 推薦框架改寫為「容器化是既定方向，試算回答**跑在哪家**與**哪些元件不該容器化**」（§0、§7）。
4. **物件儲存經試算後列為例外，維持受管**——自架貴 10–30 倍，且 MinIO 的 AGPL 授權與 2025 年後社群版功能縮減構成不必要的風險。已列 SeaweedFS（Apache-2.0）／Garage 作為替代（§5.2）。**指示提到 MinIO，但數字不支持自架物件儲存，照實記錄。**
5. **Sandbox 執行節點維持獨立 VM 池、不進 K8s**（ADR-015 的執行平面隔離不變）。
6. 新增 §5.3 自架風險（**E1 為 Postgres 單實例，無自動 failover**）與 §5.4 每月營運小時估計。**§5.4 的誠實結論是：容器化在 MVP 規模不靠省錢回本，損益平衡費率僅 $18–69/h。**
7. **與 ADR-014 的衝突已於 §0 標明**，並列為 §7.4 決策問題 4：修訂仍為 Proposed 的 ADR-014，或另立 ADR-018 取代。**本文件未修改任何 ADR，亦未變更任何工作項目勾選狀態。**

### 9.4 v3 增補（2026-08-14）

| # | 位置 | v2 | v3 | 原因 |
| --- | --- | --- | --- | --- |
| 14 | §6.2（新增 §6.2.3）、§6.1 三變數表 | 模型費 **$0.30/Run**（`claude-sonnet-5`），佔總帳單 61%～97%；「AWS 與 Hetzner 差距壓縮到約 20%」 | 模型費 **$0.05/Run**（`gpt-5.4-mini`，含 87% 實效快取折扣），佔比降為 **21%～44%**（打滿上限時 82%）；**AWS 與 Hetzner 差距回到 63%** | **負責人 2026-08-14 定案模型供應商採 OpenAI API**（經 LiteLLM 閘道，ADR-017 不變），試跑預設依實測定為 mini 級。**平台選型的槓桿因此回升，v2「基礎設施辯論是次要問題」的結論在中位用量下不再成立**；三大成本變數的順序改為模型分層 → Token 預算 → 免費額度 |

> **未變更**：§1～§5、§7、§8 的平台試算、推薦與定案前置全部維持原樣——模型費用自始即列為 §2.4 的明確排除項，不進入平台比較，因此本次重估不影響任何平台排序或推薦。

---

## 附錄：價格來源清單（查核日期 2026-08-13）

**本次複核實際查證的來源（v2 新增或確認）：**

- **Hetzner 官方調價公告**：https://docs.hetzner.com/general/infrastructure-and-availability/price-adjustment/ （2026-06-15 生效，CCX/CPX/CX 新舊價對照）
- Hetzner Cloud：https://www.hetzner.com/cloud/ ・ https://www.hetzner.com/cloud/general-purpose/
- Hetzner Object Storage：https://www.hetzner.com/storage/object-storage/ （€4.99 含 1 TB + 1 TB egress，**確認未變**）
- Neon：https://neon.com/pricing （$0.106／$0.222 CU-h、$0.35/GB-月、**確認無月租底價**）
- DigitalOcean Managed Databases：https://www.digitalocean.com/pricing/managed-databases （**含儲存量範圍已確認**）
- DigitalOcean Droplets：https://www.digitalocean.com/pricing/droplets
- AWS EC2（m7g.2xlarge $0.326/h）：https://instances.vantage.sh/aws/ec2/m7g.2xlarge
- AWS S3 與 egress：https://aws.amazon.com/s3/pricing/ （**首 100 GB egress 免費已確認**）
- GCP Compute Engine：https://www.economize.cloud/resources/gcp/pricing/compute-engine/e2-standard-4/ ・ .../n2-standard-8/
- EUR/USD：https://www.federalreserve.gov/releases/h10/hist/dat00_eu.htm

**v1 沿用、本次未逐項重查的來源：**

- AWS RDS：https://aws.amazon.com/rds/pricing/ ・ https://instances.vantage.sh/aws/rds/db.m7g.large
- AWS RDS 儲存：https://www.usage.ai/blogs/aws/reserved-instances/rds/storage-cost/
- GCP Cloud SQL：https://www.bytebase.com/dbcost/cloudsql-pricing/
- GCP Cloud Storage 與 egress：https://egresscost.com/gcp/
- DigitalOcean Spaces：https://docs.digitalocean.com/products/spaces/details/pricing/
- Langfuse Cloud：https://langfuse.com/pricing
- LiteLLM：https://www.litellm.ai/pricing

> **v1 已移除的來源**：https://www.bitdoze.com/hetzner-cloud-cost-optimized-plans/ ——該文描述 2026-04 調價，早於 2026-06-15 的第二次調價，是 v1 Hetzner 價格錯誤的直接來源。**Hetzner 價格一律以官方調價公告頁為準。**
