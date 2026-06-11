# Optional Services Setup Guide

This guide covers configuring the three optional integrations:

| Service | Env vars | Feature enabled |
|---------|----------|-----------------|
| AWS S3 | `AWS_S3_BUCKET` | Product image uploads (`POST /seller/product/:id/image`) |
| AWS SQS | `AWS_SQS_ORDER_QUEUE_URL` | Order event publishing (`ORDER_PLACED`, `ORDER_PAID`, `ORDER_SHIPPED`) |
| Stripe | `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` | Payment intents (`POST /orders/payment/intent`) + webhook (`POST /orders/payment/webhook`) |

All three are optional. If an env var is empty, the server starts normally,
logs a `WARNING: ... disabled` line, and the corresponding endpoints return
"not configured" errors. Everything else keeps working.

> ⚠️ **Stripe requires SQS.** The Stripe webhook handler publishes an
> `ORDER_PAID` event to SQS without a nil check
> (`internal/service/orderService.go`, `HandleStripeEvent`). If you enable
> Stripe, also enable SQS — otherwise a successful payment webhook will panic.

---

## 0. Prerequisite: AWS credentials

The S3 and SQS clients use the AWS SDK **default credential chain** — no
access keys are read from `.env`. Provide credentials one of these ways:

**Local development** — either a shared profile:

```bash
aws configure
# writes ~/.aws/credentials with aws_access_key_id / aws_secret_access_key
```

or environment variables (add to your shell or `.env` if you prefer):

```bash
export AWS_ACCESS_KEY_ID=AKIA...
export AWS_SECRET_ACCESS_KEY=...
```

**Production (EC2/ECS/EKS)** — attach an IAM role to the instance/task;
no keys needed.

The region comes from the already-required `AWS_REGION` variable (e.g.
`ap-southeast-1`). The same region is used for SES, S3, and SQS, so create
your bucket and queue in that region.

---

## 1. AWS S3 — product image storage

### 1.1 Create the bucket

```bash
aws s3api create-bucket \
  --bucket my-ecommerce-product-images \
  --region ap-southeast-1 \
  --create-bucket-configuration LocationConstraint=ap-southeast-1
```

(Or via Console: S3 → Create bucket. Bucket names are globally unique.)

### 1.2 Allow public read of uploaded images

`UploadFile` returns the object's public URL, which is stored on the product
and served to clients — so objects must be publicly readable. New buckets
block public access by default; turn that off and add a read-only policy:

```bash
aws s3api put-public-access-block \
  --bucket my-ecommerce-product-images \
  --public-access-block-configuration \
    BlockPublicAcls=false,IgnorePublicAcls=false,BlockPublicPolicy=false,RestrictPublicBuckets=false

aws s3api put-bucket-policy \
  --bucket my-ecommerce-product-images \
  --policy '{
    "Version": "2012-10-17",
    "Statement": [{
      "Sid": "PublicReadProductImages",
      "Effect": "Allow",
      "Principal": "*",
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::my-ecommerce-product-images/products/*"
    }]
  }'
```

The policy is scoped to `products/*` because the app writes keys as
`products/<productID>/<filename>`.

> For production, prefer keeping the bucket private and serving images
> through CloudFront with Origin Access Control. The app only needs the
> upload to succeed; the returned `result.Location` URL would then be
> replaced by your CDN domain (requires a small code change).

### 1.3 IAM permissions for the app

The app only uploads. Attach this to the IAM user/role the server runs as:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": "s3:PutObject",
    "Resource": "arn:aws:s3:::my-ecommerce-product-images/products/*"
  }]
}
```

### 1.4 Configure and verify

```bash
# .env
AWS_S3_BUCKET=my-ecommerce-product-images
```

Restart the server — the `AWS_S3_BUCKET not set` warning should disappear.
Then upload an image as a seller:

```bash
curl -X POST http://localhost:9000/seller/product/1/image \
  -H "Authorization: Bearer <seller-token>" \
  -F "image=@./test.jpg"
```

The response contains the public S3 URL; open it in a browser to confirm
public read works.

---

## 2. AWS SQS — order event queue

The app publishes JSON order events (`ORDER_PLACED` at checkout,
`ORDER_PAID` from the Stripe webhook) with an `event_type` message
attribute, so consumers can filter without parsing the body.

### 2.1 Create the queue

```bash
aws sqs create-queue \
  --queue-name ecommerce-order-events \
  --region ap-southeast-1
```

A **standard** queue is fine. The command prints the queue URL, or fetch it:

```bash
aws sqs get-queue-url --queue-name ecommerce-order-events
# => https://sqs.ap-southeast-1.amazonaws.com/<account-id>/ecommerce-order-events
```

Optional but recommended: add a dead-letter queue (Console → queue →
Dead-letter queue) so failed consumer processing doesn't lose events.

### 2.2 IAM permissions for the app

The app only sends:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": "sqs:SendMessage",
    "Resource": "arn:aws:sqs:ap-southeast-1:<account-id>:ecommerce-order-events"
  }]
}
```

(Consumers additionally need `sqs:ReceiveMessage` and `sqs:DeleteMessage`.)

### 2.3 Configure and verify

```bash
# .env
AWS_SQS_ORDER_QUEUE_URL=https://sqs.ap-southeast-1.amazonaws.com/<account-id>/ecommerce-order-events
```

Restart, place a test order via `POST /orders` (checkout), then poll:

```bash
aws sqs receive-message \
  --queue-url "$AWS_SQS_ORDER_QUEUE_URL" \
  --message-attribute-names All
```

You should see a message like:

```json
{"event_type":"ORDER_PLACED","order_id":1,"user_id":1,"amount":59.98}
```

---

## 3. Stripe — payment processing

### 3.1 Get the secret key

1. Sign up / log in at <https://dashboard.stripe.com>.
2. Make sure **Test mode** is toggled on (top right) while developing.
3. Go to **Developers → API keys** and copy the **Secret key**
   (`sk_test_...` in test mode, `sk_live_...` in live mode).

```bash
# .env
STRIPE_SECRET_KEY=sk_test_...
```

This alone enables `POST /orders/payment/intent`. The webhook secret is
needed for payment confirmation (next step) — without it, orders never get
marked as paid.

### 3.2 Webhook — local development (Stripe CLI)

Stripe can't reach `localhost`, so use the Stripe CLI to forward events:

```bash
brew install stripe/stripe-cli/stripe
stripe login
stripe listen --forward-to localhost:9000/orders/payment/webhook
```

`stripe listen` prints a signing secret — copy it:

```bash
# .env
STRIPE_WEBHOOK_SECRET=whsec_...
```

Keep `stripe listen` running while you test. Note: the secret printed by
`stripe listen` is different from any dashboard webhook secret — use the
CLI one locally.

### 3.3 Webhook — production

1. Dashboard → **Developers → Webhooks → Add endpoint**.
2. Endpoint URL: `https://<your-domain>/orders/payment/webhook`
3. Subscribe to exactly these events (the handler ignores everything else):
   - `payment_intent.succeeded` → marks the order paid/confirmed, publishes `ORDER_PAID` to SQS
   - `payment_intent.payment_failed` → marks the order's payment failed
4. Copy the endpoint's **Signing secret** (`whsec_...`) into
   `STRIPE_WEBHOOK_SECRET` on the production environment.

### 3.4 Verify the full flow

```bash
# 1. Place an order (checkout) as an authenticated user
curl -X POST http://localhost:9000/orders -H "Authorization: Bearer <token>"

# 2. Create a payment intent for it
curl -X POST http://localhost:9000/orders/payment/intent \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"order_id": 1}'
# => returns a client_secret for Stripe.js / mobile SDKs

# 3. Simulate a successful payment (with `stripe listen` running)
stripe trigger payment_intent.succeeded
```

Then confirm the order shows `payment_status: paid` and an `ORDER_PAID`
message landed in SQS. Use Stripe's test card `4242 4242 4242 4242` (any
future expiry, any CVC) when testing through a real frontend.

Notes:

- Amounts are charged in **USD cents**, computed as `order.TotalAmount * 100`;
  the currency is currently hard-coded to `usd`.
- The order ID travels in the payment intent's `metadata.order_id` and is
  how the webhook maps an event back to an order — don't strip metadata if
  you customize intent creation.

---

## Final `.env` block

```bash
AWS_S3_BUCKET=my-ecommerce-product-images
AWS_SQS_ORDER_QUEUE_URL=https://sqs.ap-southeast-1.amazonaws.com/<account-id>/ecommerce-order-events
STRIPE_SECRET_KEY=sk_test_...
STRIPE_WEBHOOK_SECRET=whsec_...
```

On startup, the absence of each `WARNING: ... disabled` log line confirms
that service initialized.
