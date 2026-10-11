package service

import (
	"context"
	"errors"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/domain"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

var (
	// ErrReminderBadMode modo de recordatorios desconocido.
	ErrReminderBadMode = errors.New("modo de recordatorios no válido; usa company, custom u off")
	// ErrReminderBadAutoChannel canal de recordatorios automáticos desconocido.
	ErrReminderBadAutoChannel = errors.New("canal no válido; deja vacío para usar el de la sucursal o usa all, whatsapp o email")
)

// ChargeRemindersInput cambios a los recordatorios automáticos de un cobro; los campos nulos no se tocan.
type ChargeRemindersInput struct {
	ReminderMode    *string                `json:"reminder_mode"`
	ReminderChannel *string                `json:"reminder_channel"`
	ReminderPolicy  *domain.ReminderPolicy `json:"reminder_policy"`
}

func (in ChargeRemindersInput) any() bool {
	return in.ReminderMode != nil || in.ReminderChannel != nil || in.ReminderPolicy != nil
}

// mergeChargeReminders aplica in sobre la configuración actual del cobro y la valida.
// Al pasar a personalizada sin política propia, parte de la de la empresa.
func (s *Service) mergeChargeReminders(ctx context.Context, companyID int64, cur repository.ChargeReminderSettings, in ChargeRemindersInput) (repository.ChargeReminderSettings, error) {
	next := cur
	if in.ReminderMode != nil {
		next.Mode = strings.TrimSpace(strings.ToLower(*in.ReminderMode))
	}
	switch next.Mode {
	case "":
		next.Mode = domain.ReminderModeCompany
	case domain.ReminderModeCompany, domain.ReminderModeCustom, domain.ReminderModeOff:
	default:
		return cur, ErrReminderBadMode
	}
	if in.ReminderChannel != nil {
		next.Channel = strings.TrimSpace(strings.ToLower(*in.ReminderChannel))
	}
	switch next.Channel {
	case "", "all", "whatsapp", "email":
	default:
		return cur, ErrReminderBadAutoChannel
	}
	if in.ReminderPolicy != nil {
		next.Policy = *in.ReminderPolicy
	}
	if next.Mode == domain.ReminderModeCustom {
		if in.ReminderPolicy == nil && next.Policy.OverdueEvery == 0 {
			base, err := s.Repo.GetCompanyReminderPolicy(ctx, companyID)
			if err != nil {
				return cur, err
			}
			next.Policy = base
		}
		p, err := next.Policy.Normalized()
		if err != nil {
			return cur, err
		}
		next.Policy = p
	}
	return next, nil
}

// withReminders completa el detalle del cobro con la frecuencia que lo rige.
func (s *Service) withReminders(ctx context.Context, dto *ChargeDTO) error {
	set, err := s.Repo.GetChargeReminderSettings(ctx, dto.ID)
	if err != nil {
		return err
	}
	company, err := s.Repo.GetCompanyReminderPolicy(ctx, dto.CompanyID)
	if err != nil {
		return err
	}
	dto.ReminderMode = set.Mode
	dto.ReminderChannel = set.Channel
	dto.CompanyReminderPolicy = &company
	policy := company
	if set.Mode == domain.ReminderModeCustom {
		policy = set.Policy
	}
	dto.ReminderPolicy = &policy
	return nil
}
