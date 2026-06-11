# Go E-Commerce Backend

Production-style REST API for an e-commerce platform — Go, Fiber, GORM, PostgreSQL, and AWS.

---

## Architecture

```
┌──────────────────────────────────────────────────┐
│                  HTTP Clients                    │
└─────────────────────┬────────────────────────────┘
                      │
             ┌────────▼─────────┐
             │  Fiber Router    │  JWT middleware, SellerOnly guard
             └────────┬─────────┘
        ┌─────────────┼──────────────┐
        ▼             ▼              ▼
   [User / Cart]  [Product /    [Order /
    Handler       Seller]        Payment]
        └─────────────┬──────────────┘
                      ▼
             ┌────────────────┐
             │   Services     │  business logic, interface-driven
             └────────┬───────┘
          ┌───────────┼─────────────┐
          ▼           ▼             ▼
    [Repositories] [AWS SDK]   [Stripe SDK]
    GORM/Postgres  SES SNS S3 SQS
```

Layered design: **Handler → Service → Repository → PostgreSQL**.
Each layer depends on the one below via interfaces — services are independently testable.

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.26 |
| Web framework | Fiber v2 |
| ORM | GORM + PostgreSQL 16 |
| Auth | JWT HS256 (24 h) + bcrypt |
| Email | AWS SES |
| SMS | AWS SNS |
| Image storage | AWS S3 |
| Order events | AWS SQS |
| Payments | Stripe (AWS has no general payment gateway) |
| Hot reload | Air |

---

## AWS Services

| Service | Role |
|---------|------|
| **SES** | Transactional email — verification codes, seller activation |
| **SNS** | SMS — verification code (primary, email is fallback) |
| **S3** | Product image hosting — sellers upload, URLs stored in DB |
| **SQS** | Order event queue — `ORDER_PLACED` and `ORDER_PAID` published on state changes; downstream workers subscribe independently |

> Payments use **Stripe** — AWS Payment Cryptography covers card-data primitives, not a full payment gateway.

---

## Domain Model

```
User ──── Address
 │
 ├── Cart ──── CartItem ──── Product ──── ProductImage
 │                                  └─── Category
 │
 └── Order ── OrderItem ─── Product
```

---

## API Reference

### Public

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/user/register` | Register with email, phone, password |
| `POST` | `/user/login` | Login → JWT token |
| `GET` | `/products` | List products (`?search=&category_id=&min_price=&max_price=&page=&limit=`) |
| `GET` | `/products/:id` | Product detail |
| `GET` | `/categories` | All categories |

### Authenticated (`Authorization: Bearer <token>`)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/user/me/verify` | Request verification code (SMS → email fallback) |
| `POST` | `/user/me/verify` | Submit code `{"code":"123456"}` |
| `GET` | `/user/me/profile` | Own profile |
| `POST` | `/user/me/profile` | Update `{"first_name","last_name"}` |
| `POST` | `/user/me/become-seller` | Upgrade account to seller |
| `POST` | `/user/me/cart` | Add to cart `{"product_id","quantity"}` |
| `GET` | `/user/me/cart` | View cart |
| `DELETE` | `/user/me/cart/:item_id` | Remove cart item |
| `POST` | `/user/me/order` | Place order from cart `{"shipping_address"}` |
| `GET` | `/user/me/order` | My orders |
| `GET` | `/user/me/order/:id` | Order detail |

### Seller only (UserType = `seller`)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/seller/product` | Create product |
| `GET` | `/seller/products` | My product listings |
| `PUT` | `/seller/product/:id` | Update product |
| `DELETE` | `/seller/product/:id` | Delete product |
| `POST` | `/seller/product/:id/image` | Upload image to S3 (multipart `image`) |
| `POST` | `/seller/category` | Create category |

### Payments

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/orders/payment/intent` | Create Stripe PaymentIntent `{"order_id"}` → returns `client_secret` |
| `POST` | `/orders/payment/webhook` | Stripe webhook — confirms payment, updates order |

---

## Order Flow

```
POST /user/me/cart          ← add items
POST /user/me/order         ← creates order (pending / payment pending)
                               └─ SQS: ORDER_PLACED
POST /orders/payment/intent ← Stripe PaymentIntent with client_secret
[client confirms payment in frontend]
POST /orders/payment/webhook ← Stripe calls this automatically
                               └─ order: confirmed, payment: paid
                               └─ SQS: ORDER_PAID
```

---

## Setup

### Prerequisites
- Go 1.21+, Docker, AWS credentials (SES + SNS + S3 + SQS permissions)
- Stripe account (optional — payment endpoints degrade gracefully if not set)

### Run

```bash
docker-compose up -d        # start PostgreSQL
cp .env .env.local          # fill in your secrets
make run                    # hot-reload via Air
```

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `HTTP_PORT` | ✅ | e.g. `localhost:9000` |
| `DATA_SOURCE_NAME` | ✅ | PostgreSQL DSN |
| `APP_SECRET` | ✅ | JWT signing key |
| `AWS_REGION` | ✅ | e.g. `ap-southeast-1` |
| `AWS_SES_FROM_EMAIL` | ✅ | Verified SES sender |
| `AWS_S3_BUCKET` | optional | Product image bucket |
| `AWS_SQS_ORDER_QUEUE_URL` | optional | Order event queue URL |
| `STRIPE_SECRET_KEY` | optional | Stripe secret key |
| `STRIPE_WEBHOOK_SECRET` | optional | Stripe webhook signing secret |

Optional services start as `nil` — the server starts normally and all other endpoints function.
See [docs/SERVICES_SETUP.md](docs/SERVICES_SETUP.md) for a step-by-step guide to configuring S3, SQS, and Stripe.

---

## Project Structure

```
.
├── configs/                 environment loading, AWS/Stripe wiring
├── internal/
│   ├── api/
│   │   ├── server.go        DB init, AutoMigrate, route registration
│   │   └── rest/
│   │       ├── httpHandler.go   shared handler context (DB, Auth, clients)
│   │       └── handlers/        userHandler, productHandler,
│   │                            sellerHandler, orderHandler
│   ├── domain/              GORM models: User, Product, Cart, Order, Address…
│   ├── dto/                 request/response structs
│   ├── helper/              JWT, bcrypt, SellerOnly middleware
│   ├── repository/          GORM data access per domain
│   └── service/             business logic per domain
├── pkg/
│   ├── notification/        AWS SES + SNS (composite pattern)
│   ├── storage/             AWS S3 uploader
│   ├── queue/               AWS SQS producer
│   └── payment/             Stripe PaymentIntents + webhook
├── docker-compose.yml
├── Makefile
└── .env
```
