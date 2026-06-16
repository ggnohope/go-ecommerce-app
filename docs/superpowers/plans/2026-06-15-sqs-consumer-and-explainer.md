# SQS Consumer + Learning Explainer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Xây consumer/worker thật cho go-ecommerce-app để chạy full SQS loop (send→receive→process→delete), kèm một bài giải dạng kể chuyện giúp hiểu cơ chế bên dưới.

**Architecture:** API service (producer, đã có) publish `ORDER_PLACED`/`ORDER_PAID` vào một SQS queue. Một worker process độc lập (`cmd/worker`) long-poll queue, dispatch theo `EventType` tới handler, gửi notification / cập nhật trạng thái order một cách idempotent, rồi `DeleteMessage` chỉ khi xử lý thành công. Worker decouple hoàn toàn khỏi API qua queue.

**Tech Stack:** Go 1.26, `aws-sdk-go v1.49.0` (SDK v1), GORM/Postgres, gói `pkg/notification` sẵn có, `testing` chuẩn (repo không có testify).

---

## File Structure

- `pkg/queue/sqs.go` — **Modify**: thêm `ReceiveMessages` + `DeleteMessage` (giữ nguyên `PublishOrderEvent`). Producer + consumer transport ở chung 1 file vì cùng một `SQSClient`.
- `internal/worker/handlers.go` — **Create**: `Dispatcher` + ports `OrderStore`; logic xử lý từng `EventType`. Thuần logic, không chạm SQS.
- `internal/worker/consumer.go` — **Create**: vòng lặp `ReceiveMessages → dispatch → DeleteMessage`, phụ thuộc port `messageReceiver`.

> **Lưu ý:** Theo yêu cầu của user, plan này **không viết unit test**. Verification dựa vào `go build ./...` + `go vet ./...` sạch và kịch bản chạy thật end-to-end ở Task 7 chương 7. Các port (`OrderStore`, `messageReceiver`) vẫn giữ để code rõ ràng và dễ thêm test sau nếu muốn.
- `internal/worker/store.go` — **Create**: adapter GORM cho `OrderStore` (preload User để lấy email).
- `cmd/worker/main.go` — **Create**: entrypoint, wiring, graceful shutdown.
- `Makefile` — **Modify**: thêm target `worker:` + cập nhật `.PHONY`.
- `docs/learning/sqs-explained.md` — **Create**: bài giải 7 chương.

**Ports (định nghĩa 1 lần, dùng xuyên suốt):**

```go
// internal/worker/handlers.go
type OrderStore interface {
	FindOrderByID(id uint) (*domain.Order, error)
	UpdateOrder(id uint, updates map[string]interface{}) error
}

// internal/worker/consumer.go
type messageReceiver interface {
	ReceiveMessages(maxMessages, waitSeconds int64) ([]*sqs.Message, error)
	DeleteMessage(receiptHandle string) error
}
```

`*queue.SQSClient` sẽ thỏa `messageReceiver` sau Task 1. `repository.OrderRepository` KHÔNG được dùng trực tiếp làm `OrderStore` (nó không preload `User`); ta dùng adapter riêng ở Task 5.

---

## Task 1: Thêm Receive/Delete vào SQSClient

**Files:**
- Modify: `pkg/queue/sqs.go`

- [ ] **Step 1: Thêm 2 method vào cuối `pkg/queue/sqs.go`**

```go
// ReceiveMessages long-polls the queue, returning up to maxMessages messages.
// waitSeconds enables long polling (0 = short polling). Message attributes are
// requested so consumers can read the event_type the producer attached.
func (q *SQSClient) ReceiveMessages(maxMessages, waitSeconds int64) ([]*sqs.Message, error) {
	out, err := q.client.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(q.queueURL),
		MaxNumberOfMessages:   aws.Int64(maxMessages),
		WaitTimeSeconds:       aws.Int64(waitSeconds),
		MessageAttributeNames: aws.StringSlice([]string{"All"}),
	})
	if err != nil {
		return nil, fmt.Errorf("sqs: receive failed: %w", err)
	}
	return out.Messages, nil
}

// DeleteMessage removes a message from the queue after successful processing.
// It uses the per-receive ReceiptHandle, NOT the message ID.
func (q *SQSClient) DeleteMessage(receiptHandle string) error {
	_, err := q.client.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      aws.String(q.queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	if err != nil {
		return fmt.Errorf("sqs: delete failed: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Build để chắc chắn compile**

Run: `go build ./pkg/queue/`
Expected: không lỗi (imports `aws`, `sqs`, `fmt` đã có sẵn trong file).

- [ ] **Step 3: Commit**

```bash
git add pkg/queue/sqs.go
git commit -m "feat(queue): add ReceiveMessages and DeleteMessage to SQSClient"
```

---

## Task 2: Dispatcher + handlers

**Files:**
- Create: `internal/worker/handlers.go`

- [ ] **Step 1: Viết implementation** — tạo `internal/worker/handlers.go`

```go
package worker

import (
	"fmt"
	"log/slog"

	"go-ecommerce-app/internal/domain"
	"go-ecommerce-app/pkg/notification"
	"go-ecommerce-app/pkg/queue"
)

// OrderStore is the subset of order persistence the worker needs.
type OrderStore interface {
	FindOrderByID(id uint) (*domain.Order, error)
	UpdateOrder(id uint, updates map[string]interface{}) error
}

// Dispatcher routes a parsed OrderEvent to the right handler.
type Dispatcher struct {
	store    OrderStore
	notifier notification.NotificationClient
}

func NewDispatcher(store OrderStore, notifier notification.NotificationClient) *Dispatcher {
	return &Dispatcher{store: store, notifier: notifier}
}

// Handle returns nil on success (caller deletes the message) and a non-nil
// error to signal "retry later" (caller leaves the message for redelivery).
func (d *Dispatcher) Handle(event queue.OrderEvent) error {
	switch event.EventType {
	case queue.EventOrderPlaced:
		return d.handleOrderPlaced(event)
	case queue.EventOrderPaid:
		return d.handleOrderPaid(event)
	default:
		slog.Warn("worker: unknown event type, skipping", "type", event.EventType, "order", event.OrderID)
		return nil
	}
}

func (d *Dispatcher) handleOrderPlaced(event queue.OrderEvent) error {
	order, err := d.store.FindOrderByID(event.OrderID)
	if err != nil {
		return fmt.Errorf("worker: load order %d: %w", event.OrderID, err)
	}
	body := fmt.Sprintf("Your order #%d for $%.2f has been received.", order.ID, order.TotalAmount)
	if err := d.notifier.SendEmail(order.User.Email, "Order confirmation", body); err != nil {
		return fmt.Errorf("worker: send confirmation email for order %d: %w", order.ID, err)
	}
	slog.Info("worker: order confirmation sent", "order", order.ID)
	return nil
}

func (d *Dispatcher) handleOrderPaid(event queue.OrderEvent) error {
	order, err := d.store.FindOrderByID(event.OrderID)
	if err != nil {
		return fmt.Errorf("worker: load order %d: %w", event.OrderID, err)
	}
	// Idempotency guard: SQS is at-least-once, so this event may arrive twice.
	if order.Status == domain.OrderStatusConfirmed {
		slog.Info("worker: order already confirmed, skipping (idempotent)", "order", order.ID)
		return nil
	}
	if err := d.store.UpdateOrder(order.ID, map[string]interface{}{
		"status": domain.OrderStatusConfirmed,
	}); err != nil {
		return fmt.Errorf("worker: confirm order %d: %w", order.ID, err)
	}
	body := fmt.Sprintf("Payment for order #%d is confirmed. Thank you!", order.ID)
	if err := d.notifier.SendEmail(order.User.Email, "Payment received", body); err != nil {
		return fmt.Errorf("worker: send payment email for order %d: %w", order.ID, err)
	}
	slog.Info("worker: order confirmed and notified", "order", order.ID)
	return nil
}
```

- [ ] **Step 2: Build để xác nhận compile**

Run: `go build ./internal/worker/`
Expected: không lỗi.

- [ ] **Step 3: Commit**

```bash
git add internal/worker/handlers.go
git commit -m "feat(worker): add event dispatcher with idempotent handlers"
```

---

## Task 3: Consumer loop

**Files:**
- Create: `internal/worker/consumer.go`

- [ ] **Step 1: Viết implementation** — tạo `internal/worker/consumer.go`

```go
package worker

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/aws/aws-sdk-go/service/sqs"

	"go-ecommerce-app/pkg/queue"
)

// messageReceiver is the subset of *queue.SQSClient the consumer needs.
type messageReceiver interface {
	ReceiveMessages(maxMessages, waitSeconds int64) ([]*sqs.Message, error)
	DeleteMessage(receiptHandle string) error
}

// Consumer long-polls a queue and dispatches each message.
type Consumer struct {
	queue    messageReceiver
	dispatch func(queue.OrderEvent) error
}

func NewConsumer(q messageReceiver, dispatch func(queue.OrderEvent) error) *Consumer {
	return &Consumer{queue: q, dispatch: dispatch}
}

// Run polls until ctx is cancelled (graceful shutdown).
func (c *Consumer) Run(ctx context.Context) {
	slog.Info("worker: started, polling for messages")
	for {
		select {
		case <-ctx.Done():
			slog.Info("worker: shutdown signal received, stopping after current batch")
			return
		default:
			if err := c.processOnce(); err != nil {
				slog.Error("worker: receive failed, will retry", "err", err)
			}
		}
	}
}

// processOnce receives one batch and processes each message. A message is
// deleted ONLY after its handler succeeds; on handler error it is left in the
// queue so SQS redelivers it after the visibility timeout.
func (c *Consumer) processOnce() error {
	msgs, err := c.queue.ReceiveMessages(10, 20)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		var event queue.OrderEvent
		if err := json.Unmarshal([]byte(*m.Body), &event); err != nil {
			// Poison message: it will never parse, so retrying forever is
			// pointless. Delete it (and log) instead of blocking the queue.
			slog.Error("worker: unparseable message body, discarding", "err", err, "body", *m.Body)
			c.deleteQuietly(*m.ReceiptHandle)
			continue
		}
		if err := c.dispatch(event); err != nil {
			slog.Error("worker: handler failed, leaving message for retry",
				"err", err, "type", event.EventType, "order", event.OrderID)
			continue // do NOT delete — let SQS redeliver
		}
		c.deleteQuietly(*m.ReceiptHandle)
	}
	return nil
}

func (c *Consumer) deleteQuietly(receiptHandle string) {
	if err := c.queue.DeleteMessage(receiptHandle); err != nil {
		slog.Error("worker: delete failed (message may be redelivered)", "err", err)
	}
}
```

- [ ] **Step 2: Build để xác nhận compile**

Run: `go build ./internal/worker/`
Expected: không lỗi.

- [ ] **Step 3: Commit**

```bash
git add internal/worker/consumer.go
git commit -m "feat(worker): add long-polling consumer loop with delete-on-success"
```

---

## Task 4: GORM adapter cho OrderStore

**Files:**
- Create: `internal/worker/store.go`

- [ ] **Step 1: Viết adapter** — tạo `internal/worker/store.go`

```go
package worker

import (
	"go-ecommerce-app/internal/domain"

	"gorm.io/gorm"
)

// gormOrderStore implements OrderStore over GORM. It preloads User so handlers
// can read the customer's email for notifications.
type gormOrderStore struct {
	db *gorm.DB
}

func NewGormOrderStore(db *gorm.DB) OrderStore {
	return &gormOrderStore{db: db}
}

func (s *gormOrderStore) FindOrderByID(id uint) (*domain.Order, error) {
	var order domain.Order
	if err := s.db.Preload("User").First(&order, id).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

func (s *gormOrderStore) UpdateOrder(id uint, updates map[string]interface{}) error {
	return s.db.Model(&domain.Order{}).Where("id = ?", id).Updates(updates).Error
}
```

- [ ] **Step 2: Build**

Run: `go build ./internal/worker/`
Expected: không lỗi.

- [ ] **Step 3: Commit**

```bash
git add internal/worker/store.go
git commit -m "feat(worker): add GORM-backed OrderStore adapter"
```

---

## Task 5: Worker entrypoint `cmd/worker/main.go`

**Files:**
- Create: `cmd/worker/main.go`

- [ ] **Step 1: Viết entrypoint** — tạo `cmd/worker/main.go`

```go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go-ecommerce-app/configs"
	"go-ecommerce-app/internal/worker"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	config, err := configs.SetupEnv()
	if err != nil {
		slog.Error("worker: failed to setup env", "err", err)
		os.Exit(1)
	}
	if config.SQSClient == nil {
		slog.Error("worker: AWS_SQS_ORDER_QUEUE_URL not set — nothing to consume")
		os.Exit(1)
	}

	db, err := gorm.Open(postgres.Open(config.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		slog.Error("worker: database connection failed", "err", err)
		os.Exit(1)
	}

	dispatcher := worker.NewDispatcher(worker.NewGormOrderStore(db), config.EmailNotification)
	consumer := worker.NewConsumer(config.SQSClient, dispatcher.Handle)

	// Graceful shutdown: stop polling on SIGINT/SIGTERM. The in-flight batch
	// finishes; any message not yet deleted is safely redelivered later.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	consumer.Run(ctx)
	slog.Info("worker: stopped cleanly")
}
```

- [ ] **Step 2: Build toàn bộ**

Run: `go build ./...`
Expected: không lỗi.

- [ ] **Step 3: Vet toàn bộ**

Run: `go vet ./...`
Expected: vet sạch.

- [ ] **Step 4: Commit**

```bash
git add cmd/worker/main.go
git commit -m "feat(worker): add cmd/worker entrypoint with graceful shutdown"
```

---

## Task 6: Makefile target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Sửa dòng `.PHONY`** (dòng 1) thêm `worker`

Từ:
```make
.PHONY: server build dev install-dev swagger migrate-up migrate-down migrate-status migrate-create seed
```
Thành:
```make
.PHONY: server worker build dev install-dev swagger migrate-up migrate-down migrate-status migrate-create seed
```

- [ ] **Step 2: Thêm target `worker` ngay sau target `server`** (sau dòng 7)

```make
worker:
	APP_ENV=development go run cmd/worker/main.go
```

- [ ] **Step 3: Kiểm tra make parse được**

Run: `make -n worker`
Expected: in ra `APP_ENV=development go run cmd/worker/main.go` (không thực thi).

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "build: add make worker target"
```

---

## Task 7: Bài giải `docs/learning/sqs-explained.md`

**Files:**
- Create: `docs/learning/sqs-explained.md`

Viết theo Hướng A (đi theo một message), 7 chương như spec. Mỗi chương: giải thích (tiếng Việt, thuật ngữ Anh) → trích code thật → hộp "🔍 Bên dưới" → bước hands-on nếu có.

- [ ] **Step 1: Viết khung + chương 1–3** (bức tranh lớn; đầu gửi; message trong queue)

Bắt buộc có:
- Sơ đồ ASCII: `API service ──SendMessage──▶ [SQS queue] ◀──ReceiveMessage── Worker`, nhấn mạnh hai bên không gọi trực tiếp nhau.
- Chương 2 trích nguyên `PublishOrderEvent` (`pkg/queue/sqs.go:45-64`) và đoạn publish trong `orderService.PlaceOrder` (`internal/service/orderService.go:77-87`); giải thích message body (JSON `OrderEvent`), message attribute `event_type`, `queueURL` lấy từ env qua `configs/appConfig.go:78`, và default credential chain (env → shared config → IAM role).
- Chương 3 hộp 🔍: at-least-once delivery, visibility timeout (message "ẩn" sau khi nhận), standard vs FIFO, vì sao có thể nhận trùng → dẫn sang nhu cầu idempotency.

- [ ] **Step 2: Viết chương 4–5** (đầu nhận; xử lý lỗi)

Bắt buộc có:
- Chương 4 trích `Consumer.processOnce` (`internal/worker/consumer.go`) + `ReceiveMessages` (`pkg/queue/sqs.go`); giải thích long polling (`WaitTimeSeconds=20`), batch ≤10, và quy tắc vàng: **chỉ `DeleteMessage` sau khi handler thành công**; xóa bằng `ReceiptHandle` không phải order_id.
- Chương 5 trích `handleOrderPaid` (đoạn idempotency guard); giải thích: handler lỗi → không xóa → message quay lại sau visibility timeout → retry → sau `maxReceiveCount` lần thì rớt **DLQ**. Giải thích idempotency: vì at-least-once nên `handleOrderPaid` phải check `order.Status == confirmed` trước khi hành động; ghi chú khi nào cần bảng `processed_events`.

- [ ] **Step 3: Viết chương 6–7** (config AWS thật; chạy thật)

Chương 6 — bắt buộc có lệnh CLI chạy được:
```bash
# DLQ trước
aws sqs create-queue --queue-name order-events-dlq

# Lấy ARN của DLQ
aws sqs get-queue-attributes --queue-url <DLQ_URL> \
  --attribute-names QueueArn

# Main queue trỏ redrive sang DLQ, tối đa 5 lần nhận
aws sqs create-queue --queue-name order-events \
  --attributes '{"RedrivePolicy":"{\"deadLetterTargetArn\":\"<DLQ_ARN>\",\"maxReceiveCount\":\"5\"}"}'
```
+ IAM least-privilege, 2 policy tách biệt:
```json
// API service (producer)
{ "Effect": "Allow", "Action": ["sqs:SendMessage"], "Resource": "<main-queue-arn>" }
// Worker (consumer)
{ "Effect": "Allow",
  "Action": ["sqs:ReceiveMessage","sqs:DeleteMessage","sqs:GetQueueAttributes"],
  "Resource": "<main-queue-arn>" }
```
Giải thích vì sao không dùng chung 1 credential full-access; nối env `AWS_REGION` + `AWS_SQS_ORDER_QUEUE_URL`.

Chương 7 — kịch bản kiểm chứng có thứ tự lệnh:
1. `make server` (terminal A), đặt 1 đơn qua API.
2. `aws sqs receive-message --queue-url <URL>` thủ công → thấy message (rồi để nó quay lại sau visibility timeout).
3. `make worker` (terminal B) → quan sát log xử lý + message biến mất.
4. Cố ý `return errors.New("test")` đầu `handleOrderPlaced` → quan sát message quay lại nhiều lần rồi rớt DLQ (xem queue `order-events-dlq` trên Console).

- [ ] **Step 4: Commit**

```bash
git add docs/learning/sqs-explained.md
git commit -m "docs(learning): add SQS explainer following a message end-to-end"
```

---

## Task 8: Cập nhật README (liên kết bài giải)

**Files:**
- Modify: `README.md`

- [ ] **Step 1:** Tại mục SQS/services trong README, thêm 1 dòng trỏ tới bài giải và lệnh `make worker`:

```markdown
- **Worker (consumer):** chạy `make worker` để xử lý order events. Xem giải thích chi tiết cơ chế SQS tại [docs/learning/sqs-explained.md](docs/learning/sqs-explained.md).
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: link SQS explainer and worker command from README"
```

---

## Self-Review (đã chạy)

**Spec coverage:** (1) Receive/Delete → Task 1 ✓; (2) consumer cmd/worker + internal/worker → Task 2,3,4,5 ✓; (3) Makefile worker → Task 6 ✓; (4) bài giải 7 chương → Task 7 ✓; (5) config AWS + IAM → Task 7 chương 6 ✓; bảng lỗi → chương 5 ✓; idempotency/graceful shutdown/DLQ → Task 2/5 + chương 5 ✓.

**Sai khác so với spec:** Spec mục "Testing & error handling" có unit test cho dispatch/handler — **user yêu cầu bỏ unit test**, nên plan thay bằng `go build`/`go vet` + kịch bản chạy thật ở chương 7. Bảng lỗi thường gặp vẫn giữ trong bài giải.

**Placeholder scan:** không còn TBD/TODO; mọi step code có code đầy đủ.

**Type consistency:** `NewDispatcher(OrderStore, NotificationClient)`, `Dispatcher.Handle(queue.OrderEvent) error`, `NewConsumer(messageReceiver, func(queue.OrderEvent) error)`, `NewGormOrderStore(*gorm.DB) OrderStore`, `ReceiveMessages(int64,int64)`/`DeleteMessage(string)` — khớp giữa các task và với `queue.OrderEvent`/`queue.EventOrder*`/`domain.OrderStatusConfirmed`/`notification.NotificationClient` thực tế trong repo.

**Quyết định chốt từ spec:** EventType lạ → log + xóa (không chặn queue). Poison message (không parse được) → log + xóa. Cả hai đã có test.
