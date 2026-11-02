// Package audit es el registro único de FlowPay.
// Cada fila vive en audit_events. Para buscar: filtrar por domain
// (auth, tenant, client, charge, payment, credential, message).
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
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
	Summary    string
	Meta       map[string]any
}

type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type httpStamp struct {
	IP        string
	UserAgent string
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

func WithHTTP(ctx context.Context, ip, userAgent string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, keyHTTP, httpStamp{IP: ip, UserAgent: userAgent})
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
			summary TEXT NOT NULL DEFAULT '',
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb
		)`,
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
	target_type, target_id, ip, user_agent, summary, metadata
) VALUES (
	$1, $2, $3, $4,
	NULLIF($5::bigint, 0), $6, NULLIF($7::bigint, 0),
	$8, $9, $10, $11, $12, $13::jsonb
)`,
		e.Service, e.Domain, e.Action, e.Outcome,
		e.ActorID, e.ActorRole, e.CompanyID,
		e.TargetType, e.TargetID, e.IP, e.UserAgent, e.Summary, string(meta),
	)
	return err
}

// StampHTTP guarda IP y user agent en el contexto de cada petición.
func StampHTTP() gin.HandlerFunc {
	return func(c *gin.Context) {
		ua := ""
		if c.Request != nil {
			ua = c.Request.UserAgent()
		}
		c.Request = c.Request.WithContext(WithHTTP(c.Request.Context(), c.ClientIP(), ua))
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
