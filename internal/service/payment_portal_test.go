package service

import "testing"

func TestNextPaymentPortal(t *testing.T) {
	approved := paymentPortalState{Mode: "external", URL: "https://pagos.acme.cl"}

	next, action, err := nextPaymentPortal(approved, SavePaymentPortalInput{Mode: "external", URL: "https://pagos.acme.cl/nuevo"})
	if err != nil || action != "pending" || next.Mode != "external" || next.URL != approved.URL || next.Pending != "https://pagos.acme.cl/nuevo" {
		t.Fatalf("nueva url: %+v %s %v", next, action, err)
	}

	next, action, err = nextPaymentPortal(paymentPortalState{Mode: "flowpay"}, SavePaymentPortalInput{Mode: "external", URL: "http://pagos.acme.cl"})
	if err == nil || action != "" {
		t.Fatalf("http no debe quedar pendiente: %+v %s", next, action)
	}

	next, action, err = nextPaymentPortal(approved, SavePaymentPortalInput{Mode: "flowpay"})
	if err != nil || action != "flowpay" || next.Mode != "flowpay" || next.URL != approved.URL || next.Pending != "" {
		t.Fatalf("volver a geldflus: %+v %s %v", next, action, err)
	}

	next, action, err = nextPaymentPortal(paymentPortalState{Mode: "flowpay", URL: approved.URL}, SavePaymentPortalInput{Mode: "external", URL: approved.URL})
	if err != nil || action != "enable" || next.Mode != "external" || next.Pending != "" {
		t.Fatalf("reutilizar aprobada: %+v %s %v", next, action, err)
	}

	same := paymentPortalState{Mode: "flowpay", Pending: "https://pagos.acme.cl/nuevo"}
	next, action, err = nextPaymentPortal(same, SavePaymentPortalInput{Mode: "external", URL: same.Pending})
	if err != nil || action != "same" {
		t.Fatalf("la misma pendiente: %+v %s %v", next, action, err)
	}
}
