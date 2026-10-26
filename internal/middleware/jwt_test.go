package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flowpay/flowpay-backend/internal/authjwt"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func init() {
	gin.SetMode(gin.TestMode)
}

const testSecret = "esta-es-una-clave-de-prueba-32"

func signAccess(t *testing.T, jti string) string {
	t.Helper()
	claims := authjwt.AccessClaims{
		UserID:    7,
		CompanyID: 3,
		Email:     "a@b.c",
		Role:      "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func callBearer(secret, raw string, revoked func(string) bool) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET("/api/charges", BearerJWT(secret, revoked), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/charges", nil)
	if raw != "" {
		req.Header.Set("Authorization", "Bearer "+raw)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestBearerJWTRejectsRevokedAndAllowsMissingJTI(t *testing.T) {
	revoked := func(jti string) bool { return jti == "revocado" }
	if rec := callBearer(testSecret, signAccess(t, "revocado"), revoked); rec.Code != http.StatusUnauthorized {
		t.Fatalf("jti revocado: status %d", rec.Code)
	}
	if rec := callBearer(testSecret, signAccess(t, ""), revoked); rec.Code != http.StatusNoContent {
		t.Fatalf("jti vacío: status %d", rec.Code)
	}
	if rec := callBearer(testSecret, signAccess(t, "vigente"), revoked); rec.Code != http.StatusNoContent {
		t.Fatalf("jti vigente: status %d", rec.Code)
	}
}

func TestBearerJWTEmptySecretSkipsAuth(t *testing.T) {
	if rec := callBearer("", "", func(string) bool { return true }); rec.Code != http.StatusNoContent {
		t.Fatalf("secreto vacío: status %d", rec.Code)
	}
}
