# 🛍️ Go E-Commerce App - SMS & Email Verification

A modern e-commerce API built with Go, featuring **SMS + Email verification** using AWS services.

## ✨ Features

- ✅ User Registration with Email + Phone
- ✅ JWT Authentication
- ✅ **SMS Verification** via AWS SNS (Vietnam supported)
- ✅ **Email Verification** via AWS SES
- ✅ Automatic fallback (SMS fails → Email succeeds)
- ✅ PostgreSQL Database with GORM ORM
- ✅ Fiber Web Framework

## 🚀 Quick Start

### 1️⃣ Prerequisites

```bash
# Required
go version 1.21+
docker (for PostgreSQL)
AWS Account with SNS + SES

# Optional
Postman (for API testing)
jq (for JSON parsing in scripts)
```

### 2️⃣ Setup AWS (5 minutes)

See **[QUICK_START.md](./QUICK_START.md)** for immediate setup.

For detailed guide, see **[AWS_SETUP_GUIDE.md](./AWS_SETUP_GUIDE.md)**.

```bash
# Quick setup
aws configure
aws ses verify-email-identity --email-address noreply@yourdomain.com --region ap-southeast-1
```

### 3️⃣ Configure Environment

```bash
# Update .env
nano .env

# Verify these are set:
AWS_REGION=ap-southeast-1
AWS_SES_FROM_EMAIL=noreply@yourdomain.com
```

### 4️⃣ Start Server

```bash
# Terminal 1: Start PostgreSQL (if using Docker)
docker run --name postgres -e POSTGRES_PASSWORD=root -d -p 5432:5432 postgres

# Terminal 2: Run server
go run main.go

# Expected:
# Database connection established
# [Fiber] 🚀 Started server http://127.0.0.1:9000
```

### 5️⃣ Test

**Option A: Automated Script**
```bash
chmod +x test-verification.sh
./test-verification.sh
```

**Option B: Postman Collection**
- Import `Postman_Collection.json` into Postman
- Follow the requests in order

**Option C: Manual cURL**
```bash
# Register
curl -X POST http://localhost:9000/user/register \
  -H "Content-Type: application/json" \
  -d '{
    "email":"testuser@gmail.com",
    "password":"password123",
    "phone":"+84969650661"
  }'

# Get verification code (triggers SMS + Email)
curl -X GET http://localhost:9000/user/me/verify \
  -H "Authorization: Bearer $TOKEN"

# Verify code
curl -X POST http://localhost:9000/user/me/verify \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"code":"123456"}'
```

---

## 📋 API Endpoints

### Public Routes

| Method | Endpoint | Body |
|--------|----------|------|
| POST | `/user/register` | `{email, password, phone}` |
| POST | `/user/login` | `{email, password}` |

### Protected Routes (require Bearer token)

| Method | Endpoint | Body |
|--------|----------|------|
| GET | `/user/me/verify` | - |
| POST | `/user/me/verify` | `{code}` |
| GET | `/user/me/profile` | - |
| POST | `/user/me/profile` | - |

---

## 🏗️ Architecture

```
go-ecommerce-app/
├── main.go                 # Entry point
├── configs/                # Configuration files
│   └── appConfig.go       # AWS SES + SNS setup
├── internal/
│   ├── domain/            # Business logic models
│   │   └── User.go
│   ├── dto/               # Data Transfer Objects
│   │   └── userRequestDto.go
│   ├── service/           # Business logic
│   │   └── userService.go
│   ├── repository/        # Database access
│   │   └── userRepository.go
│   ├── helper/            # Utilities
│   │   ├── auth.go        # JWT + Password hashing
│   │   └── utils.go
│   └── api/               # HTTP Handlers
│       └── rest/
│           └── handlers/
│               └── userHandler.go
└── pkg/notification/      # External services
    ├── notification.go    # Interface
    ├── awsSes.go         # Email service
    └── awsSns.go         # SMS service
```

## 🔐 Verification Flow

```
1. POST /user/register
   ↓
2. User gets JWT token
   ↓
3. GET /user/me/verify (authenticated)
   ↓
4. System generates 6-digit code
   ↓
5. Code saved to database (expires in 30 min)
   ↓
6. SMS sent to phone + Email sent to inbox
   ↓
7. User receives code on phone OR email
   ↓
8. POST /user/me/verify {code}
   ↓
9. Code verified, user marked as verified ✅
```

## 📊 Notification Methods

### SMS (Primary)
- **Service**: AWS SNS
- **Cost**: ~500 VND/SMS (~0.022 USD)
- **Speed**: 5-10 seconds
- **Support**: Vietnam ✅ (+84)

### Email (Backup)
- **Service**: AWS SES
- **Cost**: FREE (62K emails/month in Free Tier)
- **Speed**: 30-60 seconds
- **Fallback**: If SMS fails, email is sent

**Composite Strategy**: Both methods are triggered. If SMS fails to send, email silently succeeds. User gets code through one or both channels.

## 🧪 Testing

### Automated Test Script
```bash
./test-verification.sh
```

### Postman Collection  
Import `Postman_Collection.json` and follow requests

### AWS CLI Testing
```bash
# Test SMS directly
aws sns publish \
  --phone-number +84969650661 \
  --message "Test SMS" \
  --region ap-southeast-1

# Test Email directly
aws ses send-email \
  --from noreply@yourdomain.com \
  --to your-email@gmail.com \
  --subject "Test" \
  --text "Hello" \
  --region ap-southeast-1
```

## 📚 Documentation

- **[QUICK_START.md](./QUICK_START.md)** - 5-minute setup
- **[AWS_SETUP_GUIDE.md](./AWS_SETUP_GUIDE.md)** - Detailed AWS configuration
- **[Postman_Collection.json](./Postman_Collection.json)** - API test collection
- **[test-verification.sh](./test-verification.sh)** - Automated test script

## 🛠️ Troubleshooting

### SMS not arriving
1. Check phone number format: `+84969650661`
2. Verify AWS SNS has Vietnam SMS support
3. Check AWS CloudWatch logs
4. Test with CLI: `aws sns publish --phone-number +84... --message "test" --region ap-southeast-1`

### Email not arriving
1. Check AWS SES email is verified
2. Verify sending email is same as verified email
3. Check spam folder
4. Check AWS CloudWatch logs

### "unauthorized" error
1. Verify JWT token is included in Authorization header
2. Check token format: `Bearer <token>`
3. Token might be expired (24-hour expiry)

For more troubleshooting, see **[AWS_SETUP_GUIDE.md](./AWS_SETUP_GUIDE.md)**.

## 💰 Cost Estimation (Monthly)

| Component | Usage | Cost |
|-----------|-------|------|
| AWS SNS SMS | 100 | ~1,000 VND (~$0.04) |
| AWS SES Email | 1,000 | FREE (Free Tier) |
| **Total** | - | **~1,000 VND (~$0.04)** |

For 1,000 codes/month:
- SMS: ~500,000 VND (~$20)
- Email: FREE
- **Total: ~500,000 VND/month**

---

## 🔧 Configuration

### Environment Variables

```env
# Server
HTTP_PORT=localhost:9000

# Database
DATA_SOURCE_NAME=host=127.0.0.1 user=root password=root dbname=online_shopping port=5432 sslmode=disable

# Authentication
APP_SECRET=your-secret-key-here

# AWS Configuration
AWS_REGION=ap-southeast-1
AWS_SES_FROM_EMAIL=noreply@yourdomain.com
```

### AWS Requirements

- ✅ IAM User with SNS + SES permissions
- ✅ AWS Access Key ID + Secret
- ✅ SES Email verified
- ✅ SNS SMS enabled in region

---

## 👨‍💻 Development

```bash
# Install dependencies
go mod download

# Format code
go fmt ./...

# Run tests
go test ./...

# Build
go build -o bin/ecommerce

# Run
./bin/ecommerce
```

## 📦 Dependencies

- **Fiber**: Web framework (github.com/gofiber/fiber/v2)
- **GORM**: ORM (gorm.io/gorm)
- **JWT**: Authentication (github.com/golang-jwt/jwt/v5)
- **AWS SDK**: (github.com/aws/aws-sdk-go)
- **PostgreSQL**: Database (gorm.io/driver/postgres)

---

## ✅ Production Checklist

- [ ] AWS credentials secured (use IAM roles, not access keys)
- [ ] Environment variables in secure vault (not .env)
- [ ] Database backups enabled
- [ ] Error handling and logging configured
- [ ] Rate limiting added to endpoints
- [ ] HTTPS enabled (TLS certificate)
- [ ] CORS properly configured
- [ ] AWS spending limits set
- [ ] Monitoring/Alerting configured

---

## 📝 License

MIT

---

## 🤝 Contributing

Pull requests welcome! Please follow Go conventions.

---

## 📞 Support

For issues:
1. Check [AWS_SETUP_GUIDE.md](./AWS_SETUP_GUIDE.md)
2. View application logs
3. Test AWS credentials: `aws sts get-caller-identity`
4. Check AWS CloudWatch

---

**Built with ❤️ using Go, AWS, and Fiber**

