package routes

import (
	"net/http"

	"github.com/flowpay/flowpay-backend/internal/controller"
	"github.com/flowpay/flowpay-backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

func Register(r *gin.Engine, deps controller.Deps, jwtMiddleware gin.HandlerFunc, allowWhatsAppSimulate bool) {
	r.GET("/api/public/attachments/:token", deps.PublicAttachment)
	r.GET("/api/public/chat-files/:token/:name", deps.PublicChatFile)
	limitTwilio := middleware.TwilioWebhookLimit()
	r.POST("/api/webhooks/twilio/whatsapp", limitTwilio, deps.TwilioWhatsAppWebhook)
	r.POST("/api/webhooks/twilio/status", limitTwilio, deps.TwilioStatusWebhook)

	r.GET("/api/public/credential-confirm/:token", deps.PreviewPaymentCredentialChange)
	r.POST("/api/public/credential-confirm/:token", deps.ConfirmPaymentCredentialChange)

	api := r.Group("/api")
	api.Use(jwtMiddleware)
	{
		api.GET("/charges", deps.ListCharges)
		api.POST("/charges", deps.CreateCharge)
		api.GET("/charges/:id", deps.GetCharge)
		api.PATCH("/charges/:id", deps.PatchCharge)
		api.DELETE("/charges/:id", deps.DeleteCharge)
		api.GET("/charges/:id/reminders", deps.ListReminders)
		api.GET("/charges/:id/inbound-whatsapp", deps.ListChargeInboundWhatsApp)
		api.GET("/charges/:id/inbound-whatsapp/:msgId/media/:index", deps.ChargeInboundMedia)
		api.POST("/charges/:id/whatsapp", deps.ReplyChargeWhatsApp)
		api.POST("/charges/:id/whatsapp/file", deps.SendChargeWhatsAppFile)
		api.POST("/charges/:id/read", deps.MarkChargeRead)
		api.GET("/inbox", deps.Inbox)
		if allowWhatsAppSimulate {
			api.POST("/charges/:id/inbound-whatsapp/simulate", deps.SimulateChargeInboundWhatsApp)
		}
		api.POST("/charges/:id/reminders", deps.SendReminder)
		api.POST("/charges/:id/attachment", deps.UploadChargeAttachment)
		api.GET("/dashboard", deps.Dashboard)
		api.GET("/platform/overview", deps.PlatformOverview)
		api.GET("/platform/whatsapp-numbers", deps.ListPlatformWhatsAppNumbers)
		api.PUT("/platform/companies/:id/whatsapp", deps.PutCompanyWhatsApp)
		api.GET("/platform/mailboxes", deps.ListPlatformMailboxes)
		api.PUT("/platform/companies/:id/mailbox", deps.PutCompanyMailbox)
		api.GET("/company/clients/export", deps.ExportClients)
		api.GET("/company/channels", deps.GetCompanyChannels)
		api.GET("/company/messaging", deps.GetCompanyMessaging)
		api.PUT("/company/messaging", deps.PutCompanyMessaging)
		api.GET("/company/payment-portal", deps.GetPaymentPortal)
		api.PUT("/company/payment-portal", deps.PutPaymentPortal)
		api.GET("/platform/payment-portals", deps.ListPendingPaymentPortals)
		api.POST("/platform/companies/:id/payment-portal/approve", deps.ApprovePaymentPortal)
		api.POST("/platform/companies/:id/payment-portal/reject", deps.RejectPaymentPortal)
		api.GET("/company/payment-credentials", deps.GetPaymentCredentials)
		api.PUT("/company/payment-credentials", deps.PutPaymentCredentials)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "flowpay-backend"})
	})
}
