package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/notify"
	"github.com/flowpay/flowpay-backend/internal/repository"
	"github.com/flowpay/flowpay-backend/internal/secretbox"
)

const credentialConfirmTTL = 30 * time.Minute

var (
	ErrCredentialDelivery = errors.New("no se pudo enviar la confirmación")
	ErrCredentialLink     = repository.ErrCredentialConfirmMissing
	ErrCredentialExpired  = repository.ErrCredentialConfirmExpired
	ErrCredentialUsed     = repository.ErrCredentialConfirmUsed
)

// CredentialChangeReceipt avisa que el cambio quedó esperando el correo.
type CredentialChangeReceipt struct {
	Pending  bool   `json:"pending"`
	Action   string `json:"action"`
	Provider string `json:"provider"`
	SentTo   string `json:"sent_to"`
}

// PendingCredentialChange es un cambio que todavía no se confirma.
type PendingCredentialChange struct {
	Provider  string    `json:"provider"`
	Action    string    `json:"action"`
	SentTo    string    `json:"sent_to"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CredentialChangePreview es la página del enlace. No incluye secretos.
type CredentialChangePreview struct {
	Company       string    `json:"company"`
	Provider      string    `json:"provider"`
	ProviderLabel string    `json:"provider_label"`
	Action        string    `json:"action"`
	ActionLabel   string    `json:"action_label"`
	ActionVerb    string    `json:"action_verb"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (s *Service) PaymentCredentialStatus(ctx context.Context, companyID int64) (PaymentCredentialStatus, error) {
	flags, err := s.Repo.PaymentCredentialFlags(ctx, companyID)
	if err != nil {
		return PaymentCredentialStatus{}, err
	}
	out := PaymentCredentialStatus{
		Khipu:           flags.Khipu,
		Webpay:          flags.Webpay,
		Oneclick:        flags.Oneclick,
		Fintoc:          flags.Fintoc,
		TransferGateway: flags.TransferGateway,
	}
	pending, err := s.Repo.ListPendingCredentialConfirmations(ctx, companyID, time.Now())
	if err != nil {
		return PaymentCredentialStatus{}, err
	}
	for _, row := range pending {
		out.Pending = append(out.Pending, PendingCredentialChange{
			Provider:  row.Provider,
			Action:    row.Action,
			SentTo:    maskEmail(row.AdminEmail),
			ExpiresAt: row.ExpiresAt,
		})
	}
	return out, nil
}

// RequestPaymentCredentialChange no aplica el cambio. Lo deja cifrado hasta que el administrador abra el correo.
func (s *Service) RequestPaymentCredentialChange(ctx context.Context, companyID, userID int64, in SavePaymentCredentialsInput) (CredentialChangeReceipt, error) {
	writes, err := planCredentialWrites(in)
	if err != nil {
		return CredentialChangeReceipt{}, err
	}
	gateway, setGateway, err := planTransferGateway(in.TransferGateway)
	if err != nil {
		return CredentialChangeReceipt{}, err
	}
	if len(writes) == 0 && !setGateway {
		return CredentialChangeReceipt{}, &PaymentCredentialError{msg: "no hay credenciales para guardar"}
	}
	if len(writes) == 0 && setGateway {
		return CredentialChangeReceipt{}, s.Repo.SetTransferGateway(ctx, companyID, gateway)
	}
	flags, err := s.Repo.PaymentCredentialFlags(ctx, companyID)
	if err != nil {
		return CredentialChangeReceipt{}, err
	}
	provider, action, err := classifyCredentialChange(writes, flags)
	if err != nil {
		return CredentialChangeReceipt{}, err
	}
	if len(s.PaymentKey) != 32 {
		return CredentialChangeReceipt{}, ErrPaymentCredentialKey
	}
	base := strings.TrimRight(strings.TrimSpace(s.AppPublicURL), "/")
	if base == "" {
		log.Printf("confirmación de credenciales: falta FLOWPAY_APP_PUBLIC_URL")
		return CredentialChangeReceipt{}, ErrCredentialDelivery
	}
	admin, err := s.Repo.CompanyRegisteredAdmin(ctx, companyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CredentialChangeReceipt{}, &PaymentCredentialError{msg: "la empresa no tiene un correo de administrador para confirmar el cambio"}
		}
		return CredentialChangeReceipt{}, err
	}
	admin.Email = strings.TrimSpace(strings.ToLower(admin.Email))
	if !strings.Contains(admin.Email, "@") {
		return CredentialChangeReceipt{}, &PaymentCredentialError{msg: "la empresa no tiene un correo de administrador para confirmar el cambio"}
	}

	fields := make(map[string]*string, len(writes))
	for _, write := range writes {
		if write.plain == "" {
			fields[write.column] = nil
			continue
		}
		sealed, err := secretbox.Seal(s.PaymentKey, []byte(write.plain), credentialAAD(companyID, write.column))
		if err != nil {
			return CredentialChangeReceipt{}, ErrPaymentCredentialKey
		}
		fields[write.column] = &sealed
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return CredentialChangeReceipt{}, err
	}
	token, hash, err := newCredentialToken()
	if err != nil {
		return CredentialChangeReceipt{}, err
	}
	expires := time.Now().Add(credentialConfirmTTL)
	if err := s.Repo.ReplaceCredentialConfirmation(ctx, companyID, hash, provider, action, admin.Email, payload, userID, expires); err != nil {
		return CredentialChangeReceipt{}, err
	}
	link := base + "/confirmar-credencial/" + token
	if err := s.sendCredentialConfirmation(admin, provider, action, link); err != nil {
		_ = s.Repo.DeleteCredentialConfirmation(ctx, hash)
		log.Printf("confirmación de credenciales: %v", err)
		return CredentialChangeReceipt{}, ErrCredentialDelivery
	}
	return CredentialChangeReceipt{
		Pending:  true,
		Action:   action,
		Provider: provider,
		SentTo:   maskEmail(admin.Email),
	}, nil
}

func (s *Service) PreviewCredentialConfirmation(ctx context.Context, token string) (CredentialChangePreview, error) {
	row, err := s.Repo.CredentialConfirmation(ctx, credentialTokenHash(token))
	if err != nil {
		return CredentialChangePreview{}, err
	}
	if row.Used {
		return CredentialChangePreview{}, ErrCredentialUsed
	}
	if !row.ExpiresAt.After(time.Now()) {
		return CredentialChangePreview{}, ErrCredentialExpired
	}
	return credentialPreview(row.CompanyName, row.Provider, row.Action, row.ExpiresAt), nil
}

func (s *Service) ConfirmCredentialChange(ctx context.Context, token string) (CredentialChangePreview, error) {
	provider, action, err := s.Repo.ConsumeCredentialConfirmation(ctx, credentialTokenHash(token), time.Now())
	if err != nil {
		return CredentialChangePreview{}, err
	}
	return credentialPreview("", provider, action, time.Time{}), nil
}

func (s *Service) sendCredentialConfirmation(admin repository.CompanyRegisteredAdmin, provider, action, link string) error {
	if s.Notify == nil {
		return fmt.Errorf("smtp sin configurar")
	}
	who := strings.TrimSpace(admin.Name)
	greeting := "Estimado,"
	if who != "" && !strings.EqualFold(who, admin.Email) {
		greeting = "Estimado " + who + ","
	}
	verb := credentialActionVerb(action)
	label := credentialProviderLabel(provider)
	company := strings.TrimSpace(admin.CompanyName)
	if company == "" {
		company = "su empresa"
	}
	minutes := int(credentialConfirmTTL.Minutes())
	return s.Notify.SendCredentialConfirmation(admin.Email, "Confirme un cambio de credenciales en GeldFlus", notify.CredentialNotice{
		Greeting: greeting,
		Lead: fmt.Sprintf(
			"Hay una solicitud para %s las credenciales de %s en %s. El enlace de pago no cambia hasta que usted confirme.",
			verb, label, company,
		),
		Notes: []string{
			fmt.Sprintf("Este enlace caduca en %d minutos y se puede usar una sola vez.", minutes),
			"Si usted no pidió este cambio, ignore el mensaje. Las credenciales actuales siguen vigentes.",
		},
		Rows: [][2]string{
			{"Empresa", company},
			{"Medio", label},
			{"Acción", credentialActionLabel(action)},
		},
		ActionURL: link,
	})
}

func credentialPreview(company, provider, action string, expires time.Time) CredentialChangePreview {
	return CredentialChangePreview{
		Company:       company,
		Provider:      provider,
		ProviderLabel: credentialProviderLabel(provider),
		Action:        action,
		ActionLabel:   credentialActionLabel(action),
		ActionVerb:    credentialActionVerb(action),
		ExpiresAt:     expires,
	}
}

func classifyCredentialChange(writes []credentialWrite, flags repository.PaymentCredentialFlags) (string, string, error) {
	provider := ""
	remove := true
	for _, write := range writes {
		next := credentialColumnProvider(write.column)
		if next == "" {
			return "", "", &PaymentCredentialError{msg: "no hay credenciales para guardar"}
		}
		if provider == "" {
			provider = next
		} else if provider != next {
			return "", "", &PaymentCredentialError{msg: "cambia un medio de pago a la vez"}
		}
		if write.plain != "" {
			remove = false
		}
	}
	if provider == "" {
		return "", "", &PaymentCredentialError{msg: "no hay credenciales para guardar"}
	}
	active := credentialProviderActive(provider, flags)
	if remove {
		if !active {
			return "", "", &PaymentCredentialError{msg: "no hay credenciales de " + credentialProviderLabel(provider) + " para quitar"}
		}
		return provider, "remove", nil
	}
	if active {
		return provider, "edit", nil
	}
	return provider, "create", nil
}

func credentialColumnProvider(column string) string {
	switch column {
	case "khipu_api_key":
		return "khipu"
	case "webpay_commerce_code", "webpay_api_key":
		return "webpay"
	case "oneclick_mall_code", "oneclick_store_code", "oneclick_api_key":
		return "oneclick"
	case "fintoc_secret_key", "fintoc_webhook_secret":
		return "fintoc"
	default:
		return ""
	}
}

func credentialProviderActive(provider string, flags repository.PaymentCredentialFlags) bool {
	switch provider {
	case "khipu":
		return flags.Khipu
	case "webpay":
		return flags.Webpay
	case "oneclick":
		return flags.Oneclick
	case "fintoc":
		return flags.Fintoc
	default:
		return false
	}
}

func credentialProviderLabel(provider string) string {
	switch provider {
	case "khipu":
		return "Khipu"
	case "webpay":
		return "Webpay"
	case "oneclick":
		return "Oneclick"
	case "fintoc":
		return "Fintoc"
	default:
		return provider
	}
}

func credentialActionLabel(action string) string {
	switch action {
	case "create":
		return "Cargar"
	case "edit":
		return "Reemplazar"
	case "remove":
		return "Quitar"
	default:
		return action
	}
}

func credentialActionVerb(action string) string {
	switch action {
	case "create":
		return "cargar"
	case "edit":
		return "reemplazar"
	case "remove":
		return "quitar"
	default:
		return "cambiar"
	}
}

func maskEmail(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	at := strings.Index(email, "@")
	if at <= 0 || at == len(email)-1 {
		return ""
	}
	local := email[:at]
	domain := email[at:]
	if len(local) <= 1 {
		return local + "***" + domain
	}
	return local[:1] + "***" + domain
}

func newCredentialToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	return token, credentialTokenHash(token), nil
}

func credentialTokenHash(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}
