package main

import "testing"

func TestLocalhostCORS(t *testing.T) {
	t.Setenv("FLOWPAY_ENV", "")
	t.Setenv("PORT", "")
	t.Setenv("FLOWPAY_CORS_ORIGINS", "")
	if !allowBrowserOrigin("http://localhost:5173") || !allowBrowserOrigin("http://127.0.0.1:5173") {
		t.Fatal("en local, localhost y 127.0.0.1 tienen que pasar")
	}

	t.Setenv("FLOWPAY_ENV", "production")
	if allowBrowserOrigin("http://localhost:5173") || allowBrowserOrigin("http://127.0.0.1:4173") {
		t.Fatal("en producción, localhost no es un origen permitido")
	}
	if !allowBrowserOrigin("https://app.geldflus.com") {
		t.Fatal("el panel público tiene que seguir permitido")
	}
}
