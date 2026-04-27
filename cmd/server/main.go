package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/joho/godotenv/autoload"

	"github.com/flowpay/flowpay-backend/internal/config"
	"github.com/flowpay/flowpay-backend/internal/handler"
	"github.com/flowpay/flowpay-backend/internal/jobs"
	"github.com/flowpay/flowpay-backend/internal/middleware"
	"github.com/flowpay/flowpay-backend/internal/repository"
	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/flowpay/flowpay-backend/internal/services"
)

func main() {
	cfg := config.Load()
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(time.Minute * 3)
	if err := db.Ping(); err != nil {
		log.Fatal("mysql ping:", err)
	}

	repo := repository.New(db)
	if err := os.MkdirAll(filepath.Clean(cfg.UploadDir), 0o755); err != nil {
		log.Fatal("upload dir:", err)
	}
	svc := &service.Service{Repo: repo, Notify: cfg.Notify, UploadDir: cfg.UploadDir}
	wa := &services.WhatsAppService{
		Repo:       repo,
		AccountSID: cfg.TwilioAccountSID,
		AuthToken:  cfg.TwilioAuthToken,
	}
	h := &handler.HTTP{
		Svc:      svc,
		WhatsApp: wa,
		TwilioWebhook: handler.TwilioWebhookDeps{
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
	h.Register(r, middleware.BearerJWT(cfg.JWTSecret))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	jobs.StartReminderJob(ctx, repo, cfg.Notify, cfg.ReminderInterval)

	srv := &http.Server{Addr: cfg.Addr, Handler: r}
	go func() {
		log.Printf("FlowPay API escuchando en %s", cfg.Addr)
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
