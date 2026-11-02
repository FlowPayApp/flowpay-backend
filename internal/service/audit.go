package service

import (
	"context"
	"log"

	"github.com/flowpay/flowpay-backend/internal/audit"
)

func (s *Service) recordReminderFailed(ctx context.Context, companyID, chargeID int64, channel string) {
	summary := "No se pudo enviar el recordatorio por correo"
	if channel == "whatsapp" {
		summary = "No se pudo enviar el recordatorio por WhatsApp"
	}
	s.Record(ctx, audit.Event{
		Domain:     audit.DomainMessage,
		Action:     audit.ActionReminderFailed,
		Outcome:    audit.OutcomeFailure,
		CompanyID:  companyID,
		TargetType: audit.TargetCharge,
		TargetID:   audit.ID(chargeID),
		Summary:    summary,
		Meta:       map[string]any{"channel": channel, "kind": "manual"},
	})
}

func (s *Service) Record(ctx context.Context, e audit.Event) {
	if s == nil || s.Repo == nil {
		return
	}
	if err := s.Repo.RecordAudit(ctx, e); err != nil {
		log.Printf("[FlowPay audit] %s.%s: %v", e.Domain, e.Action, err)
	}
}
