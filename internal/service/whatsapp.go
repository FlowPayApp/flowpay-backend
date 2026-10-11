package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/model"
	"github.com/flowpay/flowpay-backend/internal/notify"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

var (
	// ErrUnknownWhatsAppTo indica que el número "To" no está asociado a ninguna empresa activa.
	ErrUnknownWhatsAppTo = errors.New("whatsapp to no registrado")
	// ErrInvalidWhatsAppNumber el texto no es un teléfono usable.
	ErrInvalidWhatsAppNumber = errors.New("número de WhatsApp inválido; usa código de país, por ejemplo +56912345678")
	// ErrWhatsAppNumberInUse ese Business ya está asignado a otra empresa.
	ErrWhatsAppNumberInUse = errors.New("ese número ya está asignado a otra empresa")
	// ErrCompanyNotFound no hay empresa con ese id.
	ErrCompanyNotFound = errors.New("empresa no encontrada")
)

// ErrMediaNotFound el adjunto no existe o no pertenece al cobro.
var ErrMediaNotFound = errors.New("adjunto no encontrado")

// Solo a Twilio se le envían las credenciales de la cuenta: la URL del adjunto llega por el webhook.
const twilioMediaPrefix = "https://api.twilio.com/"

var mediaClient = &http.Client{Timeout: 30 * time.Second}

// WhatsAppService procesa webhooks entrantes de Twilio (guardar en messages) y entrega sus adjuntos.
type WhatsAppService struct {
	Repo       *repository.DB
	AccountSID string
	AuthToken  string
}

// Estados de entrega de Twilio que se guardan; los intermedios (sending, scheduled...) no cambian lo que ve el panel.
var deliveryStatuses = map[string]bool{
	"queued":      true,
	"accepted":    true,
	"sent":        true,
	"delivered":   true,
	"read":        true,
	"failed":      true,
	"undelivered": true,
}

// HandleDeliveryStatus aplica el aviso de Twilio al WhatsApp enviado con ese sid.
func (s *WhatsAppService) HandleDeliveryStatus(ctx context.Context, sid, status, errorCode string) error {
	sid = strings.TrimSpace(sid)
	status = strings.ToLower(strings.TrimSpace(status))
	if sid == "" || !deliveryStatuses[status] {
		return nil
	}
	errorCode = strings.TrimSpace(errorCode)
	if len(errorCode) > 20 {
		errorCode = errorCode[:20]
	}
	_, err := s.Repo.UpdateWhatsAppDelivery(ctx, sid, status, errorCode)
	return err
}

// HandleInbound guarda mensaje entrante enrutado por número receptor (To).
func (s *WhatsAppService) HandleInbound(ctx context.Context, fromRaw, toRaw, body string, media []model.MessageMedia) error {
	toNorm := canonicalWhatsApp(toRaw)
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
	fromNorm := canonicalWhatsApp(fromRaw)
	if fromNorm == "" {
		fromNorm = strings.TrimSpace(fromRaw)
	}
	content := strings.TrimSpace(body)
	var chargeID *int64
	if cid, err := s.Repo.FindOpenChargeIDForInboundWhatsApp(ctx, wn.CompanyID, fromNorm); err == nil && cid != nil {
		chargeID = cid
	} else if err != nil {
		log.Printf("[FlowPay WhatsApp] warn asociando cobro inbound: %v", err)
	}
	log.Printf("[FlowPay WhatsApp] inbound company=%d from=%s to=%s charge_id=%v len=%d media=%d", wn.CompanyID, fromNorm, toNorm, chargeID, len(content), len(media))
	_, err = s.Repo.InsertMessage(ctx, &model.Message{
		CompanyID:  wn.CompanyID,
		ChargeID:   chargeID,
		FromNumber: fromNorm,
		ToNumber:   toNorm,
		Content:    content,
		Media:      media,
		Direction:  "inbound",
		Status:     "received",
	})
	if err != nil {
		log.Printf("[FlowPay WhatsApp] error guardando inbound: %v", err)
	}
	return err
}

// OpenChargeMedia abre un adjunto de un mensaje del cobro: los recibidos se descargan de Twilio,
// los enviados por la empresa salen de la base de datos.
// El llamador debe cerrar el cuerpo devuelto.
func (s *WhatsAppService) OpenChargeMedia(ctx context.Context, companyID, chargeID, memberUID, msgID int64, index int) (io.ReadCloser, string, int64, error) {
	if _, err := s.Repo.GetCharge(ctx, companyID, chargeID, memberUID); err != nil {
		return nil, "", 0, err
	}
	m, err := s.Repo.GetMessageByID(ctx, companyID, msgID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", 0, ErrMediaNotFound
		}
		return nil, "", 0, err
	}
	if m.ChargeID == nil || *m.ChargeID != chargeID || index < 0 || index >= len(m.Media) {
		return nil, "", 0, ErrMediaNotFound
	}
	media := m.Media[index]
	if media.FileToken != "" {
		f, err := s.Repo.GetMessageFile(ctx, companyID, media.FileToken)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, "", 0, ErrMediaNotFound
			}
			return nil, "", 0, err
		}
		return io.NopCloser(bytes.NewReader(f.Data)), f.ContentType, int64(len(f.Data)), nil
	}
	if !strings.HasPrefix(media.URL, twilioMediaPrefix) {
		return nil, "", 0, ErrMediaNotFound
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, media.URL, nil)
	if err != nil {
		return nil, "", 0, err
	}
	req.SetBasicAuth(s.AccountSID, s.AuthToken)
	resp, err := mediaClient.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", 0, fmt.Errorf("twilio media: %s", resp.Status)
	}
	contentType := media.ContentType
	if contentType == "" {
		contentType = resp.Header.Get("Content-Type")
	}
	return resp.Body, contentType, resp.ContentLength, nil
}

// ListActiveNumbers números Business asignados (solo activos).
func (s *WhatsAppService) ListActiveNumbers(ctx context.Context) ([]model.WhatsAppNumber, error) {
	return s.Repo.ListActiveWhatsAppNumbers(ctx)
}

// AssignCompanyNumber fija el WhatsApp Business que envía y recibe para la empresa.
// rawPhone vacío quita la asignación.
func (s *WhatsAppService) AssignCompanyNumber(ctx context.Context, companyID int64, rawPhone string) (*model.WhatsAppNumber, error) {
	rawPhone = strings.TrimSpace(rawPhone)
	phone := ""
	if rawPhone != "" {
		phone = canonicalWhatsApp(rawPhone)
		digits := strings.TrimPrefix(phone, "whatsapp:+")
		if phone == "" || len(digits) < 8 || len(digits) > 15 {
			return nil, ErrInvalidWhatsAppNumber
		}
	}
	w, err := s.Repo.SetCompanyWhatsAppNumber(ctx, companyID, phone)
	if err != nil {
		if errors.Is(err, repository.ErrCompanyMissing) {
			return nil, ErrCompanyNotFound
		}
		if errors.Is(err, repository.ErrWhatsAppNumberTaken) {
			return nil, ErrWhatsAppNumberInUse
		}
		return nil, err
	}
	return w, nil
}

func canonicalWhatsApp(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(strings.ToLower(s), "whatsapp:")
	return notify.NormalizeWhatsAppForTwilio(strings.TrimSpace(s))
}
