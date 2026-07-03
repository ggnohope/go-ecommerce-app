# Design: AWS SQS explainer + building the consumer for go-ecommerce-app

**Date:** 2026-06-15
**Status:** Design approved, pending spec review before drafting the plan

## Goal

Help the user genuinely understand "what runs underneath" when go-ecommerce-app uses AWS SQS — not
just read AI-generated code. Achieved by: (1) a narrative-style explainer following the journey of a
single message, and (2) building the **missing consumer/worker** so the user can run the full loop
`send → receive → process → delete` against **real AWS**.

Learner level: knows AWS basics (credentials, region, has used a few services) but hasn't grasped
the mechanics of SQS and message queues. So explain the SQS/queue mechanics **deeply**, and do
**not** re-teach the basic AWS account/credential material.

## Current code context (surveyed)

- `pkg/queue/sqs.go`: **producer only** — `PublishOrderEvent` → `SendMessage`. Defines
  `OrderEvent{EventType, OrderID, UserID, Amount}` and 3 event types: `ORDER_PLACED`, `ORDER_PAID`,
  `ORDER_SHIPPED` (the last one defined but not yet published).
- `internal/service/orderService.go`: publishes `ORDER_PLACED` in `PlaceOrder`, publishes
  `ORDER_PAID` in `HandleStripeEvent` (on `payment_intent.succeeded`). SQS is optional (nil-check);
  a publish error only logs rather than failing the request.
- `configs/appConfig.go`: initializes `SQSClient` from the env vars `AWS_REGION` +
  `AWS_SQS_ORDER_QUEUE_URL`; optional — order events are disabled if the env is missing.
- **No consumer yet** (`grep` for ReceiveMessage/DeleteMessage/worker/consumer = empty).
- `infra/` is empty → **no IaC**, AWS configured manually via Console/CLI.
- SDK: `aws-sdk-go v1.49.0` (SDK v1). Auth via the default credential chain.
- `pkg/notification`: the `NotificationClient` interface has `SendEmail(to, subject, body)` and
  `SendSMS(to, message)` — the worker will reuse these to process events.
- The Makefile follows a simple target pattern (`server:`, `dev:`, `build:`, ...).

**Core finding:** the user sees half the loop (sending) but not the receiving/processing half —
exactly where "what runs underneath" is most opaque.

## Chosen approach

**Approach A — "Follow a message".** A narrative explainer following the journey of one order; each
SQS concept is explained the moment it appears in the flow, tied to real code + a hands-on step.
(Rejected Approach B "theory first" and Approach C "minimal code-first" because they don't achieve
deep understanding.)

## Deliverables

Created in the repo (without breaking existing code):

```
cmd/worker/main.go              # entrypoint: load config, init consumer, run poll loop + graceful shutdown
internal/worker/consumer.go     # loop: ReceiveMessage → dispatch → DeleteMessage
internal/worker/handlers.go     # handle each EventType (send notification, update status)
internal/worker/handlers_test.go# unit test for dispatch + handlers (SQS mocked via an interface)
pkg/queue/sqs.go                # ADD ReceiveMessages() + DeleteMessage(), keep Publish unchanged
docs/learning/sqs-explained.md  # the Approach A explainer
Makefile                        # ADD a `worker:` target
```

Why split `cmd/worker` out: it reflects the real model — the API service (producer) and the worker
(consumer) are 2 independent processes, scaled/deployed separately. This is precisely the value of
SQS: the two sides don't call each other directly.

## Explainer outline (`docs/learning/sqs-explained.md`)

Each chapter: explanation (in Vietnamese, English terms kept as-is) → real code excerpt → a "🔍 Under
the hood" box explaining internals → (where applicable) a hands-on step.

1. **The big picture** — why a queue is needed; a diagram of the API service and worker not calling
   each other directly, with SQS in between; the problems it solves (decoupling, load handling, retry).
2. **The sending end (existing code)** — walk through `PlaceOrder` → `PublishOrderEvent` →
   `SendMessage`; message body, message attributes, where the queue URL comes from, the default
   credential chain.
3. **The message sits in the queue** — 🔍 how SQS stores a message; at-least-once delivery;
   visibility timeout (why a message "disappears" temporarily); standard vs FIFO; why duplicates can
   be received.
4. **The receiving end (we build it)** — long polling vs short polling; `ReceiveMessage` fetching a
   batch; process then mandatory `DeleteMessage`; what happens if you don't delete.
5. **When processing fails** — the message returns to the queue; retry; Dead Letter Queue;
   idempotency (the handler must tolerate running twice due to at-least-once).
6. **Real AWS config** — create the queue + DLQ; IAM least-privilege; set the env.
7. **Run it for real** — place an order → see the message in the queue → run the worker → watch it
   process + delete; observe the visibility timeout; deliberately make the handler fail to see retry
   and landing in the DLQ.

## Consumer design

### Additions to `pkg/queue/sqs.go`

```go
// ReceiveMessages: long polling, up to maxMessages (<=10) per call
func (q *SQSClient) ReceiveMessages(maxMessages int64, waitSeconds int64) ([]*sqs.Message, error)
// DeleteMessage: delete after processing completes, using the ReceiptHandle (NOT the message ID)
func (q *SQSClient) DeleteMessage(receiptHandle string) error
```
Teaching points: `WaitTimeSeconds` (long polling reduces cost + latency), `MaxNumberOfMessages`
(batch ≤10), why you delete by `ReceiptHandle` rather than `OrderID`. Also request
`MessageAttributeNames` to read back the `event_type` attribute the producer attached.

### `internal/worker/consumer.go` — the main loop

```
for {
    msgs = ReceiveMessages(10, 20)            // long poll 20s
    for each msg:
        event = json.Unmarshal(msg.Body)      // parse OrderEvent
        err = dispatch(event)                 // call the handler by EventType
        if err == nil:
            DeleteMessage(msg.ReceiptHandle)  // ONLY delete on success
        else:
            log + DON'T delete → returns after the visibility timeout → retry → (n times) → DLQ
}
```
Core decision: **delete the message after successful processing, not after receiving** — this is how
SQS guarantees no work is lost, and the reason at-least-once exists.

### `internal/worker/handlers.go` — dispatch by EventType

- `ORDER_PLACED` → send an order-confirmation email (via `NotificationClient`).
- `ORDER_PAID` → send a "paid" email / trigger fulfillment.
- Each handler takes a parsed `OrderEvent` and returns an `error`. On error → don't delete → retry.
- Unknown EventType → log a warning + delete (avoid getting stuck on a poison message forever) OR let
  it fall to the DLQ — decided in the plan; default: log + delete so the queue isn't blocked.

### Idempotency

Because of at-least-once, the handler must tolerate running twice. Illustrate a simple pattern: check
the order's status in the DB before acting (if it is already in the target state, skip). Note when a
dedicated `processed_events` table is needed (dedup by message/event ID).

### Graceful shutdown

Catch `SIGINT`/`SIGTERM`: stop receiving new messages, finish processing the current batch, then
exit. Teach why this is safe — a message not yet `DeleteMessage`-d automatically returns after the
visibility timeout.

### Makefile

Add a `worker:` target that runs `go run cmd/worker/main.go`, matching the style of `server:`.

## Real AWS config + IAM

1. **Create queues**: main queue `order-events` + DLQ `order-events-dlq`, with a redrive policy
   `maxReceiveCount=5`. Show both the Console and the `aws sqs create-queue` CLI.
2. **Queue URL** → set it into `AWS_SQS_ORDER_QUEUE_URL`.
3. **IAM least-privilege** — 2 separate policies:
   - API service (producer): only `sqs:SendMessage` on the main queue.
   - Worker (consumer): `sqs:ReceiveMessage`, `sqs:DeleteMessage`, `sqs:GetQueueAttributes`.
   - Explain why not to share one full-access credential.
4. **Local dev credentials**: profile/`~/.aws/credentials` or env, tying back to the default
   credential chain from chapter 2.
5. **Verification**: place an order via the API → `aws sqs receive-message` manually → run the worker
   → watch it process + disappear; deliberately fail the handler to observe retry + DLQ.

## Testing & error handling (Section 5)

- **Unit test** (`handlers_test.go`): separate the processing logic from SQS I/O. `dispatch` and the
  handlers take an `OrderEvent` + a mocked `NotificationClient` (fake/stub), asserting correct
  behavior (right email sent, error returned at the right time). The consumer loop relies on a small
  SQS interface so poll/delete can be tested with a fake, without real AWS.
- **A table of common errors** in the explainer: wrong/missing credentials, queue URL in the wrong
  region, a message body that won't parse (poison message), handler timeout > visibility timeout
  (duplicate processing), forgetting DeleteMessage (infinite reprocessing) — each with symptoms + how
  to recognize it.

## Out of scope (YAGNI)

- No IaC/Terraform (keep manual config as it currently is).
- No SDK upgrade to v2 (keep `aws-sdk-go v1` in sync with the rest of the repo).
- No real `processed_events` table — only explain when it's needed.
- No touching FIFO queues beyond explaining the concept (the real queue uses standard).

## Success criteria

- The user reads the explainer and can re-explain: what visibility timeout, at-least-once, long
  polling, DLQ, and idempotency are, and why the consumer deletes the message *after* processing.
- `make worker` runs, connects to real AWS, receives and processes `ORDER_PLACED`/`ORDER_PAID`, then
  deletes.
- Retry + landing in the DLQ can be observed when the handler deliberately fails.
- The unit tests for dispatch/handlers pass.
