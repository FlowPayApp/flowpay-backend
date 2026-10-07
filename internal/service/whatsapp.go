package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/model"
	"github.com/flowpay/flowpay-backend/internal/notify"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

var (
	// ErrUnknownWhatsAppTo indica que el número "To" no está asociado a ninguna empresa activa.
	ErrUnknownWhatsAppTo = errors.New("whatsapp to no registrado")
	// ErrInvalidWhatsAppNumber el texto no es un teléfono usable.
	ErrInvalidWhatsAppNumber = errors.New("número de WhatsApp inválido; usa código de país, por ejemplo +56912345678")
	// ErrWhatsAppNumberInUse ese Business ya está asignado a otra empresa.
	ErrWhatsAppNumberInUse = errors.New("ese número ya está asignado a otra empresa")
	// ErrCompanyNotFound no hay empresa con ese id.
	ErrCompanyNotFound = errors.New("empresa no encontrada")
)

// WhatsAppService solo procesa webhooks entrantes de Twilio (guardar en messages).
type WhatsAppService struct {
	Repo *repository.DB
}

// HandleInbound guarda mensaje entrante enrutado por número receptor (To).
func (s *WhatsAppService) HandleInbound(ctx context.Context, fromRaw, toRaw, body string) error {
	toNorm := canonicalWhatsApp(toRaw)
	if toNorm == "" {
		return fmt.Errorf("to vacío")
	}
	wn, err := s.Repo.FindWhatsAppNumberByTo(ctx, toNorm)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnknownWhatsAppTo
		}
		return err
	}
	fromNorm := canonicalWhatsApp(fromRaw)
	if fromNorm == "" {
		fromNorm = strings.TrimSpace(fromRaw)
	}
	content := strings.TrimSpace(body)
	var chargeID *int64
	if cid, err := s.Repo.FindOpenChargeIDForInboundWhatsApp(ctx, wn.CompanyID, fromNorm); err == nil && cid != nil {
		chargeID = cid
	} else if err != nil {
		log.Printf("[FlowPay WhatsApp] warn asociando cobro inbound: %v", err)
	}
	log.Printf("[FlowPay WhatsApp] inbound company=%d from=%s to=%s charge_id=%v len=%d", wn.CompanyID, fromNorm, toNorm, chargeID, len(content))
	_, err = s.Repo.InsertMessage(ctx, &model.Message{
		CompanyID:  wn.CompanyID,
		ChargeID:   chargeID,
		FromNumber: fromNorm,
		ToNumber:   toNorm,
		Content:    content,
		Direction:  "inbound",
		Status:     "received",
	})
	if err != nil {
		log.Printf("[FlowPay WhatsApp] error guardando inbound: %v", err)
	}
	return err
}

// ListActiveNumbers números Business asignados (solo activos).
func (s *WhatsAppService) ListActiveNumbers(ctx context.Context) ([]model.WhatsAppNumber, error) {
	return s.Repo.ListActiveWhatsAppNumbers(ctx)
}

// AssignCompanyNumber fija el WhatsApp Business que envía y recibe para la empresa.
// rawPhone vacío quita la asignación.
func (s *WhatsAppService) AssignCompanyNumber(ctx context.Context, companyID int64, rawPhone string) (*model.WhatsAppNumber, error) {
	rawPhone = strings.TrimSpace(rawPhone)
	phone := ""
	if rawPhone != "" {
		phone = canonicalWhatsApp(rawPhone)
		digits := strings.TrimPrefix(phone, "whatsapp:+")
		if phone == "" || len(digits) < 8 || len(digits) > 15 {
			return nil, ErrInvalidWhatsAppNumber
		}
	}
	w, err := s.Repo.SetCompanyWhatsAppNumber(ctx, companyID, phone)
	if err != nil {
		if errors.Is(err, repository.ErrCompanyMissing) {
			return nil, ErrCompanyNotFound
		}
		if errors.Is(err, repository.ErrWhatsAppNumberTaken) {
			return nil, ErrWhatsAppNumberInUse
		}
		return nil, err
	}
	return w, nil
}

func canonicalWhatsApp(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(strings.ToLower(s), "whatsapp:")
	return notify.NormalizeWhatsAppForTwilio(strings.TrimSpace(s))
}
