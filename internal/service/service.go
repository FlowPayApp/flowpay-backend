package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/domain"
	"github.com/flowpay/flowpay-backend/internal/model"
	"github.com/flowpay/flowpay-backend/internal/notify"
	"github.com/flowpay/flowpay-backend/internal/remindercontent"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

type ChargeDTO struct {
	repository.Charge
	Status string `json:"status"`
	// NextReminderAt solo en el detalle: canales que siguen en espera y desde cuándo se puede volver a enviar.
	NextReminderAt map[string]time.Time `json:"next_reminder_at,omitempty"`
	// Solo en el detalle: recordatorios automáticos. ReminderPolicy es la que rige (la propia o la de la empresa).
	ReminderMode    string                 `json:"reminder_mode,omitempty"`
	ReminderChannel string                 `json:"reminder_channel,omitempty"`
	ReminderPolicy  *domain.ReminderPolicy `json:"reminder_policy,omitempty"`
	// CompanyReminderPolicy la de la empresa, para volver a ella desde una personalizada.
	CompanyReminderPolicy *domain.ReminderPolicy `json:"company_reminder_policy,omitempty"`
}

// ReminderCooldown espera mínima entre recordatorios manuales de un mismo cobro por el mismo canal.
const ReminderCooldown = time.Hour

var (
	// ErrReminderNoChannel no se eligió ningún canal.
	ErrReminderNoChannel = errors.New("elige al menos un canal: WhatsApp o correo")
	// ErrReminderBadChannel canal desconocido.
	ErrReminderBadChannel = errors.New("canal no válido; usa whatsapp o email")
	// ErrReminderNoEmail la sucursal no tiene correo.
	ErrReminderNoEmail = errors.New("la sucursal no tiene correo registrado")
	// ErrReminderNoPhone la sucursal no tiene teléfono.
	ErrReminderNoPhone = errors.New("la sucursal no tiene teléfono de WhatsApp registrado")
)

// ReminderCooldownError un canal pedido todavía está en espera.
type ReminderCooldownError struct {
	Channel string
	Until   time.Time
}

func (e *ReminderCooldownError) Error() string {
	label := "correo"
	if e.Channel == "whatsapp" {
		label = "WhatsApp"
	}
	return fmt.Sprintf("ya se envió un recordatorio por %s hace poco; podrás enviar otro a las %s", label, e.Until.Format("15:04"))
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
	Repo         *repository.DB
	Notify       *notify.Dispatcher
	UploadDir    string
	AppPublicURL string
	// ReminderSendTime hora diaria (HH:MM) del ciclo de recordatorios automáticos, para mostrarla en el panel.
	ReminderSendTime string
}

func (s *Service) withStatus(ch repository.Charge) ChargeDTO {
	st := domain.ChargeStatus(ch.PaidAt, ch.DueDate, time.Now())
	return ChargeDTO{Charge: ch, Status: st}
}

func (s *Service) ListCharges(ctx context.Context, companyID, memberUID int64) ([]ChargeDTO, error) {
	list, err := s.Repo.ListCharges(ctx, companyID, memberUID)
	if err != nil {
		return nil, err
	}
	out := make([]ChargeDTO, 0, len(list))
	for _, ch := range list {
		out = append(out, s.withStatus(ch))
	}
	return out, nil
}

func (s *Service) GetCharge(ctx context.Context, companyID, id, memberUID int64) (*ChargeDTO, error) {
	ch, err := s.Repo.GetCharge(ctx, companyID, id, memberUID)
	if err != nil {
		return nil, err
	}
	dto := s.withStatus(*ch)
	if dto.Status != "paid" {
		next, err := s.nextReminderAt(ctx, id, time.Now())
		if err != nil {
			log.Printf("[FlowPay] cobro %d sin espera de recordatorios: %v", id, err)
		}
		dto.NextReminderAt = next
	}
	if err := s.withReminders(ctx, &dto); err != nil {
		log.Printf("[FlowPay] cobro %d sin configuración de recordatorios: %v", id, err)
	}
	return &dto, nil
}

// nextReminderAt canales con un recordatorio manual reciente y la hora desde la que se puede repetir.
func (s *Service) nextReminderAt(ctx context.Context, chargeID int64, now time.Time) (map[string]time.Time, error) {
	last, err := s.Repo.LastManualReminderByChannel(ctx, chargeID)
	if err != nil {
		return nil, err
	}
	var out map[string]time.Time
	for channel, at := range last {
		until := at.Add(ReminderCooldown)
		if until.After(now) {
			if out == nil {
				out = map[string]time.Time{}
			}
			out[channel] = until
		}
	}
	return out, nil
}

type CreateChargeInput struct {
	ClientID int64   `json:"client_id"`
	Amount   float64 `json:"amount"`
	DueDate  string  `json:"due_date"`
	ChargeRemindersInput
}

func (s *Service) CreateCharge(ctx context.Context, companyID, memberUID int64, in CreateChargeInput) (int64, error) {
	if in.ClientID == 0 || in.Amount <= 0 || in.DueDate == "" {
		return 0, errors.New("payload de cobro inválido")
	}
	reminders, err := s.mergeChargeReminders(ctx, companyID, repository.ChargeReminderSettings{Mode: domain.ReminderModeCompany}, in.ChargeRemindersInput)
	if err != nil {
		return 0, err
	}
	due, err := time.ParseInLocation("2006-01-02", in.DueDate, time.Local)
	if err != nil {
		return 0, errors.New("due_date debe ser YYYY-MM-DD")
	}
	ok, err := s.Repo.ActiveClientBelongsToCompany(ctx, companyID, in.ClientID, memberUID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errors.New("cliente no válido, inactivo o fuera de tu cartera")
	}
	id, err := s.Repo.CreateCharge(ctx, companyID, in.ClientID, in.Amount, due)
	if err != nil {
		return 0, err
	}
	if in.ChargeRemindersInput.any() {
		if err := s.Repo.UpdateChargeReminderSettings(ctx, companyID, id, reminders); err != nil {
			return 0, err
		}
	}
	if _, err := s.Repo.EnsureClientPaymentToken(ctx, companyID, in.ClientID); err != nil {
		log.Printf("[FlowPay] no se pudo asignar el enlace de pago a la sucursal: %v", err)
	}
	return id, nil
}

func (s *Service) DeleteCharge(ctx context.Context, companyID, chargeID int64) error {
	return s.Repo.DeleteCharge(ctx, companyID, chargeID)
}

func (s *Service) Dashboard(ctx context.Context, companyID, memberUID int64) (*DashboardResponse, error) {
	totals, err := s.Repo.DashboardAggregate(ctx, companyID, memberUID)
	if err != nil {
		return nil, err
	}
	overdue, _ := s.Repo.ChargesOverdueUnpaid(ctx, companyID, memberUID)
	dueSoon, _ := s.Repo.ChargesDueSoon(ctx, companyID, 7, memberUID)
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

// SendReminderNow envía un recordatorio manual por los canales pedidos ("email", "whatsapp").
// Sin canales usa el canal preferido de la sucursal.
func (s *Service) SendReminderNow(ctx context.Context, companyID, chargeID, memberUID int64, channels []string) error {
	ch, err := s.Repo.GetCharge(ctx, companyID, chargeID, memberUID)
	if err != nil {
		return err
	}
	now := time.Now()
	st := domain.ChargeStatus(ch.PaidAt, ch.DueDate, now)
	if st == "paid" {
		return errors.New("cobro ya cerrado")
	}

	var sendEmail, sendWhatsApp bool
	if len(channels) == 0 {
		channel := strings.TrimSpace(strings.ToLower(ch.ClientFollowupChannel))
		if set, err := s.Repo.GetChargeReminderSettings(ctx, chargeID); err == nil && set.Channel != "" {
			channel = set.Channel
		}
		if channel == "" {
			channel = "all"
		}
		if channel == "none" {
			return errors.New("cliente con seguimiento desactivado (none)")
		}
		sendEmail = channel == "all" || channel == "email"
		sendWhatsApp = channel == "all" || channel == "whatsapp"
	} else {
		for _, c := range channels {
			switch strings.TrimSpace(strings.ToLower(c)) {
			case "email":
				sendEmail = true
			case "whatsapp":
				sendWhatsApp = true
			default:
				return ErrReminderBadChannel
			}
		}
	}
	if !sendEmail && !sendWhatsApp {
		return ErrReminderNoChannel
	}
	if sendEmail && (ch.ClientEmail == nil || strings.TrimSpace(*ch.ClientEmail) == "") {
		return ErrReminderNoEmail
	}
	if sendWhatsApp && (ch.ClientPhone == nil || strings.TrimSpace(*ch.ClientPhone) == "") {
		return ErrReminderNoPhone
	}

	waiting, err := s.nextReminderAt(ctx, chargeID, now)
	if err != nil {
		return err
	}
	if until, ok := waiting["whatsapp"]; ok && sendWhatsApp {
		return &ReminderCooldownError{Channel: "whatsapp", Until: until}
	}
	if until, ok := waiting["email"]; ok && sendEmail {
		return &ReminderCooldownError{Channel: "email", Until: until}
	}

	priorOverdue, _ := s.Repo.CountRemindersByKind(ctx, chargeID, "overdue")
	phase, daysU := remindercontent.PhaseFromCharge(*ch, now, priorOverdue)
	subj, textBody, _, payURL, resErr := remindercontent.ResolveReminder(ctx, s.Repo, companyID, phase, daysU, priorOverdue, *ch, s.AppPublicURL)
	if resErr != nil {
		subj, textBody = manualReminderTemplate(*ch, priorOverdue, now)
	}
	whatsAppMessage, _ := notify.BuildWhatsAppTemplate(phase, *ch, payURL, notify.TemplateSIDs{})
	emailMessage := fmt.Sprintf("Asunto: %s\n\n%s", subj, textBody)

	if sendEmail && s.Notify != nil {
		if err := s.Notify.SendReminderEmailFrom(*ch, subj, textBody, s.CompanySMTP(ctx, companyID)); err != nil {
			return err
		}
	}
	if sendEmail {
		if _, err := s.Repo.InsertReminder(ctx, chargeID, "manual", "email", "sent", emailMessage, &now); err != nil {
			return err
		}
	}
	if sendWhatsApp {
		var delivery notify.SentWhatsApp
		if s.Notify != nil {
			from, ferr := s.Repo.FirstActiveWhatsAppToForCompany(ctx, companyID)
			if ferr != nil && !errors.Is(ferr, sql.ErrNoRows) {
				return ferr
			}
			if ferr != nil {
				from = ""
			}
			preview, sent, err := s.Notify.SendCompanyWhatsAppTemplate(*ch, from, phase, payURL)
			if err != nil {
				return err
			}
			if preview != "" {
				whatsAppMessage.Preview = preview
			}
			delivery = sent
		}
		id, err := s.Repo.InsertReminder(ctx, chargeID, "manual", "whatsapp", "sent", whatsAppMessage.Preview, &now)
		if err != nil {
			return err
		}
		if delivery.SID != "" {
			if err := s.Repo.SetReminderDelivery(ctx, id, delivery.SID, delivery.Status); err != nil {
				log.Printf("[FlowPay WhatsApp] recordatorio %d sin seguimiento de entrega: %v", id, err)
			}
		}
	}
	return nil
}

// MessagingSettingsResponse plantillas + textos globales para recordatorios.
type MessagingSettingsResponse struct {
	TransferInstructions string                           `json:"transfer_instructions"`
	PaymentURLTemplate   string                           `json:"payment_url_template"`
	Templates            []repository.ReminderTemplateRow `json:"templates"`
	ReminderPolicy       domain.ReminderPolicy            `json:"reminder_policy"`
	// SendTime hora diaria (HH:MM, hora de Chile por defecto) del ciclo de recordatorios automáticos.
	SendTime string `json:"send_time"`
	// Defaults lo que recibe el cliente en cada fase con un cobro de ejemplo: la plantilla de WhatsApp
	// aprobada (no editable) y el correo del sistema que se usa si la empresa no escribe el suyo.
	Defaults map[string]MessagingPhaseDefaults `json:"defaults"`
}

type MessagingPhaseDefaults struct {
	WhatsApp     string `json:"whatsapp"`
	EmailSubject string `json:"email_subject"`
	EmailBody    string `json:"email_body"`
}

func (s *Service) messagingDefaults() map[string]MessagingPhaseDefaults {
	today := dateOnly(time.Now())
	payURL := strings.TrimRight(strings.TrimSpace(s.AppPublicURL), "/") + "/pay/ejemplo"
	if strings.TrimSpace(s.AppPublicURL) == "" {
		payURL = "https://geldflus.com/pay/ejemplo"
	}
	phases := []struct {
		phase string
		due   time.Time
	}{
		{remindercontent.PhaseApproaching, today.AddDate(0, 0, 3)},
		{remindercontent.PhaseDueToday, today},
		{remindercontent.PhaseOverdueFirst, today.AddDate(0, 0, -2)},
		{remindercontent.PhaseOverdueFollowUp, today.AddDate(0, 0, -8)},
	}
	out := make(map[string]MessagingPhaseDefaults, len(phases))
	for _, p := range phases {
		sample := repository.Charge{Amount: 150000, DueDate: p.due, ClientName: "Sucursal Centro"}
		wa, _ := notify.BuildWhatsAppTemplate(p.phase, sample, payURL, notify.TemplateSIDs{})
		subject, body := remindercontent.DefaultEmail(p.phase, sample, payURL)
		out[p.phase] = MessagingPhaseDefaults{WhatsApp: wa.Preview, EmailSubject: subject, EmailBody: body}
	}
	return out
}

// MessagingTemplateInput fila de plantilla desde el panel.
type MessagingTemplateInput struct {
	Phase        string `json:"phase"`
	DayMin       int    `json:"day_min"`
	DayMax       int    `json:"day_max"`
	SortOrder    int    `json:"sort_order"`
	EmailSubject string `json:"email_subject"`
	Body         string `json:"body"`
	WhatsAppBody string `json:"whatsapp_body"`
}

// SaveMessagingInput PUT /api/company/messaging
type SaveMessagingInput struct {
	TransferInstructions string                   `json:"transfer_instructions"`
	PaymentURLTemplate   string                   `json:"payment_url_template"`
	Templates            []MessagingTemplateInput `json:"templates"`
	// ReminderPolicy nula deja la frecuencia como está.
	ReminderPolicy *domain.ReminderPolicy `json:"reminder_policy"`
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
	policy, err := s.Repo.GetCompanyReminderPolicy(ctx, companyID)
	if err != nil {
		return nil, err
	}
	return &MessagingSettingsResponse{
		TransferInstructions: cm.TransferInstructions,
		PaymentURLTemplate:   cm.PaymentURLTemplate,
		Templates:            tpl,
		ReminderPolicy:       policy,
		SendTime:             s.ReminderSendTime,
		Defaults:             s.messagingDefaults(),
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
	var policy domain.ReminderPolicy
	if in.ReminderPolicy != nil {
		p, err := in.ReminderPolicy.Normalized()
		if err != nil {
			return err
		}
		policy = p
	}
	if err := s.Repo.UpdateCompanyMessaging(ctx, companyID, in.TransferInstructions, in.PaymentURLTemplate); err != nil {
		return err
	}
	if in.ReminderPolicy != nil {
		if err := s.Repo.UpdateCompanyReminderPolicy(ctx, companyID, policy); err != nil {
			return err
		}
	}
	rows := make([]repository.ReminderTemplateRow, 0, len(in.Templates))
	for _, t := range in.Templates {
		if strings.TrimSpace(t.Body) == "" && strings.TrimSpace(t.WhatsAppBody) == "" {
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
			WhatsAppBody: t.WhatsAppBody,
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

func (s *Service) ListReminders(ctx context.Context, companyID, chargeID, memberUID int64) ([]repository.Reminder, error) {
	if _, err := s.Repo.GetCharge(ctx, companyID, chargeID, memberUID); err != nil {
		return nil, err
	}
	return s.Repo.ListReminders(ctx, chargeID)
}

var (
	// ErrWhatsAppReplyWindow el cliente no escribió en las últimas 24 horas.
	ErrWhatsAppReplyWindow = errors.New("pasaron más de 24 horas desde el último mensaje del cliente. Usa Enviar recordatorio ahora")
	// ErrWhatsAppReplyPhone el cobro no tiene teléfono.
	ErrWhatsAppReplyPhone = errors.New("este cobro no tiene teléfono de WhatsApp")
	// ErrWhatsAppReplyEmpty el texto viene vacío.
	ErrWhatsAppReplyEmpty = errors.New("escribe un mensaje")
	// ErrWhatsAppReplyLong el texto supera el límite.
	ErrWhatsAppReplyLong = errors.New("el mensaje puede tener hasta 1000 caracteres")
)

// prepareWhatsAppReply valida que se pueda escribirle al cliente del cobro y resuelve el número que envía.
// WhatsApp solo permite mensajes libres durante las 24 horas siguientes al último mensaje del cliente.
func (s *Service) prepareWhatsAppReply(ctx context.Context, companyID, chargeID, memberUID int64) (*repository.Charge, string, error) {
	ch, err := s.Repo.GetCharge(ctx, companyID, chargeID, memberUID)
	if err != nil {
		return nil, "", err
	}
	if ch.ClientPhone == nil || strings.TrimSpace(*ch.ClientPhone) == "" {
		return nil, "", ErrWhatsAppReplyPhone
	}
	if s.Notify == nil {
		return nil, "", errors.New("WhatsApp no está configurado")
	}
	since := time.Now().Add(-24 * time.Hour)
	open, err := s.Repo.HasInboundFromPhoneSince(ctx, companyID, *ch.ClientPhone, since)
	if err != nil {
		return nil, "", err
	}
	if !open {
		return nil, "", ErrWhatsAppReplyWindow
	}
	from := ""
	if tn, ferr := s.Repo.FirstActiveWhatsAppToForCompany(ctx, companyID); ferr == nil {
		from = strings.TrimSpace(tn)
	} else if !errors.Is(ferr, sql.ErrNoRows) {
		return nil, "", ferr
	}
	return ch, from, nil
}

// ReplyChargeWhatsApp envía un texto libre al cliente si escribió en las últimas 24 horas.
func (s *Service) ReplyChargeWhatsApp(ctx context.Context, companyID, chargeID, memberUID int64, text string) (*model.Message, error) {
	msg := strings.TrimSpace(text)
	if msg == "" {
		return nil, ErrWhatsAppReplyEmpty
	}
	if len([]rune(msg)) > 1000 {
		return nil, ErrWhatsAppReplyLong
	}
	ch, from, err := s.prepareWhatsAppReply(ctx, companyID, chargeID, memberUID)
	if err != nil {
		return nil, err
	}
	sent, err := s.Notify.SendCompanyWhatsAppText(*ch, msg, from)
	if err != nil {
		return nil, err
	}
	return s.saveOutboundMessage(ctx, companyID, chargeID, from, *ch.ClientPhone, msg, nil, sent)
}

func (s *Service) saveOutboundMessage(ctx context.Context, companyID, chargeID int64, from, clientPhone, msg string, media []model.MessageMedia, sent notify.SentWhatsApp) (*model.Message, error) {
	to := notify.NormalizeWhatsAppForTwilio(clientPhone)
	cid := chargeID
	status := sent.Status
	if status == "" {
		status = "sent"
	}
	saved := &model.Message{
		CompanyID:   companyID,
		ChargeID:    &cid,
		FromNumber:  from,
		ToNumber:    to,
		Content:     msg,
		Media:       media,
		Direction:   "outbound",
		Status:      status,
		ProviderSID: sent.SID,
	}
	id, err := s.Repo.InsertMessage(ctx, saved)
	if err != nil {
		log.Printf("[FlowPay WhatsApp] respuesta enviada pero no se guardó charge=%d: %v", chargeID, err)
		return saved, nil
	}
	got, err := s.Repo.GetMessageByID(ctx, companyID, id)
	if err != nil {
		return saved, nil
	}
	return got, nil
}

// ListChargeInboundWhatsApp respuestas del cliente (WhatsApp entrante) vinculadas al cobro.
func (s *Service) ListChargeInboundWhatsApp(ctx context.Context, companyID, chargeID, memberUID int64) ([]model.Message, error) {
	if _, err := s.Repo.GetCharge(ctx, companyID, chargeID, memberUID); err != nil {
		return nil, err
	}
	return s.Repo.ListInboundMessagesForCharge(ctx, companyID, chargeID)
}

// SimulateChargeInboundWhatsApp inserta un mensaje entrante de prueba vinculado al cobro (demo / QA).
func (s *Service) SimulateChargeInboundWhatsApp(ctx context.Context, companyID, chargeID, memberUID int64, text string) (*model.Message, error) {
	ch, err := s.Repo.GetCharge(ctx, companyID, chargeID, memberUID)
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
	m := &model.Message{
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
	ChargeRemindersInput
}

func (s *Service) PatchCharge(ctx context.Context, companyID, chargeID, memberUID int64, in PatchChargeInput) error {
	has := in.ClientID != nil || in.DueDate != nil || in.Amount != nil || in.SetPaid != nil || in.ChargeRemindersInput.any()
	if !has {
		return errors.New("nada que actualizar")
	}
	if _, err := s.Repo.GetCharge(ctx, companyID, chargeID, memberUID); err != nil {
		return err
	}
	if in.ClientID != nil {
		ok, err := s.Repo.ActiveClientBelongsToCompany(ctx, companyID, *in.ClientID, memberUID)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("cliente no válido, inactivo o fuera de tu cartera")
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

	var reminders *repository.ChargeReminderSettings
	if in.ChargeRemindersInput.any() {
		cur, err := s.Repo.GetChargeReminderSettings(ctx, chargeID)
		if err != nil {
			return err
		}
		next, err := s.mergeChargeReminders(ctx, companyID, cur, in.ChargeRemindersInput)
		if err != nil {
			return err
		}
		reminders = &next
	}

	if err := s.Repo.UpdateChargeFields(ctx, companyID, chargeID, in.ClientID, duePtr, in.Amount); err != nil {
		return err
	}
	if reminders != nil {
		if err := s.Repo.UpdateChargeReminderSettings(ctx, companyID, chargeID, *reminders); err != nil {
			return err
		}
	}

	if in.SetPaid == nil {
		return nil
	}
	if !*in.SetPaid {
		return s.Repo.ClearChargePayment(ctx, companyID, chargeID)
	}
	ch, err := s.Repo.GetCharge(ctx, companyID, chargeID, memberUID)
	if err != nil {
		return err
	}
	if ch.PaidAt != nil {
		return nil
	}
	return s.Repo.MarkChargePaid(ctx, chargeID, ch.Amount)
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
