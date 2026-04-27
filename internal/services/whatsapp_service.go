package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/models"
	"github.com/flowpay/flowpay-backend/internal/notify"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

var (
	// ErrUnknownWhatsAppTo indica que el número "To" no está asociado a ninguna empresa activa.
	ErrUnknownWhatsAppTo = errors.New("whatsapp to no registrado")
)

// WhatsAppService envía y registra mensajes WhatsApp por empresa (Twilio).
type WhatsAppService struct {
	Repo *repository.Repository
	// Credenciales de la cuenta Twilio (compartidas entre tenants).
	AccountSID string
	AuthToken  string
}

func (s *WhatsAppService) twilioEnabled() bool {
	return strings.TrimSpace(s.AccountSID) != "" && strings.TrimSpace(s.AuthToken) != ""
}

// SendMessage envía desde el número activo de la empresa y persiste direction=outbound.
func (s *WhatsAppService) SendMessage(ctx context.Context, companyID int64, to string, message string) error {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return errors.New("mensaje vacío")
	}
	wn, err := s.Repo.GetActiveWhatsAppNumber(ctx, companyID)
	if err != nil {
		return err
	}
	toAddr := notify.NormalizeWhatsAppForTwilio(to)
	if toAddr == "" {
		return errors.New("teléfono destino inválido")
	}
	fromAddr := strings.TrimSpace(wn.PhoneNumber)
	if fromAddr == "" {
		return errors.New("número origen de empresa inválido")
	}

	if !s.twilioEnabled() {
		log.Printf("[FlowPay WhatsApp] mock outbound company=%d from=%s to=%s", companyID, fromAddr, toAddr)
		_, err := s.Repo.InsertMessage(ctx, &models.Message{
			CompanyID:  companyID,
			FromNumber: fromAddr,
			ToNumber:   toAddr,
			Content:    msg,
			Direction:  "outbound",
			Status:     "sent",
		})
		return err
	}

	statusCode, respBody, sendErr := twilioPostMessage(s.AccountSID, s.AuthToken, fromAddr, toAddr, msg)
	if sendErr != nil {
		log.Printf("[FlowPay WhatsApp] error envío company=%d: %v", companyID, sendErr)
		_, insErr := s.Repo.InsertMessage(ctx, &models.Message{
			CompanyID:  companyID,
			FromNumber: fromAddr,
			ToNumber:   toAddr,
			Content:    msg,
			Direction:  "outbound",
			Status:     "failed",
		})
		if insErr != nil {
			log.Printf("[FlowPay WhatsApp] insert failed tras error envío: %v", insErr)
		}
		return sendErr
	}
	if statusCode >= 300 {
		log.Printf("[FlowPay WhatsApp] HTTP %d company=%d body=%s", statusCode, companyID, string(respBody))
		_, insErr := s.Repo.InsertMessage(ctx, &models.Message{
			CompanyID:  companyID,
			FromNumber: fromAddr,
			ToNumber:   toAddr,
			Content:    msg,
			Direction:  "outbound",
			Status:     "failed",
		})
		if insErr != nil {
			log.Printf("[FlowPay WhatsApp] insert failed tras HTTP error: %v", insErr)
		}
		return fmt.Errorf("twilio HTTP %d", statusCode)
	}
	log.Printf("[FlowPay WhatsApp] enviado company=%d to=%s", companyID, toAddr)
	_, err = s.Repo.InsertMessage(ctx, &models.Message{
		CompanyID:  companyID,
		FromNumber: fromAddr,
		ToNumber:   toAddr,
		Content:    msg,
		Direction:  "outbound",
		Status:     "sent",
	})
	return err
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
	log.Printf("[FlowPay WhatsApp] inbound company=%d from=%s to=%s len=%d", wn.CompanyID, fromNorm, toNorm, len(content))
	_, err = s.Repo.InsertMessage(ctx, &models.Message{
		CompanyID:  wn.CompanyID,
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

// ListMessages lista mensajes del tenant; opcionalmente filtra por teléfono de conversación.
func (s *WhatsAppService) ListMessages(ctx context.Context, companyID int64, limit, offset int, phone string) ([]models.Message, error) {
	phoneNorm := ""
	if strings.TrimSpace(phone) != "" {
		phoneNorm = notify.NormalizeWhatsAppForTwilio(phone)
		if phoneNorm == "" {
			phoneNorm = strings.TrimSpace(phone)
		}
	}
	return s.Repo.ListMessagesByCompany(ctx, companyID, limit, offset, phoneNorm)
}

func twilioPostMessage(accountSID, authToken, from, to, body string) (statusCode int, respBody []byte, err error) {
	api := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", accountSID)
	data := url.Values{}
	data.Set("From", from)
	data.Set("To", to)
	data.Set("Body", body)
	req, err := http.NewRequest(http.MethodPost, api, strings.NewReader(data.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.SetBasicAuth(accountSID, authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, nil
}
