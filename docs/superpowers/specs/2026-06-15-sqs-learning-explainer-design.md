# Design: Bài giải AWS SQS + xây consumer cho go-ecommerce-app

**Ngày:** 2026-06-15
**Trạng thái:** Đã duyệt thiết kế, chờ review spec trước khi lập plan

## Mục tiêu

Giúp user thực sự hiểu "cái gì chạy bên dưới" khi go-ecommerce-app dùng AWS SQS — không chỉ
đọc code AI generate. Đạt được bằng cách: (1) một bài giải dạng kể chuyện bám theo hành trình
một message, và (2) xây nốt phần **consumer/worker còn thiếu** để user chạy được full loop
`send → receive → process → delete` trên **AWS thật**.

Trình độ người học: biết AWS cơ bản (credential, region, đã dùng vài service), nhưng chưa nắm
cơ chế SQS và message queue. Vì vậy giải thích **sâu** phần SQS/queue mechanics, **không** giảng
lại phần AWS account/credential cơ bản.

## Bối cảnh code hiện tại (đã khảo sát)

- `pkg/queue/sqs.go`: **chỉ có producer** — `PublishOrderEvent` → `SendMessage`. Định nghĩa
  `OrderEvent{EventType, OrderID, UserID, Amount}` và 3 event type: `ORDER_PLACED`,
  `ORDER_PAID`, `ORDER_SHIPPED` (cái cuối định nghĩa nhưng chưa publish).
- `internal/service/orderService.go`: publish `ORDER_PLACED` trong `PlaceOrder`, publish
  `ORDER_PAID` trong `HandleStripeEvent` (khi `payment_intent.succeeded`). SQS là optional
  (nil-check), publish lỗi chỉ log chứ không fail request.
- `configs/appConfig.go`: khởi tạo `SQSClient` từ env `AWS_REGION` + `AWS_SQS_ORDER_QUEUE_URL`;
  optional, thiếu env thì disable order events.
- **Chưa có consumer nào** (`grep` ReceiveMessage/DeleteMessage/worker/consumer = rỗng).
- `infra/` rỗng → **không có IaC**, AWS config thủ công qua Console/CLI.
- SDK: `aws-sdk-go v1.49.0` (SDK v1). Auth qua default credential chain.
- `pkg/notification`: `NotificationClient` interface có `SendEmail(to, subject, body)` và
  `SendSMS(to, message)` — worker sẽ tái dùng để xử lý event.
- Makefile theo pattern target đơn giản (`server:`, `dev:`, `build:`...).

**Phát hiện cốt lõi:** user đang thấy nửa vòng lặp (gửi đi) mà chưa thấy nửa nhận/xử lý — đúng
chỗ làm "cái gì chạy bên dưới" trở nên mơ hồ nhất.

## Hướng tiếp cận đã chọn

**Hướng A — "Đi theo một message".** Bài giải kể chuyện bám hành trình một đơn hàng; mỗi khái
niệm SQS được giải thích ngay khi xuất hiện trong dòng chảy, gắn với code thật + bước hands-on.
(Loại bỏ Hướng B "lý thuyết trước" và Hướng C "code-first tối giản" vì không đạt mục tiêu hiểu sâu.)

## Deliverables

Tạo trong repo (không phá code cũ):

```
cmd/worker/main.go              # entrypoint: load config, init consumer, chạy poll loop + graceful shutdown
internal/worker/consumer.go     # vòng lặp ReceiveMessage → dispatch → DeleteMessage
internal/worker/handlers.go     # xử lý từng EventType (gửi notification, cập nhật trạng thái)
internal/worker/handlers_test.go# unit test dispatch + handler (SQS giả lập qua interface)
pkg/queue/sqs.go                # THÊM ReceiveMessages() + DeleteMessage(), giữ nguyên Publish
docs/learning/sqs-explained.md  # bài giải Hướng A
Makefile                        # THÊM target `worker:`
```

Lý do tách `cmd/worker` riêng: phản ánh mô hình thật — API service (producer) và worker
(consumer) là 2 process độc lập, scale/deploy riêng. Đây chính là giá trị của SQS: hai bên không
gọi trực tiếp nhau.

## Dàn ý bài giải (`docs/learning/sqs-explained.md`)

Mỗi chương: giải thích (tiếng Việt, thuật ngữ Anh giữ nguyên) → trích code thật → hộp "🔍 Bên
dưới" giải thích internals → (nếu có) bước hands-on.

1. **Bức tranh lớn** — vì sao cần queue; sơ đồ API service và worker không gọi trực tiếp nhau,
   SQS đứng giữa; vấn đề giải quyết (decoupling, chịu tải, retry).
2. **Đầu gửi (code đã có)** — soi `PlaceOrder` → `PublishOrderEvent` → `SendMessage`; message
   body, message attributes, queue URL từ đâu, default credential chain.
3. **Message nằm trong queue** — 🔍 SQS lưu message ra sao; at-least-once delivery; visibility
   timeout (vì sao message "biến mất" tạm thời); standard vs FIFO; vì sao có thể nhận trùng.
4. **Đầu nhận (ta xây)** — long polling vs short polling; `ReceiveMessage` lấy batch; xử lý rồi
   bắt buộc `DeleteMessage`; không xóa thì sao.
5. **Khi xử lý lỗi** — message quay lại queue; retry; Dead Letter Queue; idempotency (handler
   phải chịu được chạy trùng do at-least-once).
6. **Config AWS thật** — tạo queue + DLQ; IAM least-privilege; set env.
7. **Chạy thật** — đặt đơn → xem message trong queue → chạy worker → thấy xử lý + xóa; quan sát
   visibility timeout; cố ý làm handler lỗi để thấy retry và rớt DLQ.

## Thiết kế consumer

### Bổ sung `pkg/queue/sqs.go`

```go
// ReceiveMessages: long polling, tối đa maxMessages (<=10) mỗi lần
func (q *SQSClient) ReceiveMessages(maxMessages int64, waitSeconds int64) ([]*sqs.Message, error)
// DeleteMessage: xóa sau khi xử lý xong, dùng ReceiptHandle (KHÔNG phải message ID)
func (q *SQSClient) DeleteMessage(receiptHandle string) error
```
Điểm dạy: `WaitTimeSeconds` (long polling giảm cost + latency), `MaxNumberOfMessages` (batch ≤10),
vì sao xóa bằng `ReceiptHandle` chứ không phải `OrderID`. Cũng request `MessageAttributeNames`
để đọc lại `event_type` attribute mà producer gắn.

### `internal/worker/consumer.go` — vòng lặp chính

```
for {
    msgs = ReceiveMessages(10, 20)            // long poll 20s
    for each msg:
        event = json.Unmarshal(msg.Body)      // parse OrderEvent
        err = dispatch(event)                 // gọi handler theo EventType
        if err == nil:
            DeleteMessage(msg.ReceiptHandle)  // CHỈ xóa khi thành công
        else:
            log + KHÔNG xóa → quay lại sau visibility timeout → retry → (n lần) → DLQ
}
```
Quyết định cốt lõi: **xóa message sau khi xử lý thành công, không phải sau khi nhận** — đây là
cách SQS đảm bảo không mất việc và là lý do at-least-once tồn tại.

### `internal/worker/handlers.go` — dispatch theo EventType

- `ORDER_PLACED` → gửi email xác nhận đơn (qua `NotificationClient`).
- `ORDER_PAID` → gửi email "đã thanh toán" / kích hoạt fulfillment.
- Mỗi handler nhận `OrderEvent` đã parse, trả `error`. Lỗi → không xóa → retry.
- EventType lạ → log warning + xóa (tránh kẹt poison message vô hạn) HOẶC để rớt DLQ — chốt
  trong plan; mặc định: log + xóa để không chặn queue.

### Idempotency

Vì at-least-once, handler phải chịu được chạy trùng. Minh hoạ pattern đơn giản: check trạng thái
order trong DB trước khi hành động (nếu đã ở trạng thái đích thì bỏ qua). Ghi chú khi nào cần bảng
`processed_events` riêng (dedup theo message/event ID).

### Graceful shutdown

Bắt `SIGINT`/`SIGTERM`: dừng nhận message mới, xử lý nốt batch hiện tại rồi thoát. Dạy: vì sao an
toàn — message chưa `DeleteMessage` sẽ tự quay lại sau visibility timeout.

### Makefile

Thêm target `worker:` chạy `go run cmd/worker/main.go`, đồng bộ style với `server:`.

## Config AWS thật + IAM

1. **Tạo queue**: main queue `order-events` + DLQ `order-events-dlq`, gắn redrive policy
   `maxReceiveCount=5`. Hướng dẫn cả Console lẫn `aws sqs create-queue` CLI.
2. **Queue URL** → đặt vào `AWS_SQS_ORDER_QUEUE_URL`.
3. **IAM least-privilege** — 2 policy tách biệt:
   - API service (producer): chỉ `sqs:SendMessage` trên main queue.
   - Worker (consumer): `sqs:ReceiveMessage`, `sqs:DeleteMessage`, `sqs:GetQueueAttributes`.
   - Giải thích vì sao không dùng chung 1 credential full-access.
4. **Credential local dev**: profile/`~/.aws/credentials` hoặc env, nối lại với default
   credential chain ở chương 2.
5. **Kiểm chứng**: đặt đơn qua API → `aws sqs receive-message` thủ công → chạy worker → thấy xử
   lý + biến mất; cố ý lỗi handler để quan sát retry + DLQ.

## Testing & error handling (Section 5)

- **Unit test** (`handlers_test.go`): tách logic xử lý khỏi I/O SQS. `dispatch` và handler nhận
  `OrderEvent` + một `NotificationClient` giả lập (fake/stub), assert đúng hành vi (gọi đúng
  email, trả error đúng lúc). Consumer loop dựa trên một interface SQS nhỏ để test poll/delete
  bằng fake, không cần AWS thật.
- **Bảng lỗi thường gặp** trong bài giải: credential sai/thiếu, queue URL sai region,
  message body không parse được (poison message), handler timeout > visibility timeout (xử lý
  trùng), quên DeleteMessage (xử lý lặp vô hạn) — mỗi lỗi kèm triệu chứng + cách nhận biết.

## Phạm vi loại trừ (YAGNI)

- Không thêm IaC/Terraform (giữ config thủ công theo hiện trạng).
- Không nâng SDK lên v2 (giữ `aws-sdk-go v1` đồng bộ phần còn lại của repo).
- Không xây bảng `processed_events` thật — chỉ giải thích khi nào cần.
- Không động vào FIFO queue ngoài phần giải thích khái niệm (queue thật dùng standard).

## Tiêu chí thành công

- User đọc bài giải và giải thích lại được: visibility timeout, at-least-once, long polling, DLQ,
  idempotency là gì và vì sao consumer xóa message *sau* khi xử lý.
- `make worker` chạy được, nối tới AWS thật, nhận và xử lý `ORDER_PLACED`/`ORDER_PAID` rồi xóa.
- Quan sát được retry + rớt DLQ khi handler cố ý lỗi.
- Unit test cho dispatch/handler pass.
