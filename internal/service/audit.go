package service

import (
	"context"
	"log"
)

const (
	auditChargeMarkedPaid = "charge_marked_paid"
	auditClientsExported  = "clients_exported"
)

func (s *Service) audit(ctx context.Context, actor, target, company int64, action string) {
	if s == nil || s.Repo == nil {
		return
	}
	if err := s.Repo.InsertAudit(ctx, actor, target, company, action); err != nil {
		log.Printf("[FlowPay audit] %s: %v", action, err)
	}
}
