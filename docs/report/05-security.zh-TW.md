# 5. 資安設計

Go-Split 儲存誰欠誰多少錢，以及成員的姓名、email 與手機號碼。對這個產品最重要的
四個風險，以及它們對應的 OWASP Top 10 類別：

1. 有人**讀取或修改不屬於自己的活動**（A01）；
2. 成員**做了超出角色權限的事**，例如編輯別人的開銷或進行結算（A01）；
3. **已結算的結果事後被改變**（A04、A08）；
4. **個人資料外洩**，尤其是訪客的 email 與手機號碼（A01、A02、A07）。

以下先列出各 OWASP 類別的狀態，再說明已做的處理，最後列出缺口與改進方式。

## OWASP Top 10（2021）

| 類別 | 對應風險 | 狀態 |
| --- | --- | --- |
| **A01 存取控制失效** | 1、2、4 | 部分處理（缺口：訪客冒用） |
| **A02 加密機制失效** | 4 | 已處理 |
| A03 Injection | — | 已處理 |
| **A04 不安全設計** | 3 | 已處理 |
| A05 安全設定錯誤 | — | 部分處理（缺口：沒有 CSP） |
| A06 易受攻擊的元件 | — | 缺口：沒有相依套件掃描 |
| **A07 識別與驗證失效** | 4 | 部分處理（缺口：沒有 rate limiting） |
| **A08 軟體與資料完整性失效** | 3 | 部分處理（缺口：Actions 未固定 commit SHA） |
| A09 記錄與監控失效 | — | 部分處理（缺口：500 沒有記錄原因） |
| A10 SSRF | — | 不適用：唯一的對外呼叫指向固定的 Google 網址 |

粗體為與上述四個風險直接相關的類別。

## 已做的處理

- **A01 存取控制。** 每個 `/events/{id}` 路由都經過 `RequireSession`（無 session 回
  401）與 `RequireEventRole`（非成員或角色不符回 403）。主辦人可管理整個活動；共同
  主辦人只能編輯自己建立的開銷；成員只能讀取。項目與成員的查詢都同時以物件 id 與
  `event_id` 篩選，其他活動的 id 查不到資料。已結算的活動在 API（409）與資料庫
  trigger 兩層都拒絕寫入。
- **A02 加密。** 瀏覽器到 Vercel、Vercel 到 Cloud Run 全程 HTTPS；Cloud SQL 只接受
  加密連線。Session token 與邀請碼都以 `crypto/rand` 產生。Session cookie 為
  `HttpOnly`、`Secure`、`SameSite=Lax`，前端從不接觸 token。資料庫密碼與 Google
  client ID 存在 Secret Manager；CD 以 Workload Identity Federation 驗證，沒有長期
  金鑰。
- **A03 Injection。** 所有 SQL 都使用 pgx 的 `$n` placeholder，只有伺服器端常數會拼進
  SQL。前端所有使用者內容都經過 JSX escape，沒有 `dangerouslySetInnerHTML` 或 `eval`。
- **A04 安全設計。** 金額一律由伺服器以自己的引擎重算，忽略用戶端算出的總額；結算後
  結果來自記錄引擎版本的凍結快照。邀請碼在活動結算後失效（410）。
- **A05 設定。** 容器以 `FROM scratch` 建置，沒有 shell。Cloud Run 的 service account
  只有 Cloud SQL client、兩個 secret 與 profiling bucket 的權限。Cloud SQL 沒有允許
  任何網路，連線都經過有 IAM 檢查的 connector。Vercel 上沒有任何 secret。
- **A07 驗證。** Google 登入時自行檢查 ID token 的 `aud`、`iss`、`exp` 與
  `email_verified`。登入時伺服器一律建立新的 session token，不接受用戶端提供的，
  因此不會有 session fixation。Session 30 天到期，每個請求都在 SQL 中檢查；登出會
  刪除 session。
- **A08 完整性。** CD 以 Workload Identity Federation 部署；結算快照記錄引擎版本，
  引擎升級不會改變已結算的結果。
- **A09 記錄。** Cloud Run 記錄每個請求並提供請求指標；記錄 log 前會移除 token。

## 缺口與改進

| 缺口 | 風險 | 改進方式 |
| --- | --- | --- |
| A01 訪客可被冒用 | 訪客身分只以未驗證的（email、手機）識別。任何人只要有任一有效邀請碼，加上受害者的 email 與手機，就能取得該訪客在所有活動中的 session | 將訪客限定在單一活動，或在沿用既有訪客前寄送一次性驗證碼到 email |
| A05 沒有 CSP 與 `frame-ancestors` | 只設定了 `Cross-Origin-Opener-Policy`，app 可被其他網站嵌入 frame，XSS 也少了一層防護 | 在 `next.config.ts` 加上 Content Security Policy 與 `frame-ancestors` |
| A06 沒有相依套件掃描 | CI 只跑 `golangci-lint`，Go 與 npm 套件的已知漏洞不會被發現 | 啟用 Dependabot，在 CI 執行 `govulncheck`、`npm audit` 與 `gosec` |
| A07 沒有 rate limiting | `/auth/join`、`/auth/recover` 與公開的邀請碼查詢都沒有限流；邀請碼約 30 bits 的熵，大規模列舉可行 | 對 `/auth/*` 做 per-IP 限流（經 Vercel 轉送時以 `X-Forwarded-For` 辨識來源），並加長邀請碼 |
| A08 Actions 以 tag 固定 | 第三方 Action 的 tag 被改寫時，CD 會執行被竄改的程式碼 | 以 commit SHA 固定 Actions |
| A09 500 沒有記錄原因 | 正式環境的錯誤只看得到失敗，看不到原因，也無法連到對應請求 | JSON log，加上 `severity`、trace id 與錯誤原因（見第 6 節） |
