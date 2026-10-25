package secretbox

import (
	"bytes"
	"strings"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	key, err := KeyFromMaterial("esta-es-una-clave-de-prueba-de-32+")
	if err != nil {
		t.Fatal(err)
	}
	aad := []byte("7|khipu_api_key")
	plain := []byte("sk_live_ejemplo")
	sealed, err := Seal(key, plain, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "v1.") {
		t.Fatalf("formato: %s", sealed)
	}
	if bytes.Contains(plain, []byte("sk_live")) {
		t.Fatal("el texto en claro quedó en el buffer")
	}
	got, err := Open(key, sealed, aad)
	if err != nil {
		t.Fatal(err)
	}
	if got != "sk_live_ejemplo" {
		t.Fatalf("got %q", got)
	}
}

func TestOpenRejectsSwappedContext(t *testing.T) {
	key, err := KeyFromMaterial("esta-es-una-clave-de-prueba-de-32+")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := Seal(key, []byte("llave-de-la-empresa"), []byte("7|khipu_api_key"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(key, sealed, []byte("8|khipu_api_key")); err == nil {
		t.Fatal("aceptó el secreto de otra empresa")
	}
	if _, err := Open(key, sealed, []byte("7|fintoc_secret_key")); err == nil {
		t.Fatal("aceptó el secreto en otra columna")
	}
	other, err := KeyFromMaterial("otra-clave-distinta-con-32-caracteres")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(other, sealed, []byte("7|khipu_api_key")); err == nil {
		t.Fatal("aceptó otra clave")
	}
}

func TestKeyFromMaterialRejectsShort(t *testing.T) {
	if _, err := KeyFromMaterial("corta"); err != ErrKeyMaterial {
		t.Fatalf("err = %v", err)
	}
}
