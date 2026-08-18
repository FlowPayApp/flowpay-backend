package routes

import (
	"net/http"

	"github.com/flowpay/flowpay-backend/internal/controller"
	"github.com/gin-gonic/gin"
)

func Register(r *gin.Engine, deps controller.Deps, jwtMiddleware gin.HandlerFunc) {
	r.GET("/api/public/attachments/:token", deps.PublicAttachment)
	r.POST("/api/webhooks/twilio/whatsapp", deps.TwilioWhatsAppWebhook)

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
		api.POST("/charges/:id/inbound-whatsapp/simulate", deps.SimulateChargeInboundWhatsApp)
		api.POST("/charges/:id/reminders", deps.SendReminder)
		api.POST("/charges/:id/attachment", deps.UploadChargeAttachment)
		api.GET("/dashboard", deps.Dashboard)
		api.GET("/platform/overview", deps.PlatformOverview)
		api.GET("/company/messaging", deps.GetCompanyMessaging)
		api.PUT("/company/messaging", deps.PutCompanyMessaging)
		api.POST("/reminder-messages", deps.CreateReminderMessage)
		api.PATCH("/reminder-messages/:id", deps.PatchReminderMessage)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "flowpay-backend"})
	})
}
