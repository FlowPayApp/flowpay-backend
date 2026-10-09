package service

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/model"
)

// WhatsApp acepta imágenes de hasta 5 MB; se usa el mismo tope para los PDF.
const MaxChatFileBytes = 5 << 20

// Twilio descarga el archivo apenas se envía; después el enlace público deja de servir.
const chatFilePublicTTL = 24 * time.Hour

const inboxLimit = 20

var (
	// ErrChatFileType el archivo no es una foto JPG/PNG ni un PDF.
	ErrChatFileType = errors.New("solo puedes enviar fotos JPG o PNG, o un PDF")
	// ErrChatFileSize el archivo supera el tope de WhatsApp.
	ErrChatFileSize = errors.New("el archivo puede pesar hasta 5 MB")
	// ErrChatFilePublicURL falta la dirección pública desde la que Twilio descarga el archivo.
	ErrChatFilePublicURL = errors.New("falta configurar FLOWPAY_PUBLIC_BASE_URL para enviar archivos")
)

// El tipo se decide por el contenido real del archivo, no por su nombre.
var chatFileTypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"application/pdf": ".pdf",
}

var unsafeFileNameChars = regexp.MustCompile(`[^\p{L}\p{N}._-]+`)

// chatFileName deja un nombre legible para el cliente, con la extensión que corresponde al tipo real.
func chatFileName(original, ext string) string {
	base := strings.TrimSuffix(filepath.Base(strings.TrimSpace(original)), filepath.Ext(original))
	base = strings.Trim(unsafeFileNameChars.ReplaceAllString(base, "-"), "-.")
	if r := []rune(base); len(r) > 60 {
		base = string(r[:60])
	}
	if base == "" {
		base = "archivo"
	}
	return base + ext
}

// Inbox respuestas sin leer de los cobros que el usuario puede ver.
func (s *Service) Inbox(ctx context.Context, companyID, memberUID int64) (*model.Inbox, error) {
	return s.Repo.ListUnreadInbound(ctx, companyID, memberUID, inboxLimit)
}

// MarkChargeRead marca como leídas las respuestas del cobro.
func (s *Service) MarkChargeRead(ctx context.Context, companyID, chargeID, memberUID int64) error {
	if _, err := s.Repo.GetCharge(ctx, companyID, chargeID, memberUID); err != nil {
		return err
	}
	_, err := s.Repo.MarkChargeInboundRead(ctx, companyID, chargeID)
	return err
}

// SendChargeWhatsAppFile envía una foto o PDF al cliente, con un texto opcional como pie.
func (s *Service) SendChargeWhatsAppFile(ctx context.Context, companyID, chargeID, memberUID int64, caption, originalName string, data []byte) (*model.Message, error) {
	caption = strings.TrimSpace(caption)
	if len([]rune(caption)) > 1000 {
		return nil, ErrWhatsAppReplyLong
	}
	if len(data) == 0 {
		return nil, ErrChatFileType
	}
	if len(data) > MaxChatFileBytes {
		return nil, ErrChatFileSize
	}
	contentType := strings.Split(http.DetectContentType(data), ";")[0]
	ext, ok := chatFileTypes[contentType]
	if !ok {
		return nil, ErrChatFileType
	}
	ch, from, err := s.prepareWhatsAppReply(ctx, companyID, chargeID, memberUID)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(s.Notify.PublicBaseURL(), "/")
	if base == "" {
		base = strings.TrimRight(s.AppPublicURL, "/")
	}
	if base == "" {
		return nil, ErrChatFilePublicURL
	}

	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	name := chatFileName(originalName, ext)
	file := model.MessageFile{ContentType: contentType, FileName: name, Data: data}
	if err := s.Repo.InsertMessageFile(ctx, companyID, token, file); err != nil {
		return nil, err
	}
	// El nombre al final de la URL es el que WhatsApp le muestra al cliente en los PDF.
	mediaURL := base + "/api/public/chat-files/" + token + "/" + url.PathEscape(name)
	sent, err := s.Notify.SendCompanyWhatsAppFile(*ch, caption, from, mediaURL)
	if err != nil {
		if derr := s.Repo.DeleteMessageFile(ctx, companyID, token); derr != nil {
			log.Printf("[FlowPay WhatsApp] archivo no enviado y no se pudo borrar token=%s: %v", token, derr)
		}
		return nil, err
	}
	media := []model.MessageMedia{{ContentType: contentType, FileName: name, FileToken: token}}
	return s.saveOutboundMessage(ctx, companyID, chargeID, from, *ch.ClientPhone, caption, media, sent)
}

// OpenPublicChatFile entrega a Twilio el archivo recién enviado.
func (s *Service) OpenPublicChatFile(ctx context.Context, token string) (*model.MessageFile, error) {
	if len(token) != 32 {
		return nil, ErrMediaNotFound
	}
	return s.Repo.GetRecentMessageFile(ctx, token, chatFilePublicTTL)
}
