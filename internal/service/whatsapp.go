package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/models"
	"github.com/flowpay/flowpay-backend/internal/notify"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

var (
	// ErrUnknownWhatsAppTo indica que el número "To" no está asociado a ninguna empresa activa.
	ErrUnknownWhatsAppTo = errors.New("whatsapp to no registrado")
)

// WhatsAppService procesa webhooks entrantes de Twilio (guardar en messages).
type WhatsAppService struct {
	Repo *repository.Repository
}

// HandleInbound guarda mensaje entrante enrutado por número receptor (To).
func (s *WhatsAppService) HandleInbound(ctx context.Context, fromRaw, toRaw, body string) error {
	toNorm := notify.NormalizeWhatsAppForTwilio(strings.TrimSpace(toRaw))
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
	fromNorm := notify.NormalizeWhatsAppForTwilio(strings.TrimSpace(fromRaw))
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
	_, err = s.Repo.InsertMessage(ctx, &models.Message{
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
