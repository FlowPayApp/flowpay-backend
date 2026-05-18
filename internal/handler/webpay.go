package handler

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/gin-gonic/gin"
)

type checkoutBody struct {
	ChargeRefs []string `json:"charge_refs"`
}

type commitBody struct {
	TokenWS string `json:"token_ws"`
}

func (h *HTTP) publicPayCheckout(c *gin.Context) {
	var body checkoutBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	out, err := h.Svc.StartWebpayCheckout(c.Request.Context(), c.Param("token"), body.ChargeRefs)
	if err != nil {
		writeWebpayError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *HTTP) publicPayCommit(c *gin.Context) {
	var body commitBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	tokenWS := strings.TrimSpace(body.TokenWS)
	if tokenWS == "" {
		tokenWS = strings.TrimSpace(c.Query("token_ws"))
	}
	out, err := h.Svc.CommitWebpayReturn(c.Request.Context(), c.Param("token"), tokenWS)
	if err != nil {
		writeWebpayError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// webpayReturn: return_url de Transbank → commit en servidor → redirect al front.
func (h *HTTP) webpayReturn(c *gin.Context) {
	portalToken := c.Param("token")
	tokenWS := strings.TrimSpace(c.Query("token_ws"))
	if tokenWS == "" {
		tokenWS = strings.TrimSpace(c.Query("TBK_TOKEN"))
	}
	if tokenWS == "" {
		loc := h.Svc.FrontendReturnURL(portalToken, &service.WebpayCommitResult{
			Status:  "failed",
			Message: "pago cancelado o incompleto",
		})
		c.Redirect(http.StatusFound, loc)
		return
	}
	out, err := h.Svc.CommitWebpayReturn(c.Request.Context(), portalToken, tokenWS)
	if err != nil {
		writeWebpayError(c, err)
		return
	}
	c.Redirect(http.StatusFound, h.Svc.FrontendReturnURL(portalToken, out))
}

// webpayBridge: auto-POST a Webpay (Transbank exige POST con token_ws).
func (h *HTTP) webpayBridge(c *gin.Context) {
	var txID int64
	if _, err := fmt.Sscanf(c.Param("id"), "%d", &txID); err != nil || txID <= 0 {
		c.String(http.StatusBadRequest, "id inválido")
		return
	}
	actionURL, token, err := h.Svc.WebpayBridge(c.Request.Context(), txID)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	action := html.EscapeString(actionURL)
	tok := html.EscapeString(token)
	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="es"><head><meta charset="utf-8"><title>Redirigiendo a Webpay</title></head>
<body>
<p>Redirigiendo a Webpay…</p>
<form id="f" method="post" action="%s"><input type="hidden" name="token_ws" value="%s"/></form>
<script>document.getElementById('f').submit();</script>
</body></html>`, action, tok)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}

func writeWebpayError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrPaymentTokenNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "token no encontrado"})
	case errors.Is(err, service.ErrInvalidChargeRefs):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrWebpayNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrWebpayTxNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}
