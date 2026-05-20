package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/joho/godotenv/autoload"

	"github.com/flowpay/flowpay-backend/internal/config"
	"github.com/flowpay/flowpay-backend/internal/controller"
	"github.com/flowpay/flowpay-backend/internal/jobs"
	"github.com/flowpay/flowpay-backend/internal/middleware"
	"github.com/flowpay/flowpay-backend/internal/repository"
	"github.com/flowpay/flowpay-backend/internal/routes"
	"github.com/flowpay/flowpay-backend/internal/service"
)

func main() {
	cfg := config.Load()
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(time.Minute * 3)
	if err := db.Ping(); err != nil {
		log.Fatal("postgres ping:", err)
	}

	repo := repository.New(db)
	if err := os.MkdirAll(filepath.Clean(cfg.UploadDir), 0o755); err != nil {
		log.Fatal("upload dir:", err)
	}
	svc := &service.Service{
		Repo:      repo,
		Notify:    cfg.Notify,
		UploadDir: cfg.UploadDir,
	}
	wa := &service.WhatsAppService{Repo: repo}
	deps := controller.Deps{
		Svc:      svc,
		WhatsApp: wa,
		TwilioWebhook: controller.TwilioWebhookDeps{
			AuthToken:               cfg.TwilioAuthToken,
			ValidateTwilioSignature: cfg.TwilioValidateWebhook,
		},
		DefaultCompany: cfg.DefaultCompanyID,
		JWTSecret:      cfg.JWTSecret,
	}

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	routes.Register(r, deps, middleware.BearerJWT(cfg.JWTSecret))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	jobs.StartReminderJob(ctx, repo, cfg.Notify, cfg.ReminderInterval)

	srv := &http.Server{Addr: cfg.Addr, Handler: r}
	go func() {
		printStartupStatus(db, cfg.Addr, cfg.DSN, cfg.ReminderInterval, cfg.JWTSecret, cfg.PublicBaseURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	_ = db.Close()
	log.Println("servidor detenido")
}

func printStartupStatus(db *sql.DB, addr, dsn string, reminderInterval time.Duration, jwtSecret, publicBase string) {
	const (
		reset  = "\033[0m"
		bold   = "\033[1m"
		cyan   = "\033[36m"
		green  = "\033[32m"
		yellow = "\033[33m"
	)
	ok := func(v string) string { return green + "OK" + reset + " " + v }
	warn := func(v string) string { return yellow + "WARN" + reset + " " + v }

	check := func(table string) bool {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`, table).Scan(&exists)
		return err == nil && exists
	}

	log.Println(cyan + "╔══════════════════════════════════════════════════════╗" + reset)
	log.Println(cyan + "║" + reset + " " + bold + "FlowPay API · Estado de inicio" + reset + "                  " + cyan + "║" + reset)
	log.Println(cyan + "╠══════════════════════════════════════════════════════╣" + reset)
	log.Printf(cyan+"║"+reset+" %s", ok("DB conectada"))
	log.Printf(cyan+"║"+reset+" %s", ok("DB destino: "+safeDSN(dsn)))
	log.Printf(cyan+"║"+reset+" %s", ok("HTTP listening en "+addr))
	log.Printf(cyan+"║"+reset+" %s", ok("Healthcheck: GET "+addr+"/health"))
	log.Printf(cyan+"║"+reset+" %s", ok("Reminder job activo ("+reminderInterval.String()+")"))
	log.Printf(cyan+"║"+reset+" %s", ok("Pagos/Webpay: flowpay-payments (repo aparte)"))

	if strings.TrimSpace(jwtSecret) == "" {
		log.Printf(cyan+"║"+reset+" %s", warn("FLOWPAY_JWT_SECRET vacío (modo dev)"))
	} else {
		log.Printf(cyan+"║"+reset+" %s", ok("JWT secret cargado"))
	}

	if strings.TrimSpace(publicBase) == "" {
		log.Printf(cyan+"║"+reset+" %s", warn("FLOWPAY_PUBLIC_BASE_URL vacía (adjuntos WhatsApp)"))
	} else {
		log.Printf(cyan+"║"+reset+" %s", ok("URL pública para adjuntos: "+publicBase))
	}

	required := []string{"companies", "clients", "charges", "users"}
	var miss []string
	for _, t := range required {
		if !check(t) {
			miss = append(miss, t)
		}
	}
	if len(miss) == 0 {
		log.Printf(cyan+"║"+reset+" %s", ok("Tablas críticas en public: "+strings.Join(required, ", ")))
	} else {
		log.Printf(cyan+"║"+reset+" %s", warn("Faltan tablas: "+strings.Join(miss, ", ")))
		log.Printf(cyan+"║"+reset+" %s", warn("Ejecuta Mysql/postgresql_migration/02_schema.sql"))
	}

	log.Printf(cyan+"║"+reset+" %s", fmt.Sprintf("%sAPI base: /api/*%s", bold, reset))
	log.Println(cyan + "╚══════════════════════════════════════════════════════╝" + reset)
}

func safeDSN(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(dsn inválido)"
	}
	if u.User != nil {
		user := u.User.Username()
		if user != "" {
			u.User = url.User(user)
		}
	}
	return u.Redacted()
}
