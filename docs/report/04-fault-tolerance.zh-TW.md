# 4. 容錯與韌性設計

Go-Split 處理的是金錢，所以設計上優先確保**故障時的正確性**：失敗的請求不能留下
寫到一半的資料，已結算的活動永遠不能改變。

## 總覽

| 故障 | 結果 | 位置 |
| --- | --- | --- |
| 請求中途 handler 出錯或 panic | 交易 rollback；用戶端收到錯誤，絕不會收到假的成功 | `internal/events/transaction.go`、`server.go` recovery |
| 同一活動同時有兩筆寫入 | 由活動列的鎖序列化；第二筆等待 | `eventTransaction`（`FOR UPDATE`） |
| 寫入已結算的活動 | API 拒絕（409），資料庫 trigger 再擋一次 | `transaction.go`、migration `0009` |
| Google 登入緩慢或故障 | 逾時 10 秒後回 504；其他上游錯誤回 502 | `internal/auth/google.go` |
| Profile 上傳到 GCS 失敗 | 記錄 log 後略過；請求不受影響 | `internal/profiling` |
| 啟動時資料庫無法連線 | 程序結束；Cloud Run 重啟 instance | `server.go`（`log.Fatalf`） |
| Instance 關閉（部署、縮減） | 進行中的請求最多有 10 秒完成 | `server.go` graceful shutdown |
| 資料庫 instance 或 zone 失效 | 停機直到恢復；可從備份／PITR 還原資料 | Terraform `deploy/gcp/main.tf` |

## 錯誤處理

**後端。** 每個 handler 都明確選擇狀態碼，並以相同的 JSON 格式 `{"error": "..."}`
回應。

Panic 由 Gin 的 recovery middleware 捕捉、連同 stack 記錄 log，並回應 500。活動
交易的 deferred rollback 仍會執行，所以 panic 不會留下部分寫入。

**前端。** `readApiError` 將錯誤格式轉成訊息，並附上 422／409 的代碼：
`error（code1、code2）`。頁面以錯誤橫幅（結算、規則）、toast（成員）或表單下方的
inline 訊息（建立活動、項目明細）顯示錯誤。規則頁採樂觀更新，若儲存失敗則重新向
伺服器取得資料，讓畫面與已儲存的狀態一致。

## 資料一致性

由三層把關。

**1. 每個活動請求一個交易。** 一個請求的所有寫入都在同一個交易中：全部成功才
commit，任何錯誤都整筆 rollback。回應等 commit 後才送出，所以用戶端不會對未儲存
的資料收到成功。

**2. 資料庫也負責把關規則。** API 檢查不是唯一的防線：資料庫本身也以 unique
index、check 約束與 trigger 限制欄位，例如每個活動恰有一位主辦人、已結算的活動
不能再寫入。即使 handler 有 bug，也改不了已結算的活動。

**3. 金額精確，結算凍結。** 金額一律以整數元儲存；結算後所有結果都凍結，不再
改變。

## 資料庫韌性

Cloud SQL 有每日備份與時間點還原、僅允許加密連線，並自動擴充磁碟。它在單一 zone
執行、沒有備援，所以 zone 或 instance 故障就會停機直到恢復（見第 6 節）。

## 可觀測性與其限制

我們依賴 Google Cloud 預設提供的功能，沒有額外工具：

| 訊號 | 能取得的內容 | 來源 |
| --- | --- | --- |
| 請求 log | 每個請求一筆：method、URL、狀態碼、延遲、大小、user agent、用戶端 IP、trace id、instance | Cloud Run，自動 |
| 應用程式 log | 伺服器寫到 stdout／stderr 的內容（logrus，純文字） | Cloud Run，自動 |
| 指標 | 請求數與延遲、instance 數、CPU、記憶體、啟動延遲、並行數、流入／流出位元組 | Cloud Run，內建 |
| Trace | 每個取樣請求一個 span，每個 instance 每 10 秒最多 1 個請求，免費 | Cloud Run → Cloud Trace，自動 |
| Profile | 每 10 分鐘一次 CPU profile，用於 PGO | 自己寫的上傳程式，存到 GCS |

這能看出請求**失敗了**、何時、在哪個 instance，但多半看不出**為什麼**，或**哪個
endpoint** 變慢：

- **請求 log 沒有內容。** 不含請求或回應內容，也沒有呼叫者身分。看得到
  `POST /events/42/items` 失敗，但看不到送了什麼、是誰送的。
- **應用程式 log 還補不上這個缺口。** 多數 500 不會記錄原因。log 是純文字，所以
  Cloud Logging 儲存時沒有 severity；沒有 `logging.googleapis.com/trace` 欄位，也就
  不會歸到對應的請求底下（第 6 節）。
- **Log 只保留 30 天。** 這是 `_Default` log bucket 的預設值；可提高到最多 3,650 天，
  但要多付儲存費用。一個月後才回報的問題就沒有 log 可查。
- **指標無法依 endpoint 拆分。** 請求數與延遲可以依狀態碼與 revision 分組，但沒有
  URL 路徑的 label，所以「P95 上升」看不出是哪個路由。Log-based metric 可以加上
  路由，但我們的路徑含有 id（`/events/42/items`），需要先正規化。
- **看不到程序內部。** 沒有量測等待連線池或活動鎖的時間、查詢耗時，或 Go runtime
  健康狀況（goroutine、GC）。第 3 節發現這些等待正是延遲的主因。Cloud SQL 自己的
  指標能部分涵蓋資料庫端。
- **Trace 只顯示總時間。** Cloud Run 自動產生的 trace 每個請求只有一個 span，看不出
  是哪個查詢或鎖花了時間。子 span 需要 OpenTelemetry instrumentation，Cloud Trace
  會另外計費。
