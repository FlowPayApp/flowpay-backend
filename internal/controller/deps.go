package controller

import (
	"strconv"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/gin-gonic/gin"
)

type Deps struct {
	Svc            *service.Service
	WhatsApp       *service.WhatsAppService
	TwilioWebhook  TwilioWebhookDeps
	DefaultCompany int64
	JWTSecret      string
}

func (d *Deps) companyID(c *gin.Context) int64 {
	if d.JWTSecret != "" {
		if v, ok := c.Get("company_id"); ok {
			if id, ok := v.(int64); ok {
				return id
			}
		}
		return d.DefaultCompany
	}
	if v := c.Query("company_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			return id
		}
	}
	return d.DefaultCompany
}

func (d *Deps) isPlatformAdmin(c *gin.Context) bool {
	v, ok := c.Get("role")
	if !ok {
		return false
	}
	role, ok := v.(string)
	if !ok {
		return false
	}
	return strings.TrimSpace(strings.ToLower(role)) == "platform_admin"
}
