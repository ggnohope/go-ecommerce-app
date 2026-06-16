package payment

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const payosCreateURL = "https://api-merchant.payos.vn/v2/payment-requests"

// PayOSClient talks to the payOS merchant API using raw HTTP + HMAC-SHA256,
// mirroring how the rest of this repo wraps third-party services directly
// (no heavyweight SDK dependency).
type PayOSClient struct {
	clientID    string
	apiKey      string
	checksumKey string
	returnURL   string
	cancelURL   string
	http        *http.Client
}

func NewPayOSClient(clientID, apiKey, checksumKey, returnURL, cancelURL string) *PayOSClient {
	return &PayOSClient{
		clientID:    clientID,
		apiKey:      apiKey,
		checksumKey: checksumKey,
		returnURL:   returnURL,
		cancelURL:   cancelURL,
		http:        &http.Client{Timeout: 15 * time.Second},
	}
}

// PaymentLink is what the frontend needs to collect a payment: redirect the
// buyer to CheckoutURL, or render QRCode for a bank-transfer/VietQR scan.
type PaymentLink struct {
	CheckoutURL   string `json:"checkout_url"`
	QRCode        string `json:"qr_code"`
	PaymentLinkID string `json:"payment_link_id"`
	OrderCode     int64  `json:"order_code"`
	Amount        int64  `json:"amount"`
}

// WebhookData is the verified payload payOS sends when a payment settles.
type WebhookData struct {
	OrderCode int64
	Amount    int64
	Reference string
}

type createRequest struct {
	OrderCode   int64  `json:"orderCode"`
	Amount      int64  `json:"amount"`
	Description string `json:"description"`
	CancelURL   string `json:"cancelUrl"`
	ReturnURL   string `json:"returnUrl"`
	Signature   string `json:"signature"`
}

type createResponse struct {
	Code string `json:"code"`
	Desc string `json:"desc"`
	Data *struct {
		OrderCode     int64  `json:"orderCode"`
		Amount        int64  `json:"amount"`
		CheckoutURL   string `json:"checkoutUrl"`
		QRCode        string `json:"qrCode"`
		PaymentLinkID string `json:"paymentLinkId"`
		Status        string `json:"status"`
	} `json:"data"`
}

// CreatePaymentLink asks payOS for a hosted checkout link + QR for an order.
// orderCode is the integer payOS uses to identify the payment; we use the
// order's own ID. amountVND is the amount in Vietnamese dong (integer).
func (c *PayOSClient) CreatePaymentLink(orderID uint, amountVND int64) (*PaymentLink, error) {
	// payOS limits description to 9 chars for non-payOS bank accounts.
	description := fmt.Sprintf("DH%d", orderID)

	// Signature is HMAC-SHA256 over the fixed field set in alphabetical order.
	signed := fmt.Sprintf("amount=%d&cancelUrl=%s&description=%s&orderCode=%d&returnUrl=%s",
		amountVND, c.cancelURL, description, int64(orderID), c.returnURL)
	signature := c.sign(signed)

	reqBody, err := json.Marshal(createRequest{
		OrderCode:   int64(orderID),
		Amount:      amountVND,
		Description: description,
		CancelURL:   c.cancelURL,
		ReturnURL:   c.returnURL,
		Signature:   signature,
	})
	if err != nil {
		return nil, fmt.Errorf("payos: marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, payosCreateURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("payos: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-client-id", c.clientID)
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("payos: request failed: %w", err)
	}
	defer resp.Body.Close()

	var out createResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("payos: decode response: %w", err)
	}
	if out.Code != "00" || out.Data == nil {
		return nil, fmt.Errorf("payos: create link failed: code=%s desc=%s", out.Code, out.Desc)
	}

	return &PaymentLink{
		CheckoutURL:   out.Data.CheckoutURL,
		QRCode:        out.Data.QRCode,
		PaymentLinkID: out.Data.PaymentLinkID,
		OrderCode:     out.Data.OrderCode,
		Amount:        out.Data.Amount,
	}, nil
}

// VerifyWebhook checks the signature on a payOS webhook body and returns the
// payment data. payOS signs the `data` object: sort its keys alphabetically,
// join as `key=value&...`, then HMAC-SHA256 with the checksum key.
func (c *PayOSClient) VerifyWebhook(body []byte) (*WebhookData, error) {
	var envelope struct {
		Data      json.RawMessage `json:"data"`
		Signature string          `json:"signature"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("payos: decode webhook: %w", err)
	}
	if len(envelope.Data) == 0 {
		return nil, fmt.Errorf("payos: webhook has no data")
	}

	// Decode data keeping numbers as-is (json.Number) so the signature string
	// matches payOS exactly (no float scientific notation).
	dec := json.NewDecoder(bytes.NewReader(envelope.Data))
	dec.UseNumber()
	var data map[string]interface{}
	if err := dec.Decode(&data); err != nil {
		return nil, fmt.Errorf("payos: decode webhook data: %w", err)
	}

	if !hmac.Equal([]byte(c.sign(sortedQuery(data))), []byte(envelope.Signature)) {
		return nil, fmt.Errorf("payos: webhook signature mismatch")
	}

	out := &WebhookData{}
	if v, ok := data["orderCode"].(json.Number); ok {
		out.OrderCode, _ = v.Int64()
	}
	if v, ok := data["amount"].(json.Number); ok {
		out.Amount, _ = v.Int64()
	}
	if v, ok := data["reference"].(string); ok {
		out.Reference = v
	}
	return out, nil
}

// sortedQuery builds the `key=value&...` string payOS signs: keys sorted
// alphabetically, null rendered as empty string, nested values as JSON.
func sortedQuery(data map[string]interface{}) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+valueToString(data[k]))
	}
	return strings.Join(parts, "&")
}

func valueToString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func (c *PayOSClient) sign(payload string) string {
	mac := hmac.New(sha256.New, []byte(c.checksumKey))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
