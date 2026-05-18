package transbank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client REST Webpay Plus (Transbank).
type Client struct {
	BaseURL      string
	CommerceCode string
	APIKey       string
	HTTP         *http.Client
}

func NewClient(environment, commerceCode, apiKey string) *Client {
	base := "https://webpay3gint.transbank.cl"
	if strings.EqualFold(environment, "production") {
		base = "https://webpay3g.transbank.cl"
	}
	return &Client{
		BaseURL:      strings.TrimSuffix(base, "/"),
		CommerceCode: strings.TrimSpace(commerceCode),
		APIKey:       strings.TrimSpace(apiKey),
		HTTP:         &http.Client{Timeout: 45 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.CommerceCode != "" && c.APIKey != ""
}

type CreateRequest struct {
	BuyOrder   string `json:"buy_order"`
	SessionID  string `json:"session_id"`
	Amount     int64  `json:"amount"`
	ReturnURL  string `json:"return_url"`
}

type CreateResponse struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

type CommitResponse struct {
	Vci                 string     `json:"vci"`
	Amount              float64    `json:"amount"`
	Status              string     `json:"status"`
	BuyOrder            string     `json:"buy_order"`
	SessionID           string     `json:"session_id"`
	CardDetail          *CardDetail `json:"card_detail"`
	AccountingDate      string     `json:"accounting_date"`
	TransactionDate     string     `json:"transaction_date"`
	AuthorizationCode   string     `json:"authorization_code"`
	PaymentTypeCode     string     `json:"payment_type_code"`
	ResponseCode        int        `json:"response_code"`
	InstallmentsAmount  float64    `json:"installments_amount"`
	InstallmentsNumber int        `json:"installments_number"`
}

type CardDetail struct {
	CardNumber string `json:"card_number"`
}

func (c *Client) Create(ctx context.Context, req CreateRequest) (*CreateResponse, []byte, error) {
	var out CreateResponse
	raw, err := c.do(ctx, http.MethodPost, "/rswebpaytransaction/api/webpay/v1.2/transactions", req, &out)
	if err != nil {
		return nil, raw, err
	}
	return &out, raw, nil
}

func (c *Client) Commit(ctx context.Context, token string) (*CommitResponse, []byte, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil, fmt.Errorf("token webpay vacío")
	}
	var out CommitResponse
	path := "/rswebpaytransaction/api/webpay/v1.2/transactions/" + token
	raw, err := c.do(ctx, http.MethodPut, path, nil, &out)
	if err != nil {
		return nil, raw, err
	}
	return &out, raw, nil
}

func (c *Client) Authorized(resp *CommitResponse) bool {
	if resp == nil {
		return false
	}
	if resp.ResponseCode == 0 {
		return true
	}
	return strings.EqualFold(resp.Status, "AUTHORIZED")
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tbk-Api-Key-Id", c.CommerceCode)
	req.Header.Set("Tbk-Api-Key-Secret", c.APIKey)

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return raw, fmt.Errorf("transbank %s %s: %s", method, res.Status, strings.TrimSpace(string(raw)))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return raw, err
		}
	}
	return raw, nil
}

func CardLast4(cardNumber string) string {
	d := digitsOnly(cardNumber)
	if len(d) >= 4 {
		return d[len(d)-4:]
	}
	return ""
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
