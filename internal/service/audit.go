package service

import (
	"context"
	"log"

	"github.com/flowpay/flowpay-backend/internal/audit"
)

func (s *Service) recordReminderFailed(ctx context.Context, companyID, chargeID int64, channel string, cause error) {
	summary, meta := audit.ReminderFailure("manual", channel, cause)
	s.Record(ctx, audit.Event{
		Domain:     audit.DomainMessage,
		Action:     audit.ActionReminderFailed,
		Outcome:    audit.OutcomeFailure,
		CompanyID:  companyID,
		TargetType: audit.TargetCharge,
		TargetID:   audit.ID(chargeID),
		Summary:    summary,
		Meta:       meta,
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
