package service

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"strconv"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/model"
	"github.com/flowpay/flowpay-backend/internal/notify"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

var (
	// ErrInvalidMailbox faltan datos o el correo no es válido.
	ErrInvalidMailbox = errors.New("revisa el correo, el servidor SMTP, el usuario y la contraseña")
	// ErrMailboxPasswordRequired la primera configuración necesita contraseña.
	ErrMailboxPasswordRequired = errors.New("indica la contraseña del correo")
	// ErrMailboxInUse ese remitente ya está en otra empresa.
	ErrMailboxInUse = errors.New("ese correo ya está asignado a otra empresa")
)

// MailboxInput lo que el super admin guarda para una empresa.
type MailboxInput struct {
	FromName     string
	FromEmail    string
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
}

// ListCompanyMailboxes buzones para el panel de plataforma. Sin contraseñas.
func (s *Service) ListCompanyMailboxes(ctx context.Context) ([]model.CompanyMailbox, error) {
	list, err := s.Repo.ListCompanyMailboxes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].SMTPPassword = ""
	}
	return list, nil
}

// SaveCompanyMailbox asigna o quita el buzón. Correo vacío lo quita.
func (s *Service) SaveCompanyMailbox(ctx context.Context, companyID int64, in MailboxInput) (*model.CompanyMailbox, error) {
	in.FromName = cleanHeader(in.FromName)
	in.FromEmail = strings.ToLower(strings.TrimSpace(in.FromEmail))
	in.SMTPHost = strings.TrimSpace(in.SMTPHost)
	in.SMTPPort = strings.TrimSpace(in.SMTPPort)
	in.SMTPUsername = strings.TrimSpace(in.SMTPUsername)
	in.SMTPPassword = strings.TrimSpace(in.SMTPPassword)

	if in.FromEmail == "" && in.SMTPHost == "" && in.SMTPUsername == "" && in.SMTPPassword == "" && in.FromName == "" {
		saved, err := s.Repo.UpsertCompanyMailbox(ctx, model.CompanyMailbox{CompanyID: companyID})
		if errors.Is(err, repository.ErrCompanyMissing) {
			return nil, ErrCompanyNotFound
		}
		return saved, err
	}
	if in.SMTPPort == "" {
		in.SMTPPort = "587"
	}
	if _, err := mail.ParseAddress(in.FromEmail); err != nil {
		return nil, ErrInvalidMailbox
	}
	port, err := strconv.Atoi(in.SMTPPort)
	if err != nil || port < 1 || port > 65535 || in.SMTPHost == "" || in.SMTPUsername == "" {
		return nil, ErrInvalidMailbox
	}
	if in.SMTPPassword == "" {
		existing, gerr := s.Repo.GetCompanyMailbox(ctx, companyID)
		if gerr != nil && !errors.Is(gerr, sql.ErrNoRows) {
			return nil, gerr
		}
		if existing == nil || strings.TrimSpace(existing.SMTPPassword) == "" {
			return nil, ErrMailboxPasswordRequired
		}
	}

	saved, err := s.Repo.UpsertCompanyMailbox(ctx, model.CompanyMailbox{
		CompanyID:    companyID,
		FromName:     in.FromName,
		FromEmail:    in.FromEmail,
		SMTPHost:     in.SMTPHost,
		SMTPPort:     in.SMTPPort,
		SMTPUsername: in.SMTPUsername,
		SMTPPassword: in.SMTPPassword,
	})
	if errors.Is(err, repository.ErrCompanyMissing) {
		return nil, ErrCompanyNotFound
	}
	if errors.Is(err, repository.ErrMailboxTaken) {
		return nil, ErrMailboxInUse
	}
	return saved, err
}

// CompanySMTP config de envío de la empresa. Nil si no tiene buzón.
func (s *Service) CompanySMTP(ctx context.Context, companyID int64) *notify.SMTPConfig {
	m, err := s.Repo.GetCompanyMailbox(ctx, companyID)
	if err != nil || m == nil {
		return nil
	}
	cfg := &notify.SMTPConfig{
		Host:     m.SMTPHost,
		Port:     m.SMTPPort,
		Username: m.SMTPUsername,
		Password: m.SMTPPassword,
		From:     m.FromEmail,
		FromName: m.FromName,
	}
	if !cfg.Enabled() {
		return nil
	}
	return cfg
}

func cleanHeader(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}
