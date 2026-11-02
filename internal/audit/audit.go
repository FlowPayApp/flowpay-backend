// Package audit es el registro único de FlowPay.
// Cada fila vive en audit_events. Para buscar: filtrar por domain
// (auth, tenant, client, charge, payment, credential, message).
package audit

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const ServiceName = "backend"

const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
	OutcomeDenied  = "denied"
)

const (
	DomainAuth       = "auth"
	DomainTenant     = "tenant"
	DomainClient     = "client"
	DomainCharge     = "charge"
	DomainPayment    = "payment"
	DomainCredential = "credential"
	DomainMessage    = "message"
)

const (
	TargetUser       = "user"
	TargetCompany    = "company"
	TargetClient     = "client"
	TargetCharge     = "charge"
	TargetPayment    = "payment"
	TargetCredential = "credential"
	TargetSession    = "session"
	TargetPlan       = "plan"
	TargetMessage    = "message"
	TargetReminder   = "reminder"
)

// Event es una fila. Service, IP y actor se completan con Finish si vienen vacíos.
type Event struct {
	Service    string
	Domain     string
	Action     string
	Outcome    string
	ActorID    int64
	ActorRole  string
	CompanyID  int64
	TargetType string
	TargetID   string
	IP         string
	UserAgent  string
	ClientHint string
	Summary    string
	Meta       map[string]any
}

type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type httpStamp struct {
	IP         string
	UserAgent  string
	ClientHint string
}

type actorStamp struct {
	ActorID   int64
	CompanyID int64
	Role      string
}

type ctxKey int

const (
	keyHTTP ctxKey = iota
	keyActor
)

func WithHTTP(ctx context.Context, ip, userAgent, clientHint string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, keyHTTP, httpStamp{IP: ip, UserAgent: userAgent, ClientHint: clientHint})
}

func WithActor(ctx context.Context, actorID, companyID int64, role string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, keyActor, actorStamp{ActorID: actorID, CompanyID: companyID, Role: role})
}

// Finish completa servicio, resultado, IP y actor desde el contexto de la petición.
func Finish(ctx context.Context, e Event) Event {
	if e.Service == "" {
		e.Service = ServiceName
	}
	if e.Outcome == "" {
		e.Outcome = OutcomeSuccess
	}
	if ctx == nil {
		return clip(e)
	}
	if http, ok := ctx.Value(keyHTTP).(httpStamp); ok {
		if e.IP == "" {
			e.IP = http.IP
		}
		if e.UserAgent == "" {
			e.UserAgent = http.UserAgent
		}
		if e.ClientHint == "" {
			e.ClientHint = http.ClientHint
		}
	}
	if actor, ok := ctx.Value(keyActor).(actorStamp); ok {
		if e.ActorID == 0 {
			e.ActorID = actor.ActorID
		}
		if e.CompanyID == 0 {
			e.CompanyID = actor.CompanyID
		}
		if e.ActorRole == "" {
			e.ActorRole = actor.Role
		}
	}
	return clip(e)
}

func clip(e Event) Event {
	e.Service = clipText(e.Service, 32)
	e.Domain = clipText(e.Domain, 32)
	e.Action = clipText(e.Action, 64)
	e.Outcome = clipText(e.Outcome, 16)
	e.ActorRole = clipText(e.ActorRole, 32)
	e.TargetType = clipText(e.TargetType, 32)
	e.TargetID = clipText(e.TargetID, 64)
	e.IP = clipText(e.IP, 64)
	e.UserAgent = clipText(e.UserAgent, 300)
	e.ClientHint = clipText(e.ClientHint, 300)
	e.Summary = clipText(e.Summary, 400)
	return e
}

func clipText(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}

// ReminderFailure arma la frase y el detalle de un aviso que no salió.
func ReminderFailure(kind, channel string, cause error) (string, map[string]any) {
	summary := "No se pudo enviar el recordatorio por correo"
	code := "reminder_email"
	if channel == "whatsapp" {
		summary = "No se pudo enviar el recordatorio por WhatsApp"
		code = "reminder_whatsapp"
	}
	reason := Cause(cause)
	if reason != "" && !strings.Contains(summary, reason) {
		summary += ": " + reason
	}
	meta := map[string]any{"channel": channel, "kind": kind, "code": code}
	if reason != "" {
		meta["reason"] = reason
	}
	return summary, meta
}

// Cause deja el motivo de un fallo, corto y sin secretos.
func Cause(err error) string {
	if err == nil {
		return ""
	}
	return Scrub(err.Error())
}

// Scrub aplana un texto de error y corta valores que parecen secretos.
func Scrub(raw string) string {
	raw = strings.Join(strings.Fields(raw), " ")
	lower := strings.ToLower(raw)
	for _, key := range []string{"password=", "passwd=", "secret=", "api_key=", "apikey=", "authorization=", "tbk-api-key-secret="} {
		if i := strings.Index(lower, key); i >= 0 {
			raw = raw[:i+len(key)] + "***"
			lower = strings.ToLower(raw)
		}
	}
	return clipText(raw, 180)
}

// ID formatea un id numérico. Cero queda vacío.
func ID(n int64) string {
	if n == 0 {
		return ""
	}
	return strconv.FormatInt(n, 10)
}

// MaskEmail deja la inicial y el dominio. Sirve para intentos de ingreso.
func MaskEmail(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	local := email[:at]
	domain := email[at:]
	if local == "" {
		return "***" + domain
	}
	r := []rune(local)
	return string(r[:1]) + "***" + domain
}

var (
	onceMu    sync.Mutex
	onceUntil map[string]time.Time
)

// Once deja pasar la primera vez de una clave dentro de la ventana.
// Sirve para que un reintento o un escaneo no llene la tabla.
func Once(key string, window time.Duration) bool {
	key = strings.TrimSpace(key)
	if key == "" || window <= 0 {
		return true
	}
	now := time.Now()
	onceMu.Lock()
	defer onceMu.Unlock()
	if onceUntil == nil {
		onceUntil = map[string]time.Time{}
	}
	if len(onceUntil) > 512 {
		for k, until := range onceUntil {
			if !now.Before(until) {
				delete(onceUntil, k)
			}
		}
	}
	if until, ok := onceUntil[key]; ok && now.Before(until) {
		return false
	}
	onceUntil[key] = now.Add(window)
	return true
}

func SchemaStatements() []string {
	return []string{
		`DROP TABLE IF EXISTS auth_audit`,
		`DROP TABLE IF EXISTS audit_log`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id BIGSERIAL PRIMARY KEY,
			occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			service TEXT NOT NULL,
			domain TEXT NOT NULL,
			action TEXT NOT NULL,
			outcome TEXT NOT NULL,
			actor_user_id BIGINT,
			actor_role TEXT NOT NULL DEFAULT '',
			company_id BIGINT,
			target_type TEXT NOT NULL DEFAULT '',
			target_id TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			client_hint TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb
		)`,
		`ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS client_hint TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_occurred ON audit_events (occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_domain ON audit_events (domain, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_company ON audit_events (company_id, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_actor ON audit_events (actor_user_id, occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_target ON audit_events (target_type, target_id)`,
		`COMMENT ON TABLE audit_events IS 'Registro único de auditoría. Buscar por domain: auth, tenant, client, charge, payment, credential, message.'`,
	}
}

func Insert(ctx context.Context, ex Execer, e Event) error {
	e = Finish(ctx, e)
	if e.Domain == "" || e.Action == "" {
		return nil
	}
	meta := []byte("{}")
	if len(e.Meta) > 0 {
		raw, err := json.Marshal(e.Meta)
		if err != nil {
			return err
		}
		meta = raw
	}
	_, err := ex.ExecContext(ctx, `
INSERT INTO audit_events (
	service, domain, action, outcome,
	actor_user_id, actor_role, company_id,
	target_type, target_id, ip, user_agent, client_hint, summary, metadata
) VALUES (
	$1, $2, $3, $4,
	NULLIF($5::bigint, 0), $6, NULLIF($7::bigint, 0),
	$8, $9, $10, $11, $12, $13, $14::jsonb
)`,
		e.Service, e.Domain, e.Action, e.Outcome,
		e.ActorID, e.ActorRole, e.CompanyID,
		e.TargetType, e.TargetID, e.IP, e.UserAgent, e.ClientHint, e.Summary, string(meta),
	)
	return err
}

// RequestIP es la IP de quien usa el navegador.
// Detrás de Vite o Railway la conexión directa es el proxy; la IP real viaja en X-Flowpay-Client-Ip.
func RequestIP(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return chooseIP(c.RemoteIP(), c.GetHeader("X-Flowpay-Client-Ip"), c.GetHeader("X-Forwarded-For"), c.GetHeader("X-Real-Ip"), c.GetHeader("X-Flowpay-Edge"))
}

func chooseIP(remote, marked, forwarded, realIP, edgeHeader string) string {
	if edgeOK(edgeHeader) {
		if ip := cleanIP(marked); ip != "" {
			return ip
		}
	}
	return pickIP(remote, marked, forwarded, realIP)
}

func edgeOK(header string) bool {
	secret := strings.TrimSpace(os.Getenv("FLOWPAY_EDGE_SECRET"))
	header = strings.TrimSpace(header)
	if secret == "" || header == "" || len(secret) != len(header) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(header), []byte(secret)) == 1
}

func pickIP(remote, marked, forwarded, realIP string) string {
	if trustedProxy(remote) {
		if ip := cleanIP(marked); ip != "" {
			return ip
		}
		if ip := firstIP(forwarded); ip != "" {
			return ip
		}
		if ip := cleanIP(realIP); ip != "" {
			return ip
		}
	}
	if ip := cleanIP(remote); ip != "" {
		return ip
	}
	return ""
}

func trustedProxy(ip string) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return false
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() {
		return true
	}
	// 100.64.0.0/10 lo usan balanceadores (Railway). No es la IP de la persona.
	v4 := parsed.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64
}

func firstIP(header string) string {
	for _, part := range strings.Split(header, ",") {
		if ip := cleanIP(part); ip != "" {
			return ip
		}
	}
	return ""
}

func cleanIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	parsed := net.ParseIP(value)
	if parsed == nil {
		return ""
	}
	return parsed.String()
}

// preferHint elige la marca del navegador. Brave y otros se anuncian ahí, y en el agente escriben Chrome.
func preferHint(marked, native string) string {
	marked = strings.TrimSpace(marked)
	native = strings.TrimSpace(native)
	if specificBrand(marked) {
		return marked
	}
	if native != "" {
		return native
	}
	return marked
}

func specificBrand(hint string) bool {
	for _, name := range brandNames(hint) {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "not") && strings.Contains(lower, "brand") {
			continue
		}
		if lower == "chromium" || lower == "google chrome" {
			continue
		}
		return true
	}
	return false
}

func brandNames(hint string) []string {
	var names []string
	for len(hint) > 0 {
		start := strings.Index(hint, `"`)
		if start < 0 {
			break
		}
		hint = hint[start+1:]
		end := strings.Index(hint, `"`)
		if end < 0 {
			break
		}
		name := hint[:end]
		hint = strings.TrimSpace(hint[end+1:])
		if strings.HasPrefix(hint, ";v=") {
			names = append(names, name)
		}
	}
	return names
}

// StampHTTP guarda IP, user agent y la marca del navegador en el contexto de cada petición.
func StampHTTP() gin.HandlerFunc {
	return func(c *gin.Context) {
		ua := ""
		hint := ""
		if c.Request != nil {
			ua = c.Request.UserAgent()
			hint = preferHint(c.GetHeader("X-Flowpay-Client-Brands"), c.GetHeader("Sec-CH-UA"))
		}
		c.Request = c.Request.WithContext(WithHTTP(c.Request.Context(), RequestIP(c), ua, hint))
		c.Next()
	}
}

// StampActor copia el usuario del JWT al contexto. Va después del middleware de token.
func StampActor() gin.HandlerFunc {
	return func(c *gin.Context) {
		var actorID, companyID int64
		role := ""
		if v, ok := c.Get("user_id"); ok {
			if id, ok := v.(int64); ok {
				actorID = id
			}
		}
		if v, ok := c.Get("company_id"); ok {
			if id, ok := v.(int64); ok {
				companyID = id
			}
		}
		if v, ok := c.Get("role"); ok {
			if s, ok := v.(string); ok {
				role = s
			}
		}
		c.Request = c.Request.WithContext(WithActor(c.Request.Context(), actorID, companyID, role))
		c.Next()
	}
}
