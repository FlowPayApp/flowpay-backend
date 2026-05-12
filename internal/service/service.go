package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/domain"
	"github.com/flowpay/flowpay-backend/internal/models"
	"github.com/flowpay/flowpay-backend/internal/notify"
	"github.com/flowpay/flowpay-backend/internal/remindercontent"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

type ChargeDTO struct {
	repository.Charge
	Status string `json:"status"`
}

type DashboardResponse struct {
	Totals               repository.DashboardTotals `json:"totals"`
	ChargesNeedAttention []ChargeDTO                `json:"charges_needing_attention"`
	Tagline              string                     `json:"tagline"`
	ProductName          string                     `json:"product_name"`
}

type PlatformOverviewResponse struct {
	Companies      []repository.CompanyOverviewRow `json:"companies"`
	TotalCompanies int                             `json:"total_companies"`
	TotalPaid      float64                         `json:"total_paid"`
	TotalPending   float64                         `json:"total_pending"`
	TotalOverdue   float64                         `json:"total_overdue"`
	TotalOwed      float64                         `json:"total_owed"`
}

type Service struct {
	Repo      *repository.Repository
	Notify    *notify.Dispatcher
	UploadDir string
}

func (s *Service) withStatus(ch repository.Charge) ChargeDTO {
	st := domain.ChargeStatus(ch.PaidAt, ch.DueDate, time.Now())
	return ChargeDTO{Charge: ch, Status: st}
}

func (s *Service) ListCharges(ctx context.Context, companyID int64) ([]ChargeDTO, error) {
	list, err := s.Repo.ListCharges(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]ChargeDTO, 0, len(list))
	for _, ch := range list {
		out = append(out, s.withStatus(ch))
	}
	return out, nil
}

func (s *Service) GetCharge(ctx context.Context, companyID, id int64) (*ChargeDTO, error) {
	ch, err := s.Repo.GetCharge(ctx, companyID, id)
	if err != nil {
		return nil, err
	}
	dto := s.withStatus(*ch)
	return &dto, nil
}

type CreateChargeInput struct {
	ClientID int64   `json:"client_id"`
	Amount   float64 `json:"amount"`
	DueDate  string  `json:"due_date"`
}

func (s *Service) CreateCharge(ctx context.Context, companyID int64, in CreateChargeInput) (int64, error) {
	if in.ClientID == 0 || in.Amount <= 0 || in.DueDate == "" {
		return 0, errors.New("payload de cobro inválido")
	}
	due, err := time.ParseInLocation("2006-01-02", in.DueDate, time.Local)
	if err != nil {
		return 0, errors.New("due_date debe ser YYYY-MM-DD")
	}
	return s.Repo.CreateCharge(ctx, companyID, in.ClientID, in.Amount, due)
}

func (s *Service) DeleteCharge(ctx context.Context, companyID, chargeID int64) error {
	return s.Repo.DeleteCharge(ctx, companyID, chargeID)
}

func (s *Service) Dashboard(ctx context.Context, companyID int64) (*DashboardResponse, error) {
	totals, err := s.Repo.DashboardAggregate(ctx, companyID)
	if err != nil {
		return nil, err
	}
	overdue, _ := s.Repo.ChargesOverdueUnpaid(ctx, companyID)
	dueSoon, _ := s.Repo.ChargesDueSoon(ctx, companyID, 7)
	seen := map[int64]struct{}{}
	var attention []ChargeDTO
	for _, ch := range overdue {
		if _, ok := seen[ch.ID]; ok {
			continue
		}
		seen[ch.ID] = struct{}{}
		attention = append(attention, s.withStatus(ch))
	}
	for _, ch := range dueSoon {
		if _, ok := seen[ch.ID]; ok {
			continue
		}
		seen[ch.ID] = struct{}{}
		attention = append(attention, s.withStatus(ch))
	}
	return &DashboardResponse{
		Totals:               totals,
		ChargesNeedAttention: attention,
		Tagline:              "Te ayudamos a cobrar más rápido, automáticamente.",
		ProductName:          "FlowPay",
	}, nil
}

func (s *Service) SendReminderNow(ctx context.Context, companyID, chargeID int64) error {
	ch, err := s.Repo.GetCharge(ctx, companyID, chargeID)
	if err != nil {
		return err
	}
	st := domain.ChargeStatus(ch.PaidAt, ch.DueDate, time.Now())
	if st == "paid" {
		return errors.New("cobro ya cerrado")
	}

	channel := strings.TrimSpace(strings.ToLower(ch.ClientFollowupChannel))
	if channel == "" {
		channel = "all"
	}
	if channel == "none" {
		return errors.New("cliente con seguimiento desactivado (none)")
	}

	priorOverdue, _ := s.Repo.CountRemindersByKind(ctx, chargeID, "overdue")
	now := time.Now()
	phase, daysU := remindercontent.PhaseFromCharge(*ch, now, priorOverdue)
	subj, textBody, resErr := remindercontent.ResolveSubjectAndBody(ctx, s.Repo, companyID, phase, daysU, priorOverdue, *ch)
	if resErr != nil {
		subj, textBody = manualReminderTemplate(*ch, priorOverdue, now)
	}
	emailMessage := fmt.Sprintf("Asunto: %s\n\n%s", subj, textBody)
	whatsAppMessage := textBody

	sendEmail := channel == "all" || channel == "email"
	sendWhatsApp := channel == "all" || channel == "whatsapp"

	if s.Notify != nil {
		switch {
		case sendEmail && sendWhatsApp:
			s.Notify.SendReminderEmail(*ch, subj, textBody)
			s.Notify.SendReminderWhatsApp(*ch, textBody)
		case sendEmail:
			s.Notify.SendReminderEmail(*ch, subj, textBody)
		case sendWhatsApp:
			s.Notify.SendReminderWhatsApp(*ch, textBody)
		}
	}

	if sendEmail {
		if _, err := s.Repo.InsertReminder(ctx, chargeID, "manual", "email", "sent", emailMessage, &now); err != nil {
			return err
		}
	}
	if sendWhatsApp {
		if _, err := s.Repo.InsertReminder(ctx, chargeID, "manual", "whatsapp", "sent", whatsAppMessage, &now); err != nil {
			return err
		}
	}
	return nil
}

// MessagingSettingsResponse plantillas + textos globales para recordatorios.
type MessagingSettingsResponse struct {
	TransferInstructions string                        `json:"transfer_instructions"`
	PaymentURLTemplate    string                        `json:"payment_url_template"`
	Templates             []repository.ReminderTemplateRow `json:"templates"`
}

// MessagingTemplateInput fila de plantilla desde el panel.
type MessagingTemplateInput struct {
	Phase        string `json:"phase"`
	DayMin       int    `json:"day_min"`
	DayMax       int    `json:"day_max"`
	SortOrder    int    `json:"sort_order"`
	EmailSubject string `json:"email_subject"`
	Body         string `json:"body"`
}

// SaveMessagingInput PUT /api/company/messaging
type SaveMessagingInput struct {
	TransferInstructions string                   `json:"transfer_instructions"`
	PaymentURLTemplate   string                   `json:"payment_url_template"`
	Templates            []MessagingTemplateInput `json:"templates"`
}

func (s *Service) GetCompanyMessagingSettings(ctx context.Context, companyID int64) (*MessagingSettingsResponse, error) {
	cm, err := s.Repo.GetCompanyMessaging(ctx, companyID)
	if err != nil {
		return nil, err
	}
	tpl, err := s.Repo.ListReminderTemplates(ctx, companyID)
	if err != nil {
		tpl = nil
	}
	return &MessagingSettingsResponse{
		TransferInstructions: cm.TransferInstructions,
		PaymentURLTemplate:   cm.PaymentURLTemplate,
		Templates:            tpl,
	}, nil
}

func (s *Service) SaveCompanyMessagingSettings(ctx context.Context, companyID int64, in SaveMessagingInput) error {
	validPhases := map[string]struct{}{
		remindercontent.PhaseApproaching:     {},
		remindercontent.PhaseDueToday:        {},
		remindercontent.PhaseOverdueFirst:    {},
		remindercontent.PhaseOverdueFollowUp: {},
	}
	for _, t := range in.Templates {
		p := strings.ToLower(strings.TrimSpace(t.Phase))
		if p == "" {
			continue
		}
		if _, ok := validPhases[p]; !ok {
			return fmt.Errorf("fase inválida: %s (use approaching|due_today|overdue_first|overdue_followup)", t.Phase)
		}
		if p == remindercontent.PhaseApproaching && t.DayMin > t.DayMax {
			return errors.New("En plantillas approaching, day_min no puede ser mayor que day_max")
		}
	}
	if err := s.Repo.UpdateCompanyMessaging(ctx, companyID, in.TransferInstructions, in.PaymentURLTemplate); err != nil {
		return err
	}
	rows := make([]repository.ReminderTemplateRow, 0, len(in.Templates))
	for _, t := range in.Templates {
		if strings.TrimSpace(t.Body) == "" {
			continue
		}
		p := strings.ToLower(strings.TrimSpace(t.Phase))
		rows = append(rows, repository.ReminderTemplateRow{
			Phase:        p,
			DayMin:       t.DayMin,
			DayMax:       t.DayMax,
			SortOrder:    t.SortOrder,
			EmailSubject: t.EmailSubject,
			Body:         t.Body,
		})
	}
	return s.Repo.ReplaceReminderTemplates(ctx, companyID, rows)
}

func manualReminderTemplate(ch repository.Charge, priorOverdueReminders int, now time.Time) (subject string, body string) {
	t0 := dateOnly(now)
	t1 := dateOnly(ch.DueDate)
	switch {
	case t0.Before(t1):
		return "Recordatorio: cobro próximo a vencer", notify.BodyApproaching(ch)
	case t0.Equal(t1):
		return "Hoy vence un cobro pendiente", notify.BodyDueToday(ch)
	default:
		if priorOverdueReminders == 0 {
			return "Cobro vencido — acción requerida", notify.BodyOverdueFirst(ch)
		}
		return "Seguimiento de cobro pendiente", notify.BodyOverdueFollowUp(ch)
	}
}

func dateOnly(t time.Time) time.Time {
	y, m, day := t.Date()
	return time.Date(y, m, day, 0, 0, 0, 0, t.Location())
}

func (s *Service) ListReminders(ctx context.Context, companyID, chargeID int64) ([]repository.Reminder, error) {
	if _, err := s.Repo.GetCharge(ctx, companyID, chargeID); err != nil {
		return nil, err
	}
	return s.Repo.ListReminders(ctx, chargeID)
}

// ListChargeInboundWhatsApp respuestas del cliente (WhatsApp entrante) vinculadas al cobro.
func (s *Service) ListChargeInboundWhatsApp(ctx context.Context, companyID, chargeID int64) ([]models.Message, error) {
	if _, err := s.Repo.GetCharge(ctx, companyID, chargeID); err != nil {
		return nil, err
	}
	return s.Repo.ListInboundMessagesForCharge(ctx, companyID, chargeID)
}

// SimulateChargeInboundWhatsApp inserta un mensaje entrante de prueba vinculado al cobro (demo / QA).
func (s *Service) SimulateChargeInboundWhatsApp(ctx context.Context, companyID, chargeID int64, text string) (*models.Message, error) {
	ch, err := s.Repo.GetCharge(ctx, companyID, chargeID)
	if err != nil {
		return nil, err
	}
	msg := strings.TrimSpace(text)
	if msg == "" {
		msg = "Hola, recibí el aviso. ¿A qué cuenta transfiero?"
	}
	fromNorm := "whatsapp:+56900000001"
	if ch.ClientPhone != nil {
		n := notify.NormalizeWhatsAppForTwilio(*ch.ClientPhone)
		if n != "" {
			fromNorm = n
		}
	}
	toNorm := "whatsapp:+10000000000"
	if tn, err := s.Repo.FirstActiveWhatsAppToForCompany(ctx, companyID); err == nil && strings.TrimSpace(tn) != "" {
		toNorm = strings.TrimSpace(tn)
	}
	cid := chargeID
	m := &models.Message{
		CompanyID:  companyID,
		ChargeID:   &cid,
		FromNumber: fromNorm,
		ToNumber:   toNorm,
		Content:    msg,
		Direction:  "inbound",
		Status:     "received",
	}
	id, err := s.Repo.InsertMessage(ctx, m)
	if err != nil {
		return nil, err
	}
	return s.Repo.GetMessageByID(ctx, companyID, id)
}

// PatchChargeInput: campos opcionales. set_paid true = marcar cobrado hoy; false = reabrir (quita pagos).
type PatchChargeInput struct {
	ClientID *int64   `json:"client_id"`
	DueDate  *string  `json:"due_date"`
	Amount   *float64 `json:"amount"`
	SetPaid  *bool    `json:"set_paid"`
}

func (s *Service) PatchCharge(ctx context.Context, companyID, chargeID int64, in PatchChargeInput) error {
	has := in.ClientID != nil || in.DueDate != nil || in.Amount != nil || in.SetPaid != nil
	if !has {
		return errors.New("nada que actualizar")
	}
	if in.ClientID != nil {
		ok, err := s.Repo.ActiveClientBelongsToCompany(ctx, companyID, *in.ClientID)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("cliente no válido o inactivo para esta empresa")
		}
	}
	if in.Amount != nil && *in.Amount <= 0 {
		return errors.New("amount debe ser mayor a 0")
	}
	var duePtr *time.Time
	if in.DueDate != nil && *in.DueDate != "" {
		t, err := time.ParseInLocation("2006-01-02", *in.DueDate, time.Local)
		if err != nil {
			return errors.New("due_date debe ser YYYY-MM-DD")
		}
		duePtr = &t
	}

	if err := s.Repo.UpdateChargeFields(ctx, companyID, chargeID, in.ClientID, duePtr, in.Amount); err != nil {
		return err
	}

	if in.SetPaid == nil {
		return nil
	}
	if !*in.SetPaid {
		return s.Repo.ClearChargePayment(ctx, companyID, chargeID)
	}
	ch, err := s.Repo.GetCharge(ctx, companyID, chargeID)
	if err != nil {
		return err
	}
	if ch.PaidAt != nil {
		return nil
	}
	return s.Repo.MarkChargePaid(ctx, chargeID, ch.Amount)
}

func (s *Service) RecordPayment(ctx context.Context, companyID, chargeID int64, amount float64) error {
	ch, err := s.Repo.GetCharge(ctx, companyID, chargeID)
	if err != nil {
		return err
	}
	if ch.PaidAt != nil {
		return errors.New("already paid")
	}
	if amount < ch.Amount-0.01 {
		return errors.New("amount must cover charge total for MVP")
	}
	return s.Repo.MarkChargePaid(ctx, chargeID, amount)
}

func (s *Service) PlatformOverview(ctx context.Context) (*PlatformOverviewResponse, error) {
	rows, err := s.Repo.PlatformCompaniesOverview(ctx)
	if err != nil {
		return nil, err
	}
	out := &PlatformOverviewResponse{
		Companies:      rows,
		TotalCompanies: len(rows),
	}
	for _, r := range rows {
		out.TotalPaid += r.PaidAmount
		out.TotalPending += r.PendingAmount
		out.TotalOverdue += r.OverdueAmount
		out.TotalOwed += r.OwedAmount
	}
	return out, nil
}

func ErrNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
