package routes

import (
	"net/http"

	"github.com/flowpay/flowpay-backend/internal/controller"
	"github.com/gin-gonic/gin"
)

func Register(r *gin.Engine, deps controller.Deps, jwtMiddleware gin.HandlerFunc) {
	charges := controller.ChargeController{Deps: deps}
	dashboard := controller.DashboardController{Deps: deps}
	messaging := controller.MessagingController{Deps: deps}
	attachment := controller.AttachmentController{Deps: deps}
	webhook := controller.WebhookController{Deps: deps}

	r.GET("/api/public/attachments/:token", attachment.PublicGet)
	r.POST("/api/webhooks/twilio/whatsapp", webhook.TwilioWhatsApp)

	api := r.Group("/api")
	api.Use(jwtMiddleware)
	{
		api.GET("/charges", charges.List)
		api.POST("/charges", charges.Create)
		api.GET("/charges/:id", charges.Get)
		api.PATCH("/charges/:id", charges.Patch)
		api.DELETE("/charges/:id", charges.Delete)
		api.GET("/charges/:id/reminders", charges.ListReminders)
		api.GET("/charges/:id/inbound-whatsapp", charges.ListInboundWhatsApp)
		api.POST("/charges/:id/inbound-whatsapp/simulate", charges.SimulateInboundWhatsApp)
		api.POST("/charges/:id/reminders", charges.SendReminder)
		api.POST("/charges/:id/attachment", charges.UploadAttachment)
		api.GET("/dashboard", dashboard.Get)
		api.GET("/platform/overview", dashboard.PlatformOverview)
		api.GET("/company/messaging", messaging.Get)
		api.PUT("/company/messaging", messaging.Put)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}
