package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/paymenttoken"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

// Estados del ENUM public.payment_token_status en PostgreSQL.
const (
	PaymentTokenStatusIssued  = "issued"
	PaymentTokenStatusViewed  = "viewed"
	PaymentTokenStatusPaid    = "paid"
	PaymentTokenStatusRevoked = "revoked"
)

// ErrPaymentTokenNotFound indica que el token no existe (o fue revocado).
var ErrPaymentTokenNotFound = errors.New("token de pago no encontrado")

// IssuePaymentToken crea un registro de token de pago para la empresa y el cliente indicados.
// Valida que el cliente pertenezca a la empresa antes de persistir.
func (s *Service) IssuePaymentToken(ctx context.Context, companyID, clientID int64) (*repository.PaymentTokenRow, error) {
	if companyID <= 0 || clientID <= 0 {
		return nil, errors.New("company_id y client_id deben ser positivos")
	}
	ok, err := s.Repo.ClientBelongsToCompany(ctx, companyID, clientID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("el cliente no pertenece a la empresa")
	}
	tok, err := paymenttoken.NewOpaque()
	if err != nil {
		return nil, err
	}
	return s.Repo.InsertPaymentToken(ctx, companyID, clientID, tok, PaymentTokenStatusIssued)
}

// PaymentPortalCompany datos visibles de la empresa que cobra; NO incluye IDs internos.
type PaymentPortalCompany struct {
	Name                 string `json:"name"`
	TransferInstructions string `json:"transfer_instructions,omitempty"`
}

// PaymentPortalClient datos visibles del cliente; NO incluye IDs internos.
type PaymentPortalClient struct {
	Label string `json:"label"`
}

// PaymentPortalResponse payload del endpoint público /api/public/pay/:token.
type PaymentPortalResponse struct {
	TokenStatus string               `json:"token_status"`
	IssuedAt    time.Time            `json:"issued_at"`
	Company     PaymentPortalCompany `json:"company"`
	Client      PaymentPortalClient  `json:"client"`
	Charges     []PortalCharge       `json:"charges"`
	Totals      PaymentPortalTotals  `json:"totals"`
}

// PaymentPortalTotals montos sumarizados de la cartola del cliente.
type PaymentPortalTotals struct {
	Pending float64 `json:"pending"`
	Overdue float64 `json:"overdue"`
	Paid    float64 `json:"paid"`
}

// ResolvePaymentToken arma la respuesta pública para /pay/:token.
func (s *Service) ResolvePaymentToken(ctx context.Context, tokenValue string) (*PaymentPortalResponse, error) {
	tokenValue = strings.TrimSpace(tokenValue)
	if tokenValue == "" {
		return nil, ErrPaymentTokenNotFound
	}
	row, err := s.Repo.GetPaymentTokenByValue(ctx, tokenValue)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPaymentTokenNotFound
		}
		return nil, err
	}
	if row.Status == PaymentTokenStatusIssued {
		if changed, mErr := s.Repo.MarkPaymentTokenViewed(ctx, row.Token); mErr == nil && changed {
			row.Status = PaymentTokenStatusViewed
		}
	}
	cm, err := s.Repo.GetCompanyMessaging(ctx, row.CompanyID)
	if err != nil {
		return nil, err
	}
	label, err := s.Repo.GetClientLabel(ctx, row.CompanyID, row.ClientID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			label = ""
		} else {
			return nil, err
		}
	}
	charges, err := s.Repo.ListChargesByClient(ctx, row.CompanyID, row.ClientID)
	if err != nil {
		return nil, err
	}
	out := &PaymentPortalResponse{
		TokenStatus: row.Status,
		IssuedAt:    row.CreatedAt,
		Company: PaymentPortalCompany{
			Name:                 cm.Name,
			TransferInstructions: cm.TransferInstructions,
		},
		Client: PaymentPortalClient{
			Label: label,
		},
		Charges: make([]PortalCharge, 0, len(charges)),
	}
	for _, ch := range charges {
		dto := s.withStatus(ch)
		pc := PortalCharge{
			Ref:             encodePortalChargeRef(row.Token, ch.ID),
			Amount:          ch.Amount,
			DueDate:         ch.DueDate,
			Status:          dto.Status,
			AttachmentToken: ch.AttachmentToken,
		}
		out.Charges = append(out.Charges, pc)
		switch dto.Status {
		case "paid":
			out.Totals.Paid += ch.Amount
		case "overdue":
			out.Totals.Overdue += ch.Amount
		default:
			out.Totals.Pending += ch.Amount
		}
	}
	return out, nil
}

// PortalCharge representación pública de un cobro: sin IDs de DB, sólo una ref opaca por token.
type PortalCharge struct {
	Ref             string    `json:"ref"`
	Amount          float64   `json:"amount"`
	DueDate         time.Time `json:"due_date"`
	Status          string    `json:"status"`
	AttachmentToken *string   `json:"attachment_token,omitempty"`
}

// encodePortalChargeRef genera una referencia estable por (token, chargeID) sin revelar el id de DB.
// Usa HMAC-SHA256 truncado y base64url. La estabilidad depende del par token+chargeID, no de un secreto global.
func encodePortalChargeRef(tokenValue string, chargeID int64) string {
	mac := hmac.New(sha256.New, []byte(tokenValue))
	mac.Write([]byte(strconv.FormatInt(chargeID, 10)))
	sum := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sum[:12])
}
