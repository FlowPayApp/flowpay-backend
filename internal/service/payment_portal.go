package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/domain"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

const (
	paymentPortalFlowpay  = "flowpay"
	paymentPortalExternal = "external"

	auditPaymentPortalFlowpay   = "payment_portal_flowpay"
	auditPaymentPortalSubmitted = "payment_portal_submitted"
	auditPaymentPortalEnabled   = "payment_portal_enabled"
	auditPaymentPortalApproved  = "payment_portal_approved"
	auditPaymentPortalRejected  = "payment_portal_rejected"
)

// PaymentPortalError es un mensaje que puede ver la empresa o el administrador.
type PaymentPortalError struct{ msg string }

func (e *PaymentPortalError) Error() string { return e.msg }

// PaymentPortalView es la elección que ve la empresa.
type PaymentPortalView struct {
	Mode       string `json:"mode"`
	URL        string `json:"url"`
	PendingURL string `json:"pending_url"`
	Rejection  string `json:"rejection"`
}

// SavePaymentPortalInput elige el portal de Geldflus o propone una dirección propia.
type SavePaymentPortalInput struct {
	Mode string `json:"mode"`
	URL  string `json:"url"`
}

// PendingPaymentPortalView es una dirección esperando revisión.
type PendingPaymentPortalView struct {
	CompanyID   int64      `json:"company_id"`
	CompanyName string     `json:"company_name"`
	Mode        string     `json:"mode"`
	URL         string     `json:"url"`
	PendingURL  string     `json:"pending_url"`
	PendingAt   *time.Time `json:"pending_at"`
}

type paymentPortalState struct {
	Mode      string
	URL       string
	Pending   string
	Rejection string
}

func (s *Service) CompanyPaymentPortal(ctx context.Context, companyID int64) (PaymentPortalView, error) {
	p, err := s.Repo.GetPaymentPortal(ctx, companyID)
	if err != nil {
		return PaymentPortalView{}, err
	}
	return presentPaymentPortal(p), nil
}

func (s *Service) SaveCompanyPaymentPortal(ctx context.Context, companyID, actor int64, in SavePaymentPortalInput) (PaymentPortalView, error) {
	current, err := s.Repo.GetPaymentPortal(ctx, companyID)
	if err != nil {
		return PaymentPortalView{}, err
	}
	next, action, err := nextPaymentPortal(portalState(current), in)
	if err != nil {
		return PaymentPortalView{}, err
	}
	if action != "same" {
		if err := s.Repo.SavePaymentPortal(ctx, companyID, repository.PaymentPortal{
			Mode:       next.Mode,
			URL:        next.URL,
			PendingURL: next.Pending,
			Rejection:  next.Rejection,
		}); err != nil {
			return PaymentPortalView{}, err
		}
		s.audit(ctx, actor, 0, companyID, paymentPortalAudit(action))
	}
	return presentPaymentPortal(repository.PaymentPortal{
		Mode:       next.Mode,
		URL:        next.URL,
		PendingURL: next.Pending,
		Rejection:  next.Rejection,
	}), nil
}

func (s *Service) ListPendingPaymentPortals(ctx context.Context) ([]PendingPaymentPortalView, error) {
	rows, err := s.Repo.ListPendingPaymentPortals(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PendingPaymentPortalView, 0, len(rows))
	for _, row := range rows {
		mode := paymentPortalFlowpay
		if strings.EqualFold(strings.TrimSpace(row.Mode), paymentPortalExternal) && strings.TrimSpace(row.URL) != "" {
			mode = paymentPortalExternal
		}
		out = append(out, PendingPaymentPortalView{
			CompanyID:   row.CompanyID,
			CompanyName: row.CompanyName,
			Mode:        mode,
			URL:         strings.TrimSpace(row.URL),
			PendingURL:  strings.TrimSpace(row.PendingURL),
			PendingAt:   row.PendingAt,
		})
	}
	return out, nil
}

func (s *Service) ApproveCompanyPaymentPortal(ctx context.Context, companyID, actor int64) error {
	if err := s.Repo.ApprovePaymentPortal(ctx, companyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &PaymentPortalError{msg: "No hay una dirección pendiente."}
		}
		return err
	}
	s.audit(ctx, actor, 0, companyID, auditPaymentPortalApproved)
	return nil
}

func (s *Service) RejectCompanyPaymentPortal(ctx context.Context, companyID, actor int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return &PaymentPortalError{msg: "Escribe el motivo del rechazo."}
	}
	if len([]rune(reason)) > 240 {
		return &PaymentPortalError{msg: "El motivo es demasiado largo."}
	}
	if err := s.Repo.RejectPaymentPortal(ctx, companyID, reason); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &PaymentPortalError{msg: "No hay una dirección pendiente."}
		}
		return err
	}
	s.audit(ctx, actor, 0, companyID, auditPaymentPortalRejected)
	return nil
}

func nextPaymentPortal(current paymentPortalState, in SavePaymentPortalInput) (paymentPortalState, string, error) {
	cur := tidyPaymentPortal(current)
	switch strings.ToLower(strings.TrimSpace(in.Mode)) {
	case paymentPortalFlowpay:
		if cur.Mode == paymentPortalFlowpay && cur.Pending == "" && cur.Rejection == "" {
			return cur, "same", nil
		}
		return paymentPortalState{Mode: paymentPortalFlowpay, URL: cur.URL}, "flowpay", nil
	case paymentPortalExternal:
		raw := strings.TrimSpace(in.URL)
		if raw == "" {
			if cur.URL == "" {
				return paymentPortalState{}, "", &PaymentPortalError{msg: "Pega la dirección del portal."}
			}
			if cur.Mode == paymentPortalExternal && cur.Pending == "" && cur.Rejection == "" {
				return cur, "same", nil
			}
			return paymentPortalState{Mode: paymentPortalExternal, URL: cur.URL}, "enable", nil
		}
		normalized, err := domain.NormalizePaymentPortalURL(raw)
		if err != nil {
			return paymentPortalState{}, "", &PaymentPortalError{msg: err.Error()}
		}
		if normalized == cur.URL {
			if cur.Mode == paymentPortalExternal && cur.Pending == "" && cur.Rejection == "" {
				return cur, "same", nil
			}
			return paymentPortalState{Mode: paymentPortalExternal, URL: cur.URL}, "enable", nil
		}
		if normalized == cur.Pending && cur.Rejection == "" {
			return cur, "same", nil
		}
		return paymentPortalState{Mode: cur.Mode, URL: cur.URL, Pending: normalized}, "pending", nil
	default:
		return paymentPortalState{}, "", &PaymentPortalError{msg: "Elige el portal de Geldflus o el de la empresa."}
	}
}

func tidyPaymentPortal(s paymentPortalState) paymentPortalState {
	s.Mode = strings.ToLower(strings.TrimSpace(s.Mode))
	s.URL = strings.TrimSpace(s.URL)
	s.Pending = strings.TrimSpace(s.Pending)
	s.Rejection = strings.TrimSpace(s.Rejection)
	if s.Mode != paymentPortalExternal || s.URL == "" {
		s.Mode = paymentPortalFlowpay
	}
	return s
}

func portalState(p repository.PaymentPortal) paymentPortalState {
	return paymentPortalState{Mode: p.Mode, URL: p.URL, Pending: p.PendingURL, Rejection: p.Rejection}
}

func presentPaymentPortal(p repository.PaymentPortal) PaymentPortalView {
	state := tidyPaymentPortal(portalState(p))
	rejection := state.Rejection
	if state.Pending != "" {
		rejection = ""
	}
	return PaymentPortalView{
		Mode:       state.Mode,
		URL:        state.URL,
		PendingURL: state.Pending,
		Rejection:  rejection,
	}
}

func paymentPortalAudit(action string) string {
	switch action {
	case "flowpay":
		return auditPaymentPortalFlowpay
	case "enable":
		return auditPaymentPortalEnabled
	default:
		return auditPaymentPortalSubmitted
	}
}
