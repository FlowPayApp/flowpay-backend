package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/paymenttoken"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

// Estado inicial de un token de pago recién emitido.
const PaymentTokenStatusIssued = "issued"

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

// PaymentPortalCompany identifica a la empresa que cobra en la página pública.
type PaymentPortalCompany struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	TransferInstructions string `json:"transfer_instructions,omitempty"`
}

// PaymentPortalClient identifica al cliente al que está dirigida la página.
type PaymentPortalClient struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
}

// PaymentPortalResponse payload del endpoint público /api/public/pay/:token.
type PaymentPortalResponse struct {
	TokenStatus string                `json:"token_status"`
	IssuedAt    time.Time             `json:"issued_at"`
	Company     PaymentPortalCompany  `json:"company"`
	Client      PaymentPortalClient   `json:"client"`
	Charges     []ChargeDTO           `json:"charges"`
	Totals      PaymentPortalTotals   `json:"totals"`
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
			ID:                   row.CompanyID,
			Name:                 cm.Name,
			TransferInstructions: cm.TransferInstructions,
		},
		Client: PaymentPortalClient{
			ID:    row.ClientID,
			Label: label,
		},
		Charges: make([]ChargeDTO, 0, len(charges)),
	}
	for _, ch := range charges {
		dto := s.withStatus(ch)
		out.Charges = append(out.Charges, dto)
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
