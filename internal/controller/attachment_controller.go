package controller

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AttachmentController struct{ Deps }

func (h *AttachmentController) PublicGet(c *gin.Context) {
	token := c.Param("token")
	f, mimeType, err := h.Svc.OpenPublicAttachment(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.Status(http.StatusNotFound)
			return
		}
		c.Status(http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Type", mimeType)
	c.Header("Content-Disposition", `inline; filename="cobro"`)
	c.DataFromReader(http.StatusOK, st.Size(), mimeType, f, nil)
}
