package controller

import (
	"net/http"
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
	return d.role(c) == "platform_admin"
}

func (d *Deps) role(c *gin.Context) string {
	v, ok := c.Get("role")
	if !ok {
		return ""
	}
	role, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(strings.ToLower(role))
}

func (d *Deps) userID(c *gin.Context) int64 {
	if v, ok := c.Get("user_id"); ok {
		if id, ok := v.(int64); ok {
			return id
		}
	}
	return 0
}

func (d *Deps) memberUID(c *gin.Context) int64 {
	if d.role(c) == "member" {
		return d.userID(c)
	}
	return 0
}

func (d *Deps) requireCompanyAdmin(c *gin.Context) bool {
	if d.role(c) == "admin" {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "solo el administrador de la empresa puede hacer esta acción"})
	return false
}
