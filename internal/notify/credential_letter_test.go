package notify

import "strings"
import "testing"

func TestCredentialLetterOmitsSecrets(t *testing.T) {
	const secret = "sk_test_SUPER_SECRET_VALUE"
	const link = "http://localhost:5173/confirmar-credencial/token-de-prueba"
	note := CredentialNotice{
		Greeting: "Estimado Pablo,",
		Lead:     "Hay una solicitud para reemplazar las credenciales de Fintoc en Flowpay. El enlace de pago no cambia hasta que usted confirme.",
		Notes: []string{
			"Este enlace caduca en 30 minutos y se puede usar una sola vez.",
			"Si usted no pidió este cambio, ignore el mensaje. Las credenciales actuales siguen vigentes.",
		},
		Rows: [][2]string{
			{"Empresa", "Flowpay"},
			{"Medio", "Fintoc"},
			{"Acción", "Reemplazar"},
		},
		ActionURL: link,
	}
	htmlBody, err := note.html()
	if err != nil {
		t.Fatal(err)
	}
	plain := note.plain()
	for _, body := range []string{htmlBody, plain} {
		if strings.Contains(body, secret) {
			t.Fatal("el correo incluyó un secreto")
		}
	}
	if !strings.Contains(htmlBody, "Confirmar cambio") || !strings.Contains(htmlBody, link) {
		t.Fatal("falta el botón o el enlace")
	}
	if !strings.Contains(plain, "Confirmar cambio: "+link) || !strings.Contains(plain, "Fintoc") {
		t.Fatal("el texto plano no cuenta el cambio")
	}
}
