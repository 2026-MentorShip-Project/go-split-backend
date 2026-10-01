# 6. 技術債紀錄

以下是開發期間刻意接受、或尚未處理的主要取捨。資安相關的未解決風險見第 5 節。

| 技術債 | 做了什麼 | 為什麼 | 修正方向 |
| --- | --- | --- | --- |
| 資料庫連線接近上限 | 每個 instance 4 條連線（`DB_MAX_CONNS`），5 個 instance 共 20 條，接近 `db-f1-micro` 約 25 條的上限 | 這個階段以成本優先，使用最小的方案 | 換更大的 Cloud SQL 方案或加連線池代理 |
| 資料庫沒有高可用 | 單一 zone、沒有 standby；有每日備份與時間點還原 | 這個階段以成本優先，standby 約讓資料庫費用加倍 | 需要可用性時改為 `REGIONAL` |
| 每個 API 請求都經 Vercel 轉送 | 瀏覽器呼叫 Vercel 上的 `/api/*`，再由 Next.js server 轉送到 Cloud Run；實測每個請求多約 41 ms（P50）／53 ms（P95） | 小專案不想購買自有網域；前後端不同網站時 session cookie 會成為第三方而被瀏覽器擋掉（第 1 節） | 購買網域，將前端與 API 放在同一網站的子網域下，瀏覽器即可直接呼叫 Cloud Run |
| 錯誤沒有記錄原因 | 多數 500 只回傳固定訊息；log 為純文字，沒有 severity，也沒有連到請求的 trace id | 開發期間本機除錯就夠用 | JSON log，加上 `severity`、trace id 與錯誤原因 |
| 前端錯誤處理與監控 | 許多載入錯誤被空的 `catch` 吞掉；沒有處理 401；瀏覽器端沒有錯誤回報 | 從原型延續，優先完成功能 | 共用錯誤與 toast 路徑、401 導向登入；規劃導入 Sentry（關閉 PII 收集） |
| 前端沒有測試 | 沒有 E2E 測試，`src/components/ui` 的 12 個共用元件（design system）也沒有單元測試 | 從原型延續，優先完成功能 | 為共用元件寫單元測試，並為建立活動與加入流程寫 E2E smoke test |
| 前端沒有錯誤頁面 | App Router 沒有 `error.tsx`、`global-error.tsx` 或 `not-found.tsx`；頁面錯誤或不存在的路由只會顯示 Next.js 預設畫面 | 從原型延續，優先完成功能 | 加上錯誤與 404 頁面，提供重試與回到首頁 |
| 部署沒有把關 | 後端 CD 不等 CI 就部署，Terraform 以 `-auto-approve` 套用；前端沒有任何 CI，push 到 `main` 即上線 | 預期所有變更都經過審查的 pull request | 部署依賴 CI、`terraform plan` 需核准；前端加上 lint、型別檢查與建置 |
| Migration 只能往前 | 自製 runner 依檔名套用、沒有 down migration；每次 instance 啟動都會再執行 | 最簡單的做法，migration 工具列為後續工作 | 改用 goose 或 golang-migrate，先擴充後收縮，只在 CD 執行 |
