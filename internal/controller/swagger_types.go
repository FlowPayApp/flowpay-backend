package controller

// Tipos que solo describen respuestas en la documentación Swagger; los handlers responden con gin.H.

type errorResponse struct {
	Error string `json:"error" example:"not found"`
}

type okResponse struct {
	OK bool `json:"ok" example:"true"`
}

type idResponse struct {
	ID int64 `json:"id" example:"42"`
}

type reminderCooldownResponse struct {
	Error       string `json:"error"`
	Channel     string `json:"channel" example:"whatsapp"`
	AvailableAt string `json:"available_at" example:"2026-11-02T15:04:05Z"`
}
