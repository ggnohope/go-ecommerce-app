# SQS explained — following the journey of a single order

> A **narrative-style** learning document: we follow **one single order** as it travels through the system,
> from the moment the customer clicks "place order" until the worker finishes processing the message. Each SQS concept
> (queue, message, visibility timeout, long polling, at-least-once, DLQ, idempotency,
> ReceiptHandle...) is explained **right where it appears** in the flow.
>
> **Audience:** you already know AWS basics (credentials, regions, you've used a few services) but
> haven't yet grasped how SQS / message queues work. So here we dig deep into SQS and don't re-teach
> the basic account/credential material.
>
> Convention: explanations are in **English**, with technical terms kept in **English** (queue,
> message, visibility timeout...). The code excerpts are the **real code** in this repo, annotated with
> `file:line` so you can open them yourself to compare.

**Table of contents**

1. [The big picture — why we need a queue](#chapter-1--the-big-picture--why-we-need-a-queue)
2. [The producer side (already in place)](#chapter-2--the-producer-side-already-in-place)
3. [The message sitting in the queue](#chapter-3--the-message-sitting-in-the-queue)
4. [The consumer side (what we just built)](#chapter-4--the-consumer-side-what-we-just-built)
5. [When processing fails](#chapter-5--when-processing-fails)
6. [Real AWS config + IAM](#chapter-6--real-aws-config--iam)
7. [Running it for real, end-to-end](#chapter-7--running-it-for-real-end-to-end)

---

## Chapter 1 — The big picture — why we need a queue

When a customer places an order, the system needs to do a few *secondary* tasks: send a confirmation email, and
once payment goes through, update the status and send a "payment received" email. The core question:
**should the API service do all of that work itself, right inside the request handler?**

If we do it directly (calling synchronously), we hit three problems:

- **Tight coupling:** the API has to know about the mail service and wait for it to finish. If the mail
  service dies → the order request dies along with it.
- **Poor load handling:** in a flash sale of 10,000 orders/second, if each order waits for the email to be sent before returning a
  response → the API collapses.
- **Poor retries:** what happens if the email fails midway? Writing retry logic by hand inside the
  request handler is messy and easy to drop work.

The solution: put a **queue** in the middle. The API simply **drops a message into the queue**
and returns the response immediately. A separate process — the **worker** — picks the message up and processes it later, at
its own pace.

```
   ┌──────────────┐                                  ┌──────────────┐
   │  API service │                                  │    Worker    │
   │  (producer)  │                                  │  (consumer)  │
   └──────┬───────┘                                  └──────▲───────┘
          │                                                 │
          │  SendMessage                      ReceiveMessage│
          │  (put in)                              (pick up) │
          ▼                                                 │
       ┌─────────────────────────────────────────────────────┐
       │                  [  SQS queue  ]                      │
       │         message · message · message · ...             │
       └───────────────────────────────────────────────────────┘
```

**The key point:** the API service and the worker **do NOT call each other directly**. They don't even
know the other exists. They only share one thing: **the queue's address** (queue URL).
That is exactly **decoupling**:

- The API can return a response to the customer right after dropping the message into the queue, without waiting for
  the email to be sent.
- The worker can die, restart, or be redeployed — the message still sits safely in the queue, waiting. No work is lost.
- A sudden load spike → messages pile up in the queue, the worker processes them gradually. The queue acts as a
  **buffer** that absorbs load.
- A worker processing error → the message is **redelivered** to be retried later. Retry is a built-in
  feature of SQS, not something you code yourself.

> 🔍 **Under the hood:** SQS (Simple Queue Service) is a **fully AWS-managed** queue.
> You don't stand up a server, you don't worry about HA, you don't worry about storage — you just call APIs over the network:
> `SendMessage`, `ReceiveMessage`, `DeleteMessage`. Every message lives in AWS's distributed
> storage and is retained by default for up to **14 days** if no one processes it. In this repo,
> the "API service" is the `make server` process, and the "worker" is the `make worker` process —
> two separate processes that can be deployed and scaled independently.

In the following chapters, we follow **one specific order**: chapter 2 is when the API drops the message in,
chapter 3 is the message waiting, chapters 4–5 are the worker picking it up and processing it (including failures).

---

## Chapter 2 — The producer side (already in place)

The producer side is the part that was already in the repo beforehand. The journey begins when the customer
calls `POST /user/me/order`. In `orderService.PlaceOrder`, after creating the order in the DB and
clearing the cart, the code drops a message into the queue:

```go
// internal/service/orderService.go:77-87
	if s.sqsClient != nil {
		ev := queue.OrderEvent{
			EventType: queue.EventOrderPlaced,
			OrderID:   order.ID,
			UserID:    userID,
			Amount:    order.TotalAmount,
		}
		if err = s.sqsClient.PublishOrderEvent(ev); err != nil {
			log.Printf("order: sqs publish failed order=%d err=%v", order.ID, err)
		}
	}
```

A few things worth noting right here:

- `if s.sqsClient != nil` — SQS is **optional**. If the queue URL env is not configured, the client is
  `nil`, this block is skipped, and ordering still works normally. Order events are merely an
  enhancement feature.
- A publish failure only logs via **`log.Printf`** and does **not** return an error to the customer. The philosophy: emitting the event
  is secondary and must not break the main ordering flow. (The trade-off: if the publish fails, that event
  is lost — acceptable for this kind of event.)

A second event, `ORDER_PAID`, is published elsewhere — when Stripe reports a successful payment
via webhook. Importantly: the API here **only** marks the order as `paid`
(`payment_status=paid`) and then publishes `ORDER_PAID`. It does **not** transition the order to
`confirmed` itself. The transition to `confirmed` (and sending the "payment received" email) belongs to the **worker** —
see chapter 5. The API only marks the order *paid* and publishes the event; the *worker*
owns the transition to `confirmed`.

```go
// internal/service/orderService.go:149-165
	case "payment_intent.succeeded":
		orderID, err := payment.ExtractOrderID(event)
		if err != nil {
			log.Printf("stripe webhook: %v", err)
			return nil
		}
		if s.sqsClient != nil {
			if sqsErr := s.sqsClient.PublishOrderEvent(queue.OrderEvent{
				EventType: queue.EventOrderPaid,
				OrderID:   orderID,
			}); sqsErr != nil {
				log.Printf("stripe webhook: sqs publish failed order=%d err=%v", orderID, sqsErr)
			}
		}
		return s.orderRepo.UpdateOrder(orderID, map[string]interface{}{
			"payment_status": domain.PaymentStatusPaid,
		})
```

Notice the same `if s.sqsClient != nil` pattern as in `PlaceOrder`: SQS is optional; if it's not
configured, the publish is skipped but the order is still marked `paid`.

So what does `PublishOrderEvent` actually do? Here is the whole function:

```go
// pkg/queue/sqs.go:45-64
func (q *SQSClient) PublishOrderEvent(event OrderEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("sqs: marshal error: %w", err)
	}
	_, err = q.client.SendMessage(&sqs.SendMessageInput{
		QueueUrl:    aws.String(q.queueURL),
		MessageBody: aws.String(string(body)),
		MessageAttributes: map[string]*sqs.MessageAttributeValue{
			"event_type": {
				DataType:    aws.String("String"),
				StringValue: aws.String(string(event.EventType)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("sqs: send failed: %w", err)
	}
	return nil
}
```

Let's break down each part of a **message**:

- **Message body** — the main payload. Here it's an `OrderEvent` that's been `json.Marshal`ed into
  a JSON string. The `OrderEvent` struct:

  ```go
  // pkg/queue/sqs.go:20-25
  type OrderEvent struct {
  	EventType EventType `json:"event_type"`
  	OrderID   uint      `json:"order_id"`
  	UserID    uint      `json:"user_id"`
  	Amount    float64   `json:"amount"`
  }
  ```

  So the message body above looks like: `{"event_type":"ORDER_PLACED","order_id":42,"user_id":7,"amount":59.9}`.
  SQS treats the body as just **a text string** — it doesn't understand JSON and doesn't look inside. Parsing the
  JSON is the receiver's responsibility (chapter 4).

- **The `event_type` message attribute** — metadata attached alongside, **separate** from the body. Why
  duplicate `event_type` (which is already in the body)? Because the attribute can be read **without
  parsing the body** — useful if you later want to filter/route messages by type (e.g. an SNS
  filter policy) without opening the body. In this repo the worker reads from the body for simplicity,
  but the attribute is still attached so it's ready for those use cases.

- **`QueueUrl`** — which queue the message goes into. The value `q.queueURL` comes from an **environment variable**.
  See where it's loaded:

  ```go
  // configs/appConfig.go:77-85
  var sqsClient *queue.SQSClient
  if queueURL := os.Getenv("AWS_SQS_ORDER_QUEUE_URL"); queueURL != "" {
  	sqsClient, err = queue.NewSQSClient(awsRegion, queueURL)
  	if err != nil {
  		log.Printf("WARNING: SQS client init failed: %v — order events disabled", err)
  	}
  } else {
  	log.Println("WARNING: AWS_SQS_ORDER_QUEUE_URL not set — order events disabled")
  }
  ```

  Two env vars determine the client: **`AWS_REGION`** (which region the queue is in) and
  **`AWS_SQS_ORDER_QUEUE_URL`** (the queue's full URL). A missing URL → a `nil` client → exactly the
  `if s.sqsClient != nil` we saw above.

> 🔍 **Under the hood — where do the credentials come from?** Notice the client constructor does **not** pass an access key
> anywhere:
>
> ```go
> // pkg/queue/sqs.go:32-43
> func NewSQSClient(region, queueURL string) (*SQSClient, error) {
> 	sess, err := session.NewSession(&aws.Config{
> 		Region: aws.String(region),
> 	})
> 	...
> }
> ```
>
> It only passes `Region`. The AWS SDK finds credentials on its own via the **default credential chain**, in
> this order:
>
> 1. **Environment variables** — `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` (and
>    `AWS_SESSION_TOKEN` if they're temporary).
> 2. **Shared config file** — `~/.aws/credentials` / `~/.aws/config` (pick the profile via
>    `AWS_PROFILE`). This is the common approach when developing locally.
> 3. **IAM role** — when running on EC2 / ECS / Lambda, the SDK automatically pulls temporary credentials from
>    the metadata of the role attached to the machine. This is the standard approach in production: **no static key
>    lives in code or env**.
>
> In other words: locally you keep keys in `~/.aws/credentials`; in production you attach an IAM role —
> **the code doesn't change a single line**. The region always has to be stated explicitly because a queue URL is tied to a
> specific region.

At this point the order's message has left the API and is sitting in the queue. The next chapter looks at how it "lives"
while waiting.

---

## Chapter 3 — The message sitting in the queue

Now the order's message is sitting in the queue, waiting for someone to pick it up. This is where you need to understand
SQS's **hidden mechanics** — they determine how we have to write the worker in the next chapter.

> 🔍 **Under the hood — at-least-once delivery**
>
> An SQS standard queue guarantees each message is delivered **at least once**, and does **not** guarantee
> *exactly* once. Meaning: normally each message is delivered once, but in certain
> situations (flaky network, the message being replicated across SQS's multiple storage servers) you
> may receive **the same message twice**.
>
> This isn't a bug — it's the trade-off for achieving extremely high throughput and availability. The direct
> consequence: **the worker must tolerate processing duplicates** without harm. This concept is called
> **idempotency**, which we'll handle in chapter 5.

> 🔍 **Under the hood — visibility timeout (why a message "disappears" temporarily)**
>
> When the worker calls `ReceiveMessage` and picks up a message, that message is **not deleted from the
> queue**. Instead it is **hidden** for a period called the **visibility
> timeout** (default 30 seconds). During this period, other consumers calling `ReceiveMessage`
> will **not see** that message.
>
> The purpose: prevent two workers from processing the same message in parallel. While it's being processed, the message is
> "invisible" to everyone else.
>
> There are two outcomes:
>
> - The worker **finishes** processing and calls `DeleteMessage` → the message disappears permanently. Done.
> - The worker has **not** deleted it (still processing for a long time, or it crashed) and the visibility timeout **expires**
>   → the message **reappears** in the queue, ready to be delivered on the next pickup. This is exactly the
>   **automatic retry** mechanism.
>
> ⚠️ This is a classic trap: if your handler takes **longer** than the visibility timeout,
> the message will reappear **mid-processing** while you're still working on it → another worker picks it up
> and processes it **again** → a duplicate. Either set the visibility timeout safely longer than the maximum
> processing time, or design idempotent processing (ideally both).

> 🔍 **Under the hood — standard vs FIFO queue**
>
> SQS has two queue types:
>
> | | **Standard** | **FIFO** |
> |---|---|---|
> | Ordering | Not guaranteed | Strictly ordered (First-In-First-Out) |
> | Delivery | At-least-once (may duplicate) | Exactly-once (dedup within a 5-minute window) |
> | Throughput | Nearly unlimited | Limited (300–3000 msg/s) |
> | Queue name | any | must end in `.fifo` |
>
> This repo uses a **standard queue** — simple, high throughput, and order events don't
> require strict ordering. The price is that *duplicates are possible* and *order may be skewed*
> → we handle that with idempotency. FIFO is only needed when ordering is strictly required (e.g.
> the steps of a financial transaction that must follow an exact sequence).

**Why duplicates can happen — three sources in short:**

1. **At-least-once** of a standard queue: by nature, SQS may redeliver.
2. **Visibility timeout expiring mid-processing**: the handler is slower than the timeout → the message reappears
   while still being processed.
3. **Crash after processing, before `DeleteMessage`**: the worker finishes the work (already sent the email) and then
   dies right before it gets to delete → the message reappears → next time it's processed from scratch.

All three lead to the same conclusion: **don't assume each message runs exactly once.**
This is why chapter 5 has to talk about idempotency. Now on to chapter 4 — the worker picking up the message.

---

## Chapter 4 — The consumer side (what we just built)

This is the half of the loop that the repo **was missing** before and that we just built: the **consumer**. Its heart
is the `processOnce` function — receive a batch of messages, process each one, delete whichever ones succeed.

```go
// internal/worker/consumer.go:62-90
func (c *Consumer) processOnce() error {
	msgs, err := c.queue.ReceiveMessages(10, 20)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		if m.Body == nil {
			// Defensive: a message without a body should never happen, but
			// dereferencing a nil Body would panic and kill the worker.
			slog.Error("worker: message has nil body, skipping")
			continue
		}
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
```

And the `ReceiveMessages` function it calls:

```go
// pkg/queue/sqs.go:69-80
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
```

Note the call `c.queue.ReceiveMessages(10, 20)` — `maxMessages=10`, `waitSeconds=20`.

**`waitSeconds=20` → long polling.** This is an important concept:

> 🔍 **Under the hood — long polling vs short polling**
>
> - **Short polling** (`WaitTimeSeconds=0`): `ReceiveMessage` returns **immediately**. If the
>   queue is empty → it returns an empty list right away. The worker has to call repeatedly → it **burns a lot
>   of API calls** (each call costs money) and most return empty. Also, because SQS storage is
>   distributed, short polling only queries *a subset* of the servers → it sometimes reports "empty" even when there
>   actually is a message.
> - **Long polling** (`WaitTimeSeconds=1..20`): if the queue is empty, SQS **keeps the connection open** for up to
>   `waitSeconds` seconds, **waiting** for a message to arrive and returning right away when one does. Only when the wait time elapses with the queue still
>   empty does it return an empty list. Benefits: **fewer API calls** (less cost), **lower latency**
>   (a message arriving is received almost instantly), and it scans every storage server so nothing is missed.
>
> 20 seconds is the **maximum value** SQS allows. Nearly every production consumer should use
> long polling.

**`maxMessages=10` → batch.** A single `ReceiveMessage` fetches **up to 10 messages** (this is
SQS's hard limit). Fetching in batches reduces network round-trips. The `for _, m := range msgs` loop
processes each message in the batch.

**The golden rule: only `DeleteMessage` AFTER the handler succeeds.** Look closely at the ordering in
`processOnce`:

- `dispatch(event)` returns an error → `continue`, **do NOT delete**. The message stays in the queue, and once the visibility
  timeout expires it reappears to be retried (chapter 5).
- `dispatch(event)` returns `nil` (success) → only then does `c.deleteQuietly(*m.ReceiptHandle)` delete it.

This is the core of the entire SQS design: **a message only disappears once the work is done**.
Deleting *before* processing (deleting right after receiving) is a fatal mistake — if the worker crashes mid-way,
that work is **lost forever**, and no one knows. Deleting *after* processing means the worst case
is just *doing it again* (thanks to idempotency), never *losing work*.

**Delete by `ReceiptHandle`, NOT by `order_id` or the message ID.** Notice we pass
`*m.ReceiptHandle`:

```go
// internal/worker/consumer.go:92-96
func (c *Consumer) deleteQuietly(receiptHandle string) {
	if err := c.queue.DeleteMessage(receiptHandle); err != nil {
		slog.Error("worker: delete failed (message may be redelivered)", "err", err)
	}
}
```

> 🔍 **Under the hood — what is a ReceiptHandle?** Each message has a fixed **message ID** (the message's
> identifier), but to **delete** it SQS requires the **ReceiptHandle** — a token **issued only
> for this particular receive**. Each time `ReceiveMessage` returns the same message (after redelivery), it
> issues a **new, different** ReceiptHandle. You must delete with the ReceiptHandle of the **current
> receive**; using an old one will fail. This is how SQS confirms "I'm deleting exactly the message I
> just held in my hand," rather than by `order_id` (which is in the body, and SQS doesn't care about it).

**Two special cases in the real code:**

1. **Poison message (body that won't parse)** — if `json.Unmarshal` fails, this message will
   **never** parse, so retrying infinitely is pointless and would **block** the queue. The code
   chooses to **delete it outright** (with a log) so it doesn't get stuck. This is a deliberate decision — if you want
   to keep it for investigation, you'd let it fall to the DLQ instead of deleting it.

2. **Receive error backoff** — if `ReceiveMessages` itself errors (wrong credentials, network loss,
   the queue was deleted...), `processOnce` returns an error and `Run` **waits a beat** before retrying, to avoid
   hot-spinning (a busy-loop) that burns CPU:

   ```go
   // internal/worker/consumer.go:36-57
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
   				// Back off before retrying so a persistent receive error
   				// doesn't hot-spin. Stay responsive to shutdown.
   				select {
   				case <-ctx.Done():
   					slog.Info("worker: shutdown signal received during backoff, stopping")
   					return
   				case <-time.After(receiveErrorBackoff):
   				}
   			}
   		}
   	}
   }
   ```

   `receiveErrorBackoff` is 5 seconds (`internal/worker/consumer.go:17`). Notice the whole `Run` loop
   respects `ctx.Done()` — that's **graceful shutdown**: when it receives SIGINT/SIGTERM, the
   worker stops picking up new batches without interrupting the work in progress. Any message not yet
   `DeleteMessage`d will reappear after the visibility timeout — safe thanks to the golden rule
   above.

---

## Chapter 5 — When processing fails

This is where at-least-once and the visibility timeout (chapter 3) meet and become a real
**retry + DLQ** mechanism. Let's follow our order when things *don't* go smoothly.

**The lifecycle of a failed message:**

```
   receive #1 ─▶ handler error ─▶ NOT deleted ─▶ hidden (visibility timeout) ─┐
       ▲                                                                       │
       └────────── reappears after timeout ◀────────────────────────────────────┘
   ... repeats up to maxReceiveCount times ...
   receive #(maxReceiveCount+1) ─▶ SQS moves it to ─▶ [ DLQ ]
```

- Handler returns an error → `processOnce` **does not delete** → the message stays.
- The **visibility timeout** expires → the message **reappears** → the worker picks it up again → **retry**.
- And so it repeats. SQS counts how many times a message has been received (`ApproximateReceiveCount`).
- When it exceeds **`maxReceiveCount`** (we set it to 5 in chapter 6), SQS automatically **moves the message to a
  Dead Letter Queue (DLQ)** — a separate queue for "poison" messages, instead of letting them
  retry forever.

> 🔍 **Under the hood — Dead Letter Queue (DLQ)** is an ordinary queue, but attached as a "controlled
> trash bin" for the main queue via the **RedrivePolicy**. When a message fails more than
> `maxReceiveCount` times, SQS automatically pushes it to the DLQ. Benefits: (1) the main queue doesn't get **stuck**
> by a broken message that keeps retrying forever; (2) you have a place to **investigate** the failed
> messages (open the DLQ and inspect the body, the log) without losing them; (3) after fixing the bug, you can **redrive**
> (push back) the message from the DLQ to the main queue to reprocess it.

**The worker owns the transition to `confirmed`.** Recall from the producer chapter: the API webhook only sets
`payment_status=paid` and then publishes `ORDER_PAID`. It's the **worker** that transitions the order from `paid`
to `confirmed` and sends the "payment received" email. The worker — not the API — owns the
`confirmed` transition.

**Idempotency — why and how.** Because of at-least-once, a message may run twice.
For `ORDER_PAID`, running twice carelessly = updating the status twice, sending two "payment
received" emails. The code prevents that with a simple **idempotency guard**: check the status
before acting. Since the API no longer sets `confirmed` itself, this guard does real
work, not decoration: on the first delivery the order isn't yet `confirmed` → the worker confirms +
sends the email; on a redelivered duplicate it sees it's already `confirmed` → it safely skips.

```go
// internal/worker/handlers.go:58-79
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

The key: `if order.Status == domain.OrderStatusConfirmed { ... return nil }`. The first time it processes,
it sets the status to `confirmed`. If the message arrives a second time, the order is **already** `confirmed` →
the handler skips, returns `nil` (treated as success) → the message is deleted. No duplicate update,
no duplicate email. **The state in the DB is itself the ledger recording "has this work been done yet."**

Compare with `handleOrderPlaced` — this handler **deliberately does NOT** have a guard:

```go
// internal/worker/handlers.go:47-49
	// No idempotency guard here: SQS is at-least-once, so a redelivered
	// ORDER_PLACED may send a duplicate confirmation email. That's an
	// accepted trade-off — a duplicate confirmation is benign.
```

The lesson: **idempotency is a choice made per type of side effect**. Sending *two* "order
received" confirmation emails is annoying but harmless → accepted. But updating the status / deducting money
twice is dangerous → it must have a guard. You weigh it by the *severity of harm from running twice*.

> 🔍 **Under the hood — when do you need a `processed_events` table?** The "check the target state"
> approach above only works when the side effect makes the state **become stable** (once confirmed,
> the next run knows to skip). But some side effects **leave no clear state
> trace** — e.g. "add 10 loyalty points on each payment": running twice gives 20
> points, and looking at the "total points" you can't tell whether this event has already been credited.
>
> In that case you need a **`processed_events`** table that records **the ID of every message/event already processed**.
> Before acting: if the event ID is already in the table → skip; if not → process and then *write the ID
> into the table in the same transaction*. This is general dedup by message ID, independent
> of business state. This repo doesn't need it yet (it only uses a state-based guard) — it's
> mentioned so you know when you'd have to level up.

### Table of common errors

| Error | Symptom / how to spot it |
|---|---|
| **Wrong/missing credentials** | `ReceiveMessages` errors continuously; the log `worker: receive failed, will retry` repeats steadily every ~5s (due to backoff). Usually accompanied by an AWS message like `AccessDenied`/`InvalidClientTokenId`/`NoCredentialProviders`. Check `~/.aws/credentials` or the `AWS_ACCESS_KEY_ID` env. |
| **Queue URL in the wrong region** | An error like `AWS.SimpleQueueService.NonExistentQueue` or `QueueDoesNotExist` — because `AWS_REGION` points to region A but the queue URL belongs to region B. Cross-check the region in the queue URL against `AWS_REGION`. |
| **Poison message (body that won't parse)** | The log `worker: unparseable message body, discarding` along with the printed body. The code deletes it on its own so it does *not* clog the queue, but if you see many → some other producer is sending malformed bodies into the queue. |
| **Handler slower than the visibility timeout** | The same `order_id` appears in the log **twice** (duplicate processing) even though the handler "seems" to run correctly; the email is sent twice. Cause: the message reappeared mid-processing. Increase the queue's visibility timeout, or make sure the handler is idempotent. |
| **Forgetting `DeleteMessage` (delete after processing)** | The same message is processed **infinitely**, the success log repeats over and over, and it eventually falls to the DLQ even though no error occurred. Cause: the success branch doesn't call delete. In this repo the success branch always calls `deleteQuietly` — this bug appears when someone gets the ordering wrong. |

---

## Chapter 6 — Real AWS config + IAM

By now you understand the mechanics. Now let's stand up the real infrastructure on AWS. This repo **does not use IaC** —
configuration is done manually with the AWS CLI (or the Console). The **mandatory** order: create the **DLQ first**, because
the main queue needs the DLQ's ARN to declare its RedrivePolicy.

```bash
# Step 1 — Create the DLQ FIRST (the main queue will point to it)
aws sqs create-queue --queue-name order-events-dlq

# Step 2 — Get the DLQ's ARN (needed for the RedrivePolicy in step 3)
#   Replace <DLQ_URL> with the QueueUrl that step 1 printed.
aws sqs get-queue-attributes --queue-url <DLQ_URL> \
  --attribute-names QueueArn

# Step 3 — Create the main queue `order-events`, pointing redrive to the DLQ, max 5 receives
#   Replace <DLQ_ARN> with the QueueArn value that step 2 printed.
aws sqs create-queue --queue-name order-events \
  --attributes '{"RedrivePolicy":"{\"deadLetterTargetArn\":\"<DLQ_ARN>\",\"maxReceiveCount\":\"5\"}"}'
```

Explaining the `RedrivePolicy`: `maxReceiveCount=5` means that a message received more than 5 times
(i.e. processed with an error 5 times) gets pushed by SQS to the `deadLetterTargetArn` (DLQ). This matches exactly the
DLQ mechanism in chapter 5.

Once created, take the **QueueUrl of `order-events`** (not the DLQ's) and put it in the env:

```bash
export AWS_REGION=ap-southeast-1
export AWS_SQS_ORDER_QUEUE_URL=https://sqs.ap-southeast-1.amazonaws.com/<account-id>/order-events
```

These two env vars connect straight back to `configs/appConfig.go:77-85` (chapter 2): `AWS_REGION` tells which
region the queue is in, `AWS_SQS_ORDER_QUEUE_URL` tells the exact URL. Both the API service and the
worker read the **same** two env vars — that's how the two processes "meet" at the same queue
without calling each other directly (chapter 1).

### IAM least-privilege — two separate policies

The API service (producer) and the worker (consumer) need **different permissions**. Don't grant them a single shared
full-access credential. Create **two** policies, attached to two separate identities:

```json
// API service (producer) — may ONLY send messages
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["sqs:SendMessage"],
      "Resource": "arn:aws:sqs:ap-southeast-1:<account-id>:order-events"
    }
  ]
}
```

```json
// Worker (consumer) — may receive, delete, and view queue attributes
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "sqs:ReceiveMessage",
        "sqs:DeleteMessage",
        "sqs:GetQueueAttributes"
      ],
      "Resource": "arn:aws:sqs:ap-southeast-1:<account-id>:order-events"
    }
  ]
}
```

**Why not use one shared full-access credential?** — the principle of **least privilege**:

- **Limit the damage from a leak.** If the API service's credential is leaked, the attacker can *only*
  stuff messages into the queue — they **cannot** delete, and **cannot** snoop on other people's
  messages. If it were a full-access credential, they could do everything.
- **Reflect the actual role.** A producer has no reason to `DeleteMessage`; a consumer has
  no reason to `SendMessage`. Splitting permissions helps ensure a bug on one side doesn't accidentally break the other.
- **Easy to audit.** Looking at the policy tells you exactly what each service is allowed to do.

Note: the worker needs `sqs:GetQueueAttributes` (in addition to Receive/Delete) because some operations and
operational tools need to read queue attributes. It does **not** need any permission on the DLQ — it's SQS
(not the worker) that pushes messages to the DLQ.

---

## Chapter 7 — Running it for real, end-to-end

A verification scenario in the **exact order**. You'll need: the queue created (chapter 6), the env set, and the Postgres
DB running (`docker-compose up -d`).

### Step 1 — Start the API and place an order (terminal A)

```bash
make server
```

Then place an order via the API (log in to get a token, add to cart, then call):

```bash
curl -X POST http://localhost:9000/user/me/order \
  -H "Authorization: Bearer <JWT>" \
  -H "Content-Type: application/json" \
  -d '{"shipping_address":"123 Le Loi, Q1"}'
```

At this point `PlaceOrder` has published an `ORDER_PLACED` message to the queue (chapter 2). The worker is **not yet**
running, so the message is still sitting in the queue.

### Step 2 — See the message in the queue with your own eyes (manually)

```bash
aws sqs receive-message --queue-url "$AWS_SQS_ORDER_QUEUE_URL" \
  --message-attribute-names All \
  --wait-time-seconds 5
```

You'll see JSON containing `Body` (the string `{"event_type":"ORDER_PLACED",...}`), a
`MessageId`, a `ReceiptHandle` (the token to delete — chapter 4), and `MessageAttributes` with
`event_type`. This is exactly what the worker will pick up.

> ⚠️ This command **has received** the message → it's now in the **visibility timeout** (hidden ~30s). Don't
> delete it. Wait past the timeout, the message **reappears**, and the worker in step 3 will pick it up. (You've just
> observed the visibility timeout from chapter 3 with your own hands.)

### Step 3 — Start the worker, watch it process and the message disappear (terminal B)

```bash
make worker
```

Watch the log: `worker: started, polling for messages`, then
`worker: order confirmation sent order=<id>`. The worker picked up the message, called
`handleOrderPlaced`, sent the confirmation email, and `DeleteMessage`d it. Verify by running
the `receive-message` command from step 2 again — the queue is **empty**. The message has completed its lifecycle:
send → receive → process → delete.

### Step 4 — Deliberately cause a failure to observe retry + falling to the DLQ

Temporarily edit the top of `handleOrderPlaced` to **always return an error** (remember `import "errors"`):

```go
// internal/worker/handlers.go — handleOrderPlaced, first line of the function (FOR TESTING ONLY)
func (d *Dispatcher) handleOrderPlaced(event queue.OrderEvent) error {
	return errors.New("test: simulate handler failure")
	// ... the rest is temporarily unreachable
}
```

Place a new order, then `make worker`. Observe:

- The log repeats `worker: handler failed, leaving message for retry ... type=ORDER_PLACED`.
- The message is **not** deleted → after each visibility timeout it reappears → the worker retries.
- After **5 times** (`maxReceiveCount=5`, chapter 6), SQS pushes the message to **`order-events-dlq`**.

Check that the DLQ has the message:

```bash
# Get the number of messages currently in the DLQ
aws sqs get-queue-attributes --queue-url <DLQ_URL> \
  --attribute-names ApproximateNumberOfMessages
```

Or open the **SQS Console** → the `order-events-dlq` queue → *Send and receive messages* → *Poll
for messages* to see that exact failed message sitting there. **Remember to revert** the `return errors.New(...)`
snippet after you're done testing.

---

That's it. You've now traversed the full lifecycle of a message: the API drops it in (chapter 2) → it waits with
visibility timeout & at-least-once (chapter 3) → the worker long-polls, picks it up, processes, and deletes at the right
moment (chapter 4) → on failure it retries and falls to the DLQ, on duplicates idempotency guards against it (chapter 5) → on
real AWS infrastructure with IAM permission separation (chapter 6) → and you verify it yourself (chapter 7). That's
"what runs under the hood" of SQS in this repo.
</content>
</invoke>
