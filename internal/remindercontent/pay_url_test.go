package remindercontent

import "testing"

func TestChoosePayURL(t *testing.T) {
	const flowpay = "https://geldflus.com/pay/token"
	const external = "https://pagos.acme.cl"
	if got := ChoosePayURL("external", external, flowpay, ""); got != external {
		t.Fatalf("portal propio: %s", got)
	}
	if got := ChoosePayURL("flowpay", external, flowpay, ""); got != flowpay {
		t.Fatalf("portal geldflus aunque haya una url guardada: %s", got)
	}
	if got := ChoosePayURL("external", "  ", flowpay, ""); got != flowpay {
		t.Fatalf("externo sin url: %s", got)
	}
	if got := ChoosePayURL("flowpay", "", "", "https://plantilla"); got != "https://plantilla" {
		t.Fatalf("plantilla: %s", got)
	}
}
