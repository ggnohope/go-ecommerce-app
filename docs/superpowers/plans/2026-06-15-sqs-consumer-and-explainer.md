# SQS Consumer + Learning Explainer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a real consumer/worker for go-ecommerce-app to run the full SQS loop (send→receive→process→delete), together with a narrative-style explainer that helps you understand the mechanics underneath.

**Architecture:** The API service (producer, already exists) publishes `ORDER_PLACED`/`ORDER_PAID` to an SQS queue. An independent worker process (`cmd/worker`) long-polls the queue, dispatches by `EventType` to a handler, sends notifications / updates the order status idempotently, then calls `DeleteMessage` only on successful processing. The worker is fully decoupled from the API via the queue.

**Tech Stack:** Go 1.26, `aws-sdk-go v1.49.0` (SDK v1), GORM/Postgres, the existing `pkg/notification` package, standard `testing` (the repo has no testify).

---

## File Structure

- `pkg/queue/sqs.go` — **Modify**: add `ReceiveMessages` + `DeleteMessage` (keep `PublishOrderEvent` unchanged). Producer + consumer transport live in the same file because they share one `SQSClient`.
- `internal/worker/handlers.go` — **Create**: `Dispatcher` + the `OrderStore` port; logic for handling each `EventType`. Pure logic, no SQS.
- `internal/worker/consumer.go` — **Create**: the `ReceiveMessages → dispatch → DeleteMessage` loop, depending on the `messageReceiver` port.

> **Note:** Per the user's request, this plan writes **no unit tests**. Verification relies on a clean `go build ./...` + `go vet ./...` and the real end-to-end run scenario in Task 7 chapter 7. The ports (`OrderStore`, `messageReceiver`) are still kept so the code is clear and tests are easy to add later if desired.
- `internal/worker/store.go` — **Create**: a GORM adapter for `OrderStore` (preloads User to get the email).
- `cmd/worker/main.go` — **Create**: entrypoint, wiring, graceful shutdown.
- `Makefile` — **Modify**: add a `worker:` target + update `.PHONY`.
- `docs/learning/sqs-explained.md` — **Create**: the 7-chapter explainer.

**Ports (defined once, used throughout):**

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

`*queue.SQSClient` will satisfy `messageReceiver` after Task 1. `repository.OrderRepository` must NOT be used directly as `OrderStore` (it doesn't preload `User`); we use a dedicated adapter in Task 5.

---

## Task 1: Add Receive/Delete to SQSClient

**Files:**
- Modify: `pkg/queue/sqs.go`

- [ ] **Step 1: Add 2 methods at the end of `pkg/queue/sqs.go`**

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

- [ ] **Step 2: Build to make sure it compiles**

Run: `go build ./pkg/queue/`
Expected: no errors (the `aws`, `sqs`, `fmt` imports are already in the file).

- [ ] **Step 3: Commit**

```bash
git add pkg/queue/sqs.go
git commit -m "feat(queue): add ReceiveMessages and DeleteMessage to SQSClient"
```

---

## Task 2: Dispatcher + handlers

**Files:**
- Create: `internal/worker/handlers.go`

- [ ] **Step 1: Write the implementation** — create `internal/worker/handlers.go`

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

- [ ] **Step 2: Build to confirm it compiles**

Run: `go build ./internal/worker/`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/worker/handlers.go
git commit -m "feat(worker): add event dispatcher with idempotent handlers"
```

---

## Task 3: Consumer loop

**Files:**
- Create: `internal/worker/consumer.go`

- [ ] **Step 1: Write the implementation** — create `internal/worker/consumer.go`

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

- [ ] **Step 2: Build to confirm it compiles**

Run: `go build ./internal/worker/`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/worker/consumer.go
git commit -m "feat(worker): add long-polling consumer loop with delete-on-success"
```

---

## Task 4: GORM adapter for OrderStore

**Files:**
- Create: `internal/worker/store.go`

- [ ] **Step 1: Write the adapter** — create `internal/worker/store.go`

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
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/worker/store.go
git commit -m "feat(worker): add GORM-backed OrderStore adapter"
```

---

## Task 5: Worker entrypoint `cmd/worker/main.go`

**Files:**
- Create: `cmd/worker/main.go`

- [ ] **Step 1: Write the entrypoint** — create `cmd/worker/main.go`

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

- [ ] **Step 2: Build everything**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 3: Vet everything**

Run: `go vet ./...`
Expected: clean vet.

- [ ] **Step 4: Commit**

```bash
git add cmd/worker/main.go
git commit -m "feat(worker): add cmd/worker entrypoint with graceful shutdown"
```

---

## Task 6: Makefile target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Edit the `.PHONY` line** (line 1) to add `worker`

From:
```make
.PHONY: server build dev install-dev swagger migrate-up migrate-down migrate-status migrate-create seed
```
To:
```make
.PHONY: server worker build dev install-dev swagger migrate-up migrate-down migrate-status migrate-create seed
```

- [ ] **Step 2: Add a `worker` target right after the `server` target** (after line 7)

```make
worker:
	APP_ENV=development go run cmd/worker/main.go
```

- [ ] **Step 3: Check that make parses**

Run: `make -n worker`
Expected: prints `APP_ENV=development go run cmd/worker/main.go` (without executing).

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "build: add make worker target"
```

---

## Task 7: The explainer `docs/learning/sqs-explained.md`

**Files:**
- Create: `docs/learning/sqs-explained.md`

Write it following Approach A (follow a message), 7 chapters as in the spec. Each chapter: explanation (in Vietnamese, English technical terms kept) → real code excerpt → a "🔍 Under the hood" box → a hands-on step if applicable.

- [ ] **Step 1: Write the skeleton + chapters 1–3** (the big picture; the sending end; the message in the queue)

Must include:
- An ASCII diagram: `API service ──SendMessage──▶ [SQS queue] ◀──ReceiveMessage── Worker`, emphasizing that the two sides don't call each other directly.
- Chapter 2 quotes `PublishOrderEvent` verbatim (`pkg/queue/sqs.go:45-64`) and the publish snippet in `orderService.PlaceOrder` (`internal/service/orderService.go:77-87`); explain the message body (JSON `OrderEvent`), the `event_type` message attribute, `queueURL` read from env via `configs/appConfig.go:78`, and the default credential chain (env → shared config → IAM role).
- Chapter 3 🔍 box: at-least-once delivery, visibility timeout (the message is "hidden" after being received), standard vs FIFO, why duplicates can be received → leading into the need for idempotency.

- [ ] **Step 2: Write chapters 4–5** (the receiving end; error handling)

Must include:
- Chapter 4 quotes `Consumer.processOnce` (`internal/worker/consumer.go`) + `ReceiveMessages` (`pkg/queue/sqs.go`); explain long polling (`WaitTimeSeconds=20`), batch ≤10, and the golden rule: **only `DeleteMessage` after the handler succeeds**; delete by `ReceiptHandle`, not order_id.
- Chapter 5 quotes `handleOrderPaid` (the idempotency guard snippet); explain: handler error → don't delete → the message returns after the visibility timeout → retry → after `maxReceiveCount` attempts it lands in the **DLQ**. Explain idempotency: because of at-least-once, `handleOrderPaid` must check `order.Status == confirmed` before acting; note when a `processed_events` table is needed.

- [ ] **Step 3: Write chapters 6–7** (real AWS config; running it for real)

Chapter 6 — must include runnable CLI commands:
```bash
# DLQ first
aws sqs create-queue --queue-name order-events-dlq

# Get the DLQ's ARN
aws sqs get-queue-attributes --queue-url <DLQ_URL> \
  --attribute-names QueueArn

# Main queue pointing its redrive to the DLQ, max 5 receives
aws sqs create-queue --queue-name order-events \
  --attributes '{"RedrivePolicy":"{\"deadLetterTargetArn\":\"<DLQ_ARN>\",\"maxReceiveCount\":\"5\"}"}'
```
+ IAM least-privilege, 2 separate policies:
```json
// API service (producer)
{ "Effect": "Allow", "Action": ["sqs:SendMessage"], "Resource": "<main-queue-arn>" }
// Worker (consumer)
{ "Effect": "Allow",
  "Action": ["sqs:ReceiveMessage","sqs:DeleteMessage","sqs:GetQueueAttributes"],
  "Resource": "<main-queue-arn>" }
```
Explain why not to share one full-access credential; connect the env `AWS_REGION` + `AWS_SQS_ORDER_QUEUE_URL`.

Chapter 7 — a verification scenario with an ordered sequence of commands:
1. `make server` (terminal A), place 1 order via the API.
2. `aws sqs receive-message --queue-url <URL>` manually → see the message (then let it return after the visibility timeout).
3. `make worker` (terminal B) → observe the processing logs + the message disappearing.
4. Deliberately `return errors.New("test")` at the top of `handleOrderPlaced` → observe the message returning several times then landing in the DLQ (check the `order-events-dlq` queue in the Console).

- [ ] **Step 4: Commit**

```bash
git add docs/learning/sqs-explained.md
git commit -m "docs(learning): add SQS explainer following a message end-to-end"
```

---

## Task 8: Update the README (link the explainer)

**Files:**
- Modify: `README.md`

- [ ] **Step 1:** In the SQS/services section of the README, add a line pointing to the explainer and the `make worker` command:

```markdown
- **Worker (consumer):** run `make worker` to process order events. See a detailed explanation of the SQS mechanics at [docs/learning/sqs-explained.md](docs/learning/sqs-explained.md).
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: link SQS explainer and worker command from README"
```

---

## Self-Review (done)

**Spec coverage:** (1) Receive/Delete → Task 1 ✓; (2) consumer cmd/worker + internal/worker → Task 2,3,4,5 ✓; (3) Makefile worker → Task 6 ✓; (4) 7-chapter explainer → Task 7 ✓; (5) AWS config + IAM → Task 7 chapter 6 ✓; error table → chapter 5 ✓; idempotency/graceful shutdown/DLQ → Task 2/5 + chapter 5 ✓.

**Deviations from the spec:** The spec's "Testing & error handling" section had unit tests for dispatch/handlers — **the user requested dropping unit tests**, so the plan replaces them with `go build`/`go vet` + the real run scenario in chapter 7. The common-errors table is still kept in the explainer.

**Placeholder scan:** no remaining TBD/TODO; every code step has complete code.

**Type consistency:** `NewDispatcher(OrderStore, NotificationClient)`, `Dispatcher.Handle(queue.OrderEvent) error`, `NewConsumer(messageReceiver, func(queue.OrderEvent) error)`, `NewGormOrderStore(*gorm.DB) OrderStore`, `ReceiveMessages(int64,int64)`/`DeleteMessage(string)` — consistent across tasks and with the actual `queue.OrderEvent`/`queue.EventOrder*`/`domain.OrderStatusConfirmed`/`notification.NotificationClient` in the repo.

**Decisions locked from the spec:** unknown EventType → log + delete (don't block the queue). Poison message (unparseable) → log + delete. Both are covered.
