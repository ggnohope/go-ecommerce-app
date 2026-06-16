# SQS giải thích — đi theo hành trình một order

> Tài liệu học theo kiểu **kể chuyện**: ta bám theo **một order duy nhất** đi qua hệ thống,
> từ lúc khách bấm "đặt hàng" cho tới lúc worker xử lý xong message. Mỗi khái niệm SQS
> (queue, message, visibility timeout, long polling, at-least-once, DLQ, idempotency,
> ReceiptHandle...) được giải thích **ngay tại chỗ nó xuất hiện** trong dòng chảy.
>
> **Đối tượng:** bạn đã biết AWS cơ bản (credential, region, đã dùng vài service) nhưng
> chưa nắm cơ chế SQS / message queue. Nên ở đây ta đào sâu phần SQS, không giảng lại
> phần account/credential cơ bản.
>
> Quy ước: giải thích bằng **tiếng Việt**, giữ nguyên thuật ngữ **tiếng Anh** (queue,
> message, visibility timeout...). Code trích là **code thật** trong repo này, có kèm
> `file:dòng` để bạn tự mở ra đối chiếu.

**Mục lục**

1. [Bức tranh lớn — vì sao cần queue](#chương-1--bức-tranh-lớn--vì-sao-cần-queue)
2. [Đầu gửi (đã có sẵn)](#chương-2--đầu-gửi-đã-có-sẵn)
3. [Message nằm trong queue](#chương-3--message-nằm-trong-queue)
4. [Đầu nhận (phần ta vừa xây)](#chương-4--đầu-nhận-phần-ta-vừa-xây)
5. [Khi xử lý lỗi](#chương-5--khi-xử-lý-lỗi)
6. [Config AWS thật + IAM](#chương-6--config-aws-thật--iam)
7. [Chạy thật end-to-end](#chương-7--chạy-thật-end-to-end)

---

## Chương 1 — Bức tranh lớn — vì sao cần queue

Khi khách đặt một order, hệ thống cần làm vài việc *phụ*: gửi email xác nhận, sau khi
thanh toán thì cập nhật trạng thái và gửi email "đã nhận tiền". Câu hỏi cốt lõi:
**API service có nên tự làm hết những việc đó ngay trong lúc xử lý request không?**

Nếu làm trực tiếp (gọi thẳng), ta gặp ba vấn đề:

- **Coupling chặt:** API phải biết về dịch vụ gửi mail, phải chờ nó xong. Service mail
  chết → request đặt hàng cũng chết theo.
- **Chịu tải kém:** flash sale 10.000 đơn/giây, mỗi đơn lại chờ gửi mail xong mới trả
  response → API sập.
- **Không retry tốt:** mail gửi lỗi giữa chừng thì sao? Tự viết lại logic retry trong
  request handler rất rối và dễ mất việc.

Giải pháp: đặt một **queue** (hàng đợi) ở giữa. API chỉ việc **bỏ một message vào queue**
rồi trả response ngay. Một process khác — **worker** — nhặt message ra và xử lý sau, theo
nhịp của riêng nó.

```
   ┌──────────────┐                                  ┌──────────────┐
   │  API service │                                  │    Worker    │
   │  (producer)  │                                  │  (consumer)  │
   └──────┬───────┘                                  └──────▲───────┘
          │                                                 │
          │  SendMessage                      ReceiveMessage│
          │  (bỏ vào)                              (nhặt ra) │
          ▼                                                 │
       ┌─────────────────────────────────────────────────────┐
       │                  [  SQS queue  ]                      │
       │         message · message · message · ...             │
       └───────────────────────────────────────────────────────┘
```

**Điểm mấu chốt:** API service và worker **KHÔNG gọi trực tiếp nhau**. Chúng thậm chí
không biết nhau tồn tại. Chúng chỉ cùng biết một thứ: **địa chỉ của queue** (queue URL).
Đó chính là **decoupling**:

- API có thể trả response cho khách ngay sau khi bỏ message vào queue, không phải chờ
  email gửi xong.
- Worker chết, restart, deploy lại — message vẫn nằm yên trong queue chờ. Không mất việc.
- Tải tăng đột biến → message dồn trong queue, worker xử lý dần. Queue đóng vai trò
  **buffer** chịu tải.
- Worker xử lý lỗi → message được **giao lại** (redeliver) để thử lần sau. Retry là tính
  năng có sẵn của SQS, không phải tự code.

> 🔍 **Bên dưới:** SQS (Simple Queue Service) là một queue **được AWS quản lý hoàn toàn**.
> Bạn không dựng server, không lo HA, không lo lưu trữ — chỉ gọi API qua mạng:
> `SendMessage`, `ReceiveMessage`, `DeleteMessage`. Mọi message nằm trong vùng lưu trữ
> phân tán của AWS, mặc định giữ tối đa **14 ngày** nếu chưa ai xử lý. Trong repo này,
> "API service" là tiến trình `make server`, còn "worker" là tiến trình `make worker` —
> hai process riêng biệt, có thể deploy và scale độc lập.

Trong các chương sau, ta đi theo **một order cụ thể**: chương 2 là lúc API bỏ message vào,
chương 3 là lúc message nằm chờ, chương 4–5 là lúc worker nhặt ra và xử lý (kể cả lỗi).

---

## Chương 2 — Đầu gửi (đã có sẵn)

Đầu gửi (**producer**) là phần đã có sẵn trong repo từ trước. Hành trình bắt đầu khi khách
gọi `POST /user/me/order`. Trong `orderService.PlaceOrder`, sau khi tạo order trong DB và
dọn giỏ hàng, code bỏ một message vào queue:

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

Vài điều đáng chú ý ngay ở đây:

- `if s.sqsClient != nil` — SQS là **optional**. Nếu không cấu hình env queue URL, client
  bằng `nil`, đoạn này bị bỏ qua, đặt hàng vẫn chạy bình thường. Order events chỉ là tính
  năng tăng cường.
- Publish lỗi chỉ **`log.Printf`**, **không** trả lỗi cho khách. Triết lý: việc gửi event
  là phụ, không được làm hỏng luồng đặt hàng chính. (Đánh đổi: nếu publish lỗi thì event
  đó mất — chấp nhận được với loại event này.)

Một event thứ hai, `ORDER_PAID`, được publish ở chỗ khác — khi Stripe báo thanh toán
thành công qua webhook. Quan trọng: API ở đây **chỉ** đánh dấu order là `paid`
(`payment_status=paid`) rồi publish `ORDER_PAID`. Nó **không** tự chuyển order sang
`confirmed`. Việc chuyển sang `confirmed` (và gửi email "đã nhận tiền") thuộc về **worker** —
xem chương 5. The API only marks the order *paid* and publishes the event; the *worker*
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

Để ý cùng pattern `if s.sqsClient != nil` như ở `PlaceOrder`: SQS optional, không cấu
hình thì bỏ qua publish nhưng order vẫn được đánh dấu `paid`.

Vậy `PublishOrderEvent` thực sự làm gì? Đây là toàn bộ hàm:

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

Phân tích từng phần của một **message**:

- **Message body** — phần dữ liệu chính. Ở đây là `OrderEvent` được `json.Marshal` thành
  một chuỗi JSON. Cấu trúc `OrderEvent`:

  ```go
  // pkg/queue/sqs.go:20-25
  type OrderEvent struct {
  	EventType EventType `json:"event_type"`
  	OrderID   uint      `json:"order_id"`
  	UserID    uint      `json:"user_id"`
  	Amount    float64   `json:"amount"`
  }
  ```

  Nên message body trên dây trông như: `{"event_type":"ORDER_PLACED","order_id":42,"user_id":7,"amount":59.9}`.
  SQS coi body chỉ là **một chuỗi text** — nó không hiểu JSON, không soi vào trong. Việc
  parse JSON là trách nhiệm của bên nhận (chương 4).

- **Message attribute `event_type`** — metadata gắn kèm, **tách rời** khỏi body. Vì sao
  lại nhân đôi `event_type` (đã có trong body rồi)? Vì attribute đọc được **mà không cần
  parse body** — hữu ích nếu sau này bạn muốn lọc/định tuyến message theo loại (ví dụ SNS
  filter policy) mà không phải mở body ra. Ở repo này worker đọc từ body cho đơn giản,
  nhưng attribute vẫn được gắn để sẵn sàng cho các use case đó.

- **`QueueUrl`** — message đi vào queue nào. Giá trị `q.queueURL` đến từ **biến môi trường**.
  Xem nơi nó được nạp:

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

  Hai env quyết định client: **`AWS_REGION`** (queue nằm ở region nào) và
  **`AWS_SQS_ORDER_QUEUE_URL`** (URL đầy đủ của queue). Thiếu URL → client `nil` → đúng cái
  `if s.sqsClient != nil` ta thấy ở trên.

> 🔍 **Bên dưới — credential lấy từ đâu?** Để ý hàm tạo client **không** truyền access key
> ở bất kỳ đâu:
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
> Nó chỉ truyền `Region`. AWS SDK tự tìm credential qua **default credential chain**, theo
> thứ tự:
>
> 1. **Biến môi trường** — `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` (và
>    `AWS_SESSION_TOKEN` nếu là tạm thời).
> 2. **Shared config file** — `~/.aws/credentials` / `~/.aws/config` (chọn profile qua
>    `AWS_PROFILE`). Đây là cách phổ biến khi dev ở máy local.
> 3. **IAM role** — khi chạy trên EC2 / ECS / Lambda, SDK tự lấy credential tạm thời từ
>    metadata của role gắn vào máy. Đây là cách chuẩn ở production: **không có key tĩnh
>    nào nằm trong code hay env**.
>
> Tức là: ở local bạn để key trong `~/.aws/credentials`; lên production bạn gắn IAM role —
> **code không đổi một dòng**. Region thì luôn phải nói rõ vì queue URL gắn với một region
> cụ thể.

Đến đây message của order đã rời khỏi API và nằm trong queue. Chương sau xem nó "sống" thế
nào trong lúc chờ.

---

## Chương 3 — Message nằm trong queue

Bây giờ message của order đang nằm trong queue, chờ ai đó nhặt ra. Đây là lúc cần hiểu
**những cơ chế ngầm** của SQS — chúng quyết định cách ta phải viết worker ở chương sau.

> 🔍 **Bên dưới — at-least-once delivery (giao ít nhất một lần)**
>
> SQS standard queue bảo đảm mỗi message được giao **ít nhất một lần**, **không** bảo đảm
> *đúng* một lần. Nghĩa là: bình thường mỗi message được giao 1 lần, nhưng trong một số
> tình huống (mạng chập chờn, message được nhân bản qua nhiều server lưu trữ của SQS) bạn
> có thể nhận **cùng một message hai lần**.
>
> Đây không phải bug — đó là đánh đổi để đạt throughput và độ sẵn sàng cực cao. Hệ quả
> trực tiếp: **worker phải chịu được việc xử lý trùng** mà không gây hại. Khái niệm này gọi
> là **idempotency**, ta sẽ làm ở chương 5.

> 🔍 **Bên dưới — visibility timeout (vì sao message "biến mất" tạm thời)**
>
> Khi worker gọi `ReceiveMessage` và nhặt được message, message đó **không bị xóa khỏi
> queue**. Thay vào đó nó bị **ẩn đi** trong một khoảng thời gian gọi là **visibility
> timeout** (mặc định 30 giây). Trong khoảng này, các consumer khác gọi `ReceiveMessage`
> sẽ **không thấy** message đó.
>
> Mục đích: tránh hai worker xử lý cùng một message song song. Khi đang xử lý, message
> "vô hình" với mọi người khác.
>
> Có hai kết cục:
>
> - Worker xử lý **xong** và gọi `DeleteMessage` → message biến mất vĩnh viễn. Xong việc.
> - Worker **chưa** xóa (đang xử lý lâu, hoặc bị crash) và visibility timeout **hết hạn**
>   → message **hiện lại** trong queue, sẵn sàng để giao cho lần nhặt tiếp theo. Đây chính
>   là cơ chế **retry tự động**.
>
> ⚠️ Đây là một cái bẫy kinh điển: nếu handler của bạn xử lý **lâu hơn** visibility timeout,
> message sẽ hiện lại **giữa chừng** trong lúc bạn vẫn đang xử lý → một worker khác nhặt nó
> và xử lý **lần nữa** → trùng. Hoặc đặt visibility timeout đủ dài hơn thời gian xử lý tối
> đa, hoặc thiết kế xử lý idempotent (tốt nhất là cả hai).

> 🔍 **Bên dưới — standard vs FIFO queue**
>
> SQS có hai loại queue:
>
> | | **Standard** | **FIFO** |
> |---|---|---|
> | Thứ tự | Không bảo đảm | Bảo đảm đúng thứ tự (First-In-First-Out) |
> | Giao hàng | At-least-once (có thể trùng) | Exactly-once (chống trùng trong cửa sổ 5 phút) |
> | Throughput | Gần như không giới hạn | Có giới hạn (300–3000 msg/s) |
> | Tên queue | tùy | bắt buộc đuôi `.fifo` |
>
> Repo này dùng **standard queue** — đơn giản, throughput cao, và việc order events không
> bắt buộc đúng thứ tự tuyệt đối. Cái giá phải trả là *có thể trùng* và *có thể lệch thứ
> tự* → ta xử lý bằng idempotency. FIFO chỉ cần khi thứ tự là bắt buộc nghiêm ngặt (ví dụ:
> các bước của một giao dịch tài chính phải theo đúng trình tự).

**Vì sao có thể nhận trùng — tóm gọn ba nguồn:**

1. **At-least-once** của standard queue: bản chất SQS có thể giao lại.
2. **Visibility timeout hết hạn giữa chừng**: handler chậm hơn timeout → message hiện lại
   trong khi vẫn đang xử lý.
3. **Crash sau xử lý, trước khi `DeleteMessage`**: worker làm xong việc (đã gửi mail) rồi
   chết ngay trước khi kịp xóa → message hiện lại → lần sau xử lý lại từ đầu.

Cả ba đều dẫn về cùng một kết luận: **đừng giả định mỗi message chỉ chạy đúng một lần.**
Đây là lý do chương 5 phải nói về idempotency. Giờ sang chương 4 — phần worker nhặt message.

---

## Chương 4 — Đầu nhận (phần ta vừa xây)

Đây là nửa vòng lặp mà repo trước đây **còn thiếu** và ta vừa xây: **consumer**. Trái tim
của nó là hàm `processOnce` — nhận một batch message, xử lý từng cái, xóa cái nào xử lý xong.

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

Và hàm `ReceiveMessages` mà nó gọi:

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

Lưu ý lời gọi `c.queue.ReceiveMessages(10, 20)` — `maxMessages=10`, `waitSeconds=20`.

**`waitSeconds=20` → long polling.** Đây là khái niệm quan trọng:

> 🔍 **Bên dưới — long polling vs short polling**
>
> - **Short polling** (`WaitTimeSeconds=0`): `ReceiveMessage` trả về **ngay lập tức**. Nếu
>   queue rỗng → trả về danh sách rỗng tức thì. Worker phải gọi lại liên tục → **tốn rất
>   nhiều API call** (mỗi call là tiền) và phần lớn trả về rỗng. Ngoài ra, do SQS lưu trữ
>   phân tán, short polling chỉ hỏi *một phần* các server → đôi khi báo "rỗng" dù thực ra
>   có message.
> - **Long polling** (`WaitTimeSeconds=1..20`): nếu queue rỗng, SQS **giữ kết nối mở** tối
>   đa `waitSeconds` giây, **chờ** có message tới thì trả về ngay. Hết thời gian chờ mà vẫn
>   rỗng mới trả danh sách rỗng. Lợi: **giảm số API call** (đỡ tốn tiền), **giảm latency**
>   (message tới là nhận gần như tức thì), và quét toàn bộ server lưu trữ nên không bỏ sót.
>
> 20 giây là **giá trị tối đa** SQS cho phép. Gần như mọi consumer production đều nên dùng
> long polling.

**`maxMessages=10` → batch.** Một lần `ReceiveMessage` lấy **tối đa 10 message** (đây là
giới hạn cứng của SQS). Lấy theo lô giúp giảm số round-trip mạng. Code lặp `for _, m := range msgs`
xử lý từng message trong lô.

**Quy tắc vàng: chỉ `DeleteMessage` SAU KHI handler thành công.** Nhìn kỹ thứ tự trong
`processOnce`:

- `dispatch(event)` trả lỗi → `continue`, **KHÔNG xóa**. Message ở lại queue, hết visibility
  timeout sẽ hiện lại để thử lại (chương 5).
- `dispatch(event)` trả `nil` (thành công) → `c.deleteQuietly(*m.ReceiptHandle)` mới xóa.

Đây là điểm cốt lõi của toàn bộ thiết kế SQS: **message chỉ biến mất khi việc đã làm xong**.
Xóa *trước* khi xử lý (xóa ngay sau khi nhận) là sai lầm chết người — worker crash giữa
chừng thì việc đó **mất luôn**, không ai biết. Xóa *sau* khi xử lý xong nghĩa là worst case
chỉ là *làm lại* (nhờ idempotency), không bao giờ *mất việc*.

**Xóa bằng `ReceiptHandle`, KHÔNG phải `order_id` hay message ID.** Để ý ta truyền
`*m.ReceiptHandle`:

```go
// internal/worker/consumer.go:92-96
func (c *Consumer) deleteQuietly(receiptHandle string) {
	if err := c.queue.DeleteMessage(receiptHandle); err != nil {
		slog.Error("worker: delete failed (message may be redelivered)", "err", err)
	}
}
```

> 🔍 **Bên dưới — ReceiptHandle là gì?** Mỗi message có một **message ID** cố định (định
> danh message), nhưng để **xóa** thì SQS yêu cầu **ReceiptHandle** — một token **chỉ cấp
> cho lần nhận này**. Mỗi lần `ReceiveMessage` trả về cùng một message (sau redelivery), nó
> cấp một ReceiptHandle **mới khác**. Bạn phải xóa bằng ReceiptHandle của **chính lần nhận
> hiện tại**; dùng cái cũ sẽ lỗi. Đây là cách SQS xác nhận "tôi xóa đúng cái message mà tôi
> vừa cầm trên tay", chứ không phải `order_id` (cái này nằm trong body, SQS không quan tâm).

**Hai trường hợp đặc biệt trong code thật:**

1. **Poison message (body không parse được)** — nếu `json.Unmarshal` lỗi, message này sẽ
   **không bao giờ** parse được, nên retry vô hạn là vô nghĩa và sẽ **chặn** queue. Code
   chọn cách **xóa luôn** (kèm log) để không kẹt. Đây là quyết định có chủ đích — nếu muốn
   giữ lại để điều tra, bạn để nó rớt DLQ thay vì xóa.

2. **Receive error backoff** — nếu chính `ReceiveMessages` lỗi (sai credential, mất mạng,
   queue bị xóa...), `processOnce` trả lỗi và `Run` **chờ một nhịp** rồi mới thử lại, tránh
   quay vòng nóng (busy-loop) đốt CPU:

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

   `receiveErrorBackoff` là 5 giây (`internal/worker/consumer.go:17`). Để ý cả vòng lặp
   `Run` tôn trọng `ctx.Done()` — đó là **graceful shutdown**: khi nhận SIGINT/SIGTERM,
   worker dừng nhặt batch mới, không cắt ngang việc đang làm. Message nào chưa kịp
   `DeleteMessage` sẽ tự hiện lại sau visibility timeout — an toàn nhờ đúng quy tắc vàng ở
   trên.

---

## Chương 5 — Khi xử lý lỗi

Đây là chỗ at-least-once và visibility timeout (chương 3) gặp nhau và trở thành cơ chế
**retry + DLQ** thực thụ. Đi theo order của ta khi mọi thứ *không* suôn sẻ.

**Vòng đời của một message bị lỗi:**

```
   nhận lần 1 ─▶ handler lỗi ─▶ KHÔNG xóa ─▶ ẩn (visibility timeout) ─┐
       ▲                                                              │
       └────────── hiện lại sau timeout ◀─────────────────────────────┘
   ... lặp lại tối đa maxReceiveCount lần ...
   nhận lần thứ (maxReceiveCount+1) ─▶ SQS chuyển sang ─▶ [ DLQ ]
```

- Handler trả lỗi → `processOnce` **không xóa** → message ở lại.
- Hết **visibility timeout** → message **hiện lại** → worker nhặt lại → **retry**.
- Cứ thế lặp. SQS đếm số lần một message được nhận (`ApproximateReceiveCount`).
- Khi vượt **`maxReceiveCount`** (ta đặt = 5 ở chương 6), SQS tự **chuyển message sang
  Dead Letter Queue (DLQ)** — một queue riêng dành cho các message "độc", thay vì để chúng
  retry vô hạn.

> 🔍 **Bên dưới — Dead Letter Queue (DLQ)** là một queue thường, nhưng được gắn làm "thùng
> rác có kiểm soát" cho queue chính qua **RedrivePolicy**. Khi một message thất bại quá
> `maxReceiveCount` lần, SQS tự đẩy nó sang DLQ. Lợi ích: (1) queue chính không bị **kẹt**
> bởi một message hỏng cứ retry mãi; (2) bạn có một nơi để **điều tra** các message lỗi
> (mở DLQ ra xem body, log) mà không mất chúng; (3) sau khi sửa bug, có thể **redrive**
> (đẩy ngược) message từ DLQ về queue chính để xử lý lại.

**Worker sở hữu việc chuyển sang `confirmed`.** Nhớ ở chương producer: API webhook chỉ đặt
`payment_status=paid` rồi publish `ORDER_PAID`. Chính **worker** mới chuyển order từ `paid`
sang `confirmed` và gửi email "đã nhận tiền". The worker — not the API — owns the
`confirmed` transition.

**Idempotency — vì sao và làm thế nào.** Vì at-least-once, một message có thể chạy 2 lần.
Với `ORDER_PAID`, chạy 2 lần mà không cẩn thận = cập nhật trạng thái 2 lần, gửi 2 email "đã
nhận tiền". Code chặn việc đó bằng một **idempotency guard** đơn giản: kiểm tra trạng thái
trước khi hành động. Vì giờ API **không** còn tự set `confirmed` nữa, guard này làm việc
thật chứ không phải trang trí: lần giao đầu tiên order chưa `confirmed` → worker confirm +
gửi mail; bản trùng giao lại thấy đã `confirmed` → bỏ qua an toàn.

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

Mấu chốt: `if order.Status == domain.OrderStatusConfirmed { ... return nil }`. Lần đầu xử
lý đặt trạng thái thành `confirmed`. Nếu message tới lần hai, order **đã** `confirmed` →
handler bỏ qua, trả `nil` (coi như thành công) → message được xóa. Không cập nhật trùng,
không gửi mail trùng. **Trạng thái trong DB chính là cuốn sổ ghi "việc này đã làm chưa".**

So sánh với `handleOrderPlaced` — handler này **cố ý KHÔNG** có guard:

```go
// internal/worker/handlers.go:47-49
	// No idempotency guard here: SQS is at-least-once, so a redelivered
	// ORDER_PLACED may send a duplicate confirmation email. That's an
	// accepted trade-off — a duplicate confirmation is benign.
```

Bài học: **idempotency là một lựa chọn theo từng loại tác dụng phụ**. Gửi *hai* email xác
nhận "đã nhận đơn" thì phiền nhưng vô hại → chấp nhận. Còn cập nhật trạng thái / trừ tiền
2 lần thì nguy hiểm → phải có guard. Bạn cân nhắc theo *mức độ tai hại khi chạy trùng*.

> 🔍 **Bên dưới — khi nào cần bảng `processed_events`?** Cách "kiểm tra trạng thái đích"
> như trên chỉ hợp khi tác dụng phụ làm cho trạng thái **trở nên ổn định** (đã confirmed
> thì lần sau biết để bỏ qua). Nhưng có những tác dụng phụ **không để lại dấu vết trạng
> thái rõ ràng** — ví dụ "cộng điểm thưởng 10 điểm mỗi lần thanh toán": chạy 2 lần thành 20
> điểm, và nhìn vào "tổng điểm" không thể biết đã cộng cho event này chưa.
>
> Khi đó cần một bảng **`processed_events`** ghi lại **ID của từng message/event đã xử lý**.
> Trước khi hành động: nếu event ID đã có trong bảng → bỏ qua; nếu chưa → xử lý rồi *ghi ID
> vào bảng trong cùng một transaction*. Đây là dedup tổng quát theo message ID, không phụ
> thuộc vào trạng thái nghiệp vụ. Repo này chưa cần (chỉ dùng guard theo trạng thái) — nêu
> ra để bạn biết khi nào phải nâng cấp.

### Bảng lỗi thường gặp

| Lỗi | Triệu chứng / cách nhận biết |
|---|---|
| **Sai/thiếu credential** | `ReceiveMessages` trả lỗi liên tục; log `worker: receive failed, will retry` lặp đều mỗi ~5s (do backoff). Thường kèm message AWS dạng `AccessDenied`/`InvalidClientTokenId`/`NoCredentialProviders`. Kiểm tra `~/.aws/credentials` hoặc env `AWS_ACCESS_KEY_ID`. |
| **Queue URL sai region** | Lỗi kiểu `AWS.SimpleQueueService.NonExistentQueue` hoặc `QueueDoesNotExist` — vì `AWS_REGION` trỏ region A nhưng queue URL thuộc region B. Đối chiếu region trong URL queue với `AWS_REGION`. |
| **Poison message (body không parse được)** | Log `worker: unparseable message body, discarding` kèm body in ra. Code tự xóa nên *không* kẹt queue, nhưng nếu thấy nhiều → có producer khác đang gửi body sai định dạng vào queue. |
| **Handler chậm hơn visibility timeout** | Cùng một `order_id` xuất hiện trong log **hai lần** (xử lý trùng) dù handler "có vẻ" chạy đúng; mail gửi lặp. Nguyên nhân: message hiện lại giữa chừng. Tăng visibility timeout của queue, hoặc bảo đảm handler idempotent. |
| **Quên `DeleteMessage` (xóa sau xử lý)** | Cùng một message được xử lý **lặp vô hạn**, log thành công lặp đi lặp lại mãi, sau cùng rớt DLQ dù không có lỗi nào. Nguyên nhân: nhánh thành công không gọi delete. Trong repo này nhánh thành công luôn gọi `deleteQuietly` — lỗi này xuất hiện khi ai đó sửa sai thứ tự. |

---

## Chương 6 — Config AWS thật + IAM

Tới đây bạn đã hiểu cơ chế. Giờ dựng hạ tầng thật trên AWS. Repo này **không dùng IaC** —
cấu hình thủ công bằng AWS CLI (hoặc Console). Thứ tự **bắt buộc**: tạo **DLQ trước**, vì
queue chính cần ARN của DLQ để khai báo RedrivePolicy.

```bash
# Bước 1 — Tạo DLQ TRƯỚC (queue chính sẽ trỏ tới nó)
aws sqs create-queue --queue-name order-events-dlq

# Bước 2 — Lấy ARN của DLQ (cần cho RedrivePolicy ở bước 3)
#   Thay <DLQ_URL> bằng QueueUrl mà bước 1 in ra.
aws sqs get-queue-attributes --queue-url <DLQ_URL> \
  --attribute-names QueueArn

# Bước 3 — Tạo queue chính `order-events`, trỏ redrive sang DLQ, tối đa 5 lần nhận
#   Thay <DLQ_ARN> bằng giá trị QueueArn mà bước 2 in ra.
aws sqs create-queue --queue-name order-events \
  --attributes '{"RedrivePolicy":"{\"deadLetterTargetArn\":\"<DLQ_ARN>\",\"maxReceiveCount\":\"5\"}"}'
```

Giải thích `RedrivePolicy`: `maxReceiveCount=5` nghĩa là một message bị nhận lại quá 5 lần
(tức xử lý lỗi 5 lần) thì SQS đẩy nó sang `deadLetterTargetArn` (DLQ). Khớp đúng với cơ chế
DLQ ở chương 5.

Sau khi tạo xong, lấy **QueueUrl của `order-events`** (không phải của DLQ) đặt vào env:

```bash
export AWS_REGION=ap-southeast-1
export AWS_SQS_ORDER_QUEUE_URL=https://sqs.ap-southeast-1.amazonaws.com/<account-id>/order-events
```

Hai env này nối thẳng về `configs/appConfig.go:77-85` (chương 2): `AWS_REGION` cho biết
queue ở region nào, `AWS_SQS_ORDER_QUEUE_URL` cho biết URL chính xác. Cả API service lẫn
worker đều đọc **cùng** hai env này — đó là cách hai process "gặp nhau" tại cùng một queue
mà không gọi trực tiếp nhau (chương 1).

### IAM least-privilege — hai policy tách biệt

API service (producer) và worker (consumer) cần **quyền khác nhau**. Đừng cấp chung một
credential full-access. Tạo **hai** policy, gắn cho hai identity riêng:

```json
// API service (producer) — CHỈ được gửi message
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
// Worker (consumer) — được nhận, xóa, xem thuộc tính queue
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

**Vì sao không dùng chung một credential full-access?** — nguyên tắc **least privilege**
(quyền tối thiểu):

- **Giảm thiệt hại khi rò rỉ.** Nếu credential của API service bị lộ, kẻ tấn công *chỉ* có
  thể nhồi message vào queue — **không** xóa được, **không** đọc trộm được message của
  người khác. Nếu đó là credential full-access, chúng làm được mọi thứ.
- **Phản ánh đúng vai trò.** Producer không có lý do gì để `DeleteMessage`; consumer không
  có lý do gì để `SendMessage`. Tách quyền giúp một bug ở bên này không vô tình phá bên kia.
- **Dễ audit.** Nhìn policy là biết chính xác mỗi service được làm gì.

Lưu ý: worker cần `sqs:GetQueueAttributes` (ngoài Receive/Delete) vì một số thao tác và
công cụ vận hành cần đọc thuộc tính queue. Nó **không** cần quyền trên DLQ — chính SQS
(không phải worker) là bên đẩy message sang DLQ.

---

## Chương 7 — Chạy thật end-to-end

Kịch bản kiểm chứng theo **đúng thứ tự**. Cần: queue đã tạo (chương 6), env đã set, DB
Postgres đang chạy (`docker-compose up -d`).

### Bước 1 — Bật API và đặt một đơn (terminal A)

```bash
make server
```

Rồi đặt một order qua API (đăng nhập lấy token, thêm vào giỏ, rồi gọi):

```bash
curl -X POST http://localhost:9000/user/me/order \
  -H "Authorization: Bearer <JWT>" \
  -H "Content-Type: application/json" \
  -d '{"shipping_address":"123 Le Loi, Q1"}'
```

Lúc này `PlaceOrder` đã publish một message `ORDER_PLACED` vào queue (chương 2). **Chưa**
chạy worker, nên message còn nằm trong queue.

### Bước 2 — Xem tận mắt message trong queue (thủ công)

```bash
aws sqs receive-message --queue-url "$AWS_SQS_ORDER_QUEUE_URL" \
  --message-attribute-names All \
  --wait-time-seconds 5
```

Bạn sẽ thấy JSON chứa `Body` (chuỗi `{"event_type":"ORDER_PLACED",...}`), một
`MessageId`, một `ReceiptHandle` (token để xóa — chương 4), và `MessageAttributes` với
`event_type`. Đây chính là cái worker sẽ nhặt ra.

> ⚠️ Lệnh này **đã nhận** message → nó đang trong **visibility timeout** (ẩn ~30s). Đừng
> xóa nó. Chờ qua timeout, message **hiện lại** và worker ở bước 3 sẽ nhặt được. (Bạn vừa
> tự tay quan sát visibility timeout của chương 3.)

### Bước 3 — Bật worker, xem nó xử lý rồi message biến mất (terminal B)

```bash
make worker
```

Quan sát log: `worker: started, polling for messages`, rồi
`worker: order confirmation sent order=<id>`. Worker đã nhặt message, gọi
`handleOrderPlaced`, gửi email xác nhận, và `DeleteMessage`. Kiểm tra lại bằng cách chạy
lệnh `receive-message` ở bước 2 lần nữa — queue **rỗng**. Message đã xong vòng đời:
send → receive → process → delete.

### Bước 4 — Cố ý gây lỗi để quan sát retry + rớt DLQ

Tạm sửa đầu `handleOrderPlaced` để **luôn trả lỗi** (nhớ `import "errors"`):

```go
// internal/worker/handlers.go — handleOrderPlaced, dòng đầu hàm (CHỈ để thử nghiệm)
func (d *Dispatcher) handleOrderPlaced(event queue.OrderEvent) error {
	return errors.New("test: simulate handler failure")
	// ... phần còn lại tạm thời không chạy tới
}
```

Đặt một order mới, rồi `make worker`. Quan sát:

- Log lặp `worker: handler failed, leaving message for retry ... type=ORDER_PLACED`.
- Message **không** bị xóa → sau mỗi visibility timeout lại hiện lại → worker thử lại.
- Sau **5 lần** (`maxReceiveCount=5`, chương 6), SQS đẩy message sang **`order-events-dlq`**.

Kiểm tra DLQ có message:

```bash
# Lấy số message đang nằm trong DLQ
aws sqs get-queue-attributes --queue-url <DLQ_URL> \
  --attribute-names ApproximateNumberOfMessages
```

Hoặc mở **SQS Console** → queue `order-events-dlq` → *Send and receive messages* → *Poll
for messages* để thấy chính cái message lỗi nằm đó. **Nhớ hoàn tác** đoạn `return errors.New(...)`
sau khi thử xong.

---

Hết. Giờ bạn đã đi trọn vòng đời một message: API bỏ vào (chương 2) → nằm chờ với
visibility timeout & at-least-once (chương 3) → worker long-poll nhặt ra, xử lý, xóa đúng
lúc (chương 4) → lỗi thì retry và rớt DLQ, trùng thì idempotency chặn (chương 5) → trên
hạ tầng AWS thật với IAM tách quyền (chương 6) → và tự tay kiểm chứng (chương 7). Đó là
"cái chạy bên dưới" của SQS trong repo này.
