package config

import (
	"log"
	"os"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/flowpay/flowpay-backend/internal/notify"
)

type Config struct {
	DSN                   string
	Addr                  string
	ReminderHour          int
	ReminderMinute        int
	ReminderLocation      *time.Location
	DefaultCompanyID      int64
	JWTSecret             string
	UploadDir             string
	PublicBaseURL         string
	AppPublicURL          string
	Notify                *notify.Dispatcher
	TwilioAccountSID      string
	TwilioAuthToken       string
	TwilioValidateWebhook bool
}

func envBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}

func Load() Config {
	dsn := os.Getenv("FLOWPAY_DSN")
	if dsn == "" {
		dsn = "postgres://flowpay:flowpay@127.0.0.1:5432/flowpay?sslmode=disable"
	}
	addr := listenAddr("FLOWPAY_ADDR", ":8080")
	reminderAt := reminderTime()
	if os.Getenv("FLOWPAY_REMINDER_INTERVAL") != "" {
		log.Println("warn: FLOWPAY_REMINDER_INTERVAL ya no se usa; los recordatorios corren a la hora de FLOWPAY_REMINDER_TIME")
	}

	publicBase := publicURL("FLOWPAY_PUBLIC_BASE_URL")
	appPublic := publicURL("FLOWPAY_APP_PUBLIC_URL")

	uploadDir := strings.TrimSpace(os.Getenv("FLOWPAY_UPLOAD_DIR"))
	if uploadDir == "" {
		uploadDir = "data/uploads"
	}

	disp := notify.NewDispatcher(notify.Options{
		SMTP: &notify.SMTPConfig{
			Host:     os.Getenv("FLOWPAY_SMTP_HOST"),
			Port:     os.Getenv("FLOWPAY_SMTP_PORT"),
			Username: os.Getenv("FLOWPAY_SMTP_USER"),
			Password: os.Getenv("FLOWPAY_SMTP_PASSWORD"),
			From:     os.Getenv("FLOWPAY_SMTP_FROM"),
		},
		EmailOverride: os.Getenv("FLOWPAY_EMAIL_OVERRIDE"),
		Twilio: &notify.TwilioConfig{
			AccountSID: os.Getenv("FLOWPAY_TWILIO_ACCOUNT_SID"),
			AuthToken:  os.Getenv("FLOWPAY_TWILIO_AUTH_TOKEN"),
			WhatsFrom:  os.Getenv("FLOWPAY_TWILIO_WHATSAPP_FROM"),
			Templates: notify.TemplateSIDs{
				Approaching:     os.Getenv("FLOWPAY_TWILIO_TEMPLATE_APPROACHING"),
				DueToday:        os.Getenv("FLOWPAY_TWILIO_TEMPLATE_DUE_TODAY"),
				OverdueFirst:    os.Getenv("FLOWPAY_TWILIO_TEMPLATE_OVERDUE"),
				OverdueFollowUp: os.Getenv("FLOWPAY_TWILIO_TEMPLATE_FOLLOWUP"),
			},
		},
		WhatsAppOverride: os.Getenv("FLOWPAY_WHATSAPP_OVERRIDE"),
		PublicBaseURL:    publicBase,
	})

	return Config{
		DSN:                   dsn,
		Addr:                  addr,
		ReminderHour:          reminderAt.Hour(),
		ReminderMinute:        reminderAt.Minute(),
		ReminderLocation:      reminderLocation(),
		DefaultCompanyID:      1,
		JWTSecret:             strings.TrimSpace(os.Getenv("FLOWPAY_JWT_SECRET")),
		UploadDir:             uploadDir,
		PublicBaseURL:         publicBase,
		AppPublicURL:          appPublic,
		Notify:                disp,
		TwilioAccountSID:      strings.TrimSpace(os.Getenv("FLOWPAY_TWILIO_ACCOUNT_SID")),
		TwilioAuthToken:       strings.TrimSpace(os.Getenv("FLOWPAY_TWILIO_AUTH_TOKEN")),
		TwilioValidateWebhook: envBool("FLOWPAY_TWILIO_VALIDATE_WEBHOOK"),
	}
}

const defaultReminderTime = "10:00"

// reminderTime hora del día (HH:MM) del ciclo de recordatorios automáticos.
func reminderTime() time.Time {
	v := strings.TrimSpace(os.Getenv("FLOWPAY_REMINDER_TIME"))
	if v != "" {
		if t, err := time.Parse("15:04", v); err == nil {
			return t
		}
		log.Printf("warn: FLOWPAY_REMINDER_TIME=%q no es HH:MM; se usa %s", v, defaultReminderTime)
	}
	t, _ := time.Parse("15:04", defaultReminderTime)
	return t
}

func reminderLocation() *time.Location {
	name := strings.TrimSpace(os.Getenv("FLOWPAY_TIMEZONE"))
	if name == "" {
		name = "America/Santiago"
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		log.Printf("warn: zona horaria %q no válida (%v); se usa la del servidor", name, err)
		return time.Local
	}
	return loc
}

const productionOrigin = "https://geldflus.com"

func onPlatform() bool {
	return strings.TrimSpace(os.Getenv("PORT")) != ""
}

func listenAddr(primaryKey, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(primaryKey)); v != "" {
		return v
	}
	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		if strings.HasPrefix(p, ":") {
			return p
		}
		return ":" + p
	}
	return fallback
}

func publicURL(key string) string {
	v := strings.TrimSuffix(strings.TrimSpace(os.Getenv(key)), "/")
	if v != "" {
		return v
	}
	if onPlatform() {
		return productionOrigin
	}
	return ""
}
