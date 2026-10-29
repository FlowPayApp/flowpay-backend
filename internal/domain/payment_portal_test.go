package domain

import (
	"strings"
	"testing"
)

func TestNormalizePaymentPortalURL(t *testing.T) {
	got, err := NormalizePaymentPortalURL(" HTTPS://Pagos.Acme.CL/cobros?cuenta=1#pago ")
	if err != nil || got != "https://pagos.acme.cl/cobros?cuenta=1" {
		t.Fatalf("url limpia: %q %v", got, err)
	}
	if _, err := NormalizePaymentPortalURL("http://pagos.acme.cl"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("http: %v", err)
	}
	if _, err := NormalizePaymentPortalURL("https://user:pass@pagos.acme.cl"); err == nil {
		t.Fatal("aceptó usuario y contraseña")
	}
	if _, err := NormalizePaymentPortalURL("https://bit.ly/abc"); err == nil {
		t.Fatal("aceptó un acortador")
	}
	if _, err := NormalizePaymentPortalURL("https://www.tinyurl.com/abc"); err == nil {
		t.Fatal("aceptó un acortador con www")
	}
	if _, err := NormalizePaymentPortalURL("https://10.0.0.8/pago"); err == nil {
		t.Fatal("aceptó una IP")
	}
	if _, err := NormalizePaymentPortalURL("https://localhost/pago"); err == nil {
		t.Fatal("aceptó localhost")
	}
	if _, err := NormalizePaymentPortalURL("https://pagos.acme.cl/{{charge_id}}"); err == nil {
		t.Fatal("aceptó datos del cobro")
	}
	if _, err := NormalizePaymentPortalURL("pagos.acme.cl"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("sin esquema: %v", err)
	}
}
