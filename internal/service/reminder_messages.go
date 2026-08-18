package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/repository"
)

type CreateReminderMessageInput struct {
	CompanyID int64  `json:"company_id"`
	Message   string `json:"message"`
	Type      string `json:"type"`
	IsActive  *bool  `json:"is_active"`
}

type PatchReminderMessageInput struct {
	Message  *string `json:"message"`
	IsActive *bool   `json:"is_active"`
}

func (s *Service) CreateReminderMessage(ctx context.Context, companyID int64, in CreateReminderMessageInput) (*repository.ReminderMessage, error) {
	if companyID <= 0 {
		return nil, errors.New("company_id es obligatorio")
	}
	msg := strings.TrimSpace(in.Message)
	if msg == "" {
		return nil, errors.New("message es obligatorio")
	}
	msgType := strings.TrimSpace(in.Type)
	if msgType == "" {
		return nil, errors.New("type es obligatorio")
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	return s.Repo.CreateReminderMessage(ctx, companyID, &active, msg, msgType)
}

func (s *Service) PatchReminderMessage(ctx context.Context, id, companyID int64, platformAdmin bool, in PatchReminderMessageInput) (*repository.ReminderMessage, error) {
	if id <= 0 {
		return nil, errors.New("id inválido")
	}
	if in.Message == nil && in.IsActive == nil {
		return nil, errors.New("nada que actualizar")
	}
	if in.Message != nil {
		trimmed := strings.TrimSpace(*in.Message)
		in.Message = &trimmed
	}

	existing, err := s.Repo.GetReminderMessage(ctx, id)
	if err != nil {
		return nil, err
	}
	if !platformAdmin && existing.CompanyID != companyID {
		return nil, sql.ErrNoRows
	}

	var tenant *int64
	if !platformAdmin {
		tenant = &companyID
	}
	if err := s.Repo.UpdateReminderMessage(ctx, id, tenant, in.Message, in.IsActive); err != nil {
		return nil, err
	}
	return s.Repo.GetReminderMessage(ctx, id)
}
