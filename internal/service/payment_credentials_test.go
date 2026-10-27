package service

import (
	"testing"

	"github.com/flowpay/flowpay-backend/internal/repository"
)

func TestPlanCredentialWrites(t *testing.T) {
	khipu := "khipu-api-key-123"
	blank := ""
	code := "597055555532"
	webpayKey := "webpay-secret-key"
	mall := "597055555541"
	store := "597055555542"
	oneclickKey := "oneclick-secret"
	fintoc := "sk_test_fintoc_key"

	writes, err := planCredentialWrites(SavePaymentCredentialsInput{KhipuAPIKey: &khipu})
	if err != nil || len(writes) != 1 || writes[0].column != "khipu_api_key" || writes[0].plain != khipu {
		t.Fatalf("khipu: %+v %v", writes, err)
	}

	writes, err = planCredentialWrites(SavePaymentCredentialsInput{KhipuAPIKey: &blank})
	if err != nil || len(writes) != 1 || writes[0].plain != "" {
		t.Fatalf("borrar khipu: %+v %v", writes, err)
	}

	writes, err = planCredentialWrites(SavePaymentCredentialsInput{
		WebpayCommerceCode: &code,
		WebpayAPIKey:       &webpayKey,
	})
	if err != nil || len(writes) != 2 {
		t.Fatalf("webpay: %+v %v", writes, err)
	}

	_, err = planCredentialWrites(SavePaymentCredentialsInput{WebpayCommerceCode: &code})
	if err == nil {
		t.Fatal("aceptó Webpay incompleto")
	}

	_, err = planCredentialWrites(SavePaymentCredentialsInput{
		WebpayCommerceCode: &blank,
		WebpayAPIKey:       &webpayKey,
	})
	if err == nil {
		t.Fatal("aceptó borrar solo una parte de Webpay")
	}

	writes, err = planCredentialWrites(SavePaymentCredentialsInput{
		OneclickMallCode:  &mall,
		OneclickStoreCode: &store,
		OneclickAPIKey:    &oneclickKey,
		FintocSecretKey:   &fintoc,
	})
	if err != nil || len(writes) != 4 {
		t.Fatalf("oneclick+fintoc: %+v %v", writes, err)
	}

	spaced := "tiene espacios"
	if _, err := planCredentialWrites(SavePaymentCredentialsInput{KhipuAPIKey: &spaced}); err == nil {
		t.Fatal("aceptó espacios")
	}
	if _, err := planCredentialWrites(SavePaymentCredentialsInput{}); err != nil {
		t.Fatal(err)
	}

	hook := "whsec_fintoc_1234"
	writes, err = planCredentialWrites(SavePaymentCredentialsInput{FintocWebhookSecret: &hook})
	if err != nil || len(writes) != 1 || writes[0].column != "fintoc_webhook_secret" || writes[0].plain != hook {
		t.Fatalf("webhook fintoc: %+v %v", writes, err)
	}
	chosen := "fintoc"
	gateway, ok, err := planTransferGateway(&chosen)
	if err != nil || !ok || gateway != "fintoc" {
		t.Fatalf("transferencia: %q %v %v", gateway, ok, err)
	}
	bad := "webpay"
	if _, _, err := planTransferGateway(&bad); err == nil {
		t.Fatal("aceptó una transferencia que no es Khipu ni Fintoc")
	}
}

func TestClassifyCredentialChange(t *testing.T) {
	khipu := credentialWrite{column: "khipu_api_key", plain: "khipu-api-key-123"}
	clear := credentialWrite{column: "khipu_api_key"}
	fintoc := credentialWrite{column: "fintoc_secret_key", plain: "sk_test_fintoc_key"}
	hook := credentialWrite{column: "fintoc_webhook_secret", plain: "whsec_fintoc_1234"}

	provider, action, err := classifyCredentialChange([]credentialWrite{khipu}, repository.PaymentCredentialFlags{})
	if err != nil || provider != "khipu" || action != "create" {
		t.Fatalf("crear khipu: %s %s %v", provider, action, err)
	}
	provider, action, err = classifyCredentialChange([]credentialWrite{khipu}, repository.PaymentCredentialFlags{Khipu: true})
	if err != nil || action != "edit" {
		t.Fatalf("reemplazar khipu: %s %v", action, err)
	}
	provider, action, err = classifyCredentialChange([]credentialWrite{clear}, repository.PaymentCredentialFlags{Khipu: true})
	if err != nil || action != "remove" {
		t.Fatalf("quitar khipu: %s %v", action, err)
	}
	if _, _, err := classifyCredentialChange([]credentialWrite{clear}, repository.PaymentCredentialFlags{}); err == nil {
		t.Fatal("quitó una credencial que no existe")
	}
	if _, _, err := classifyCredentialChange([]credentialWrite{khipu, fintoc}, repository.PaymentCredentialFlags{}); err == nil {
		t.Fatal("aceptó dos medios en el mismo cambio")
	}
	provider, action, err = classifyCredentialChange([]credentialWrite{fintoc, hook}, repository.PaymentCredentialFlags{Fintoc: true})
	if err != nil || provider != "fintoc" || action != "edit" {
		t.Fatalf("reemplazar fintoc: %s %s %v", provider, action, err)
	}
}

func TestMaskEmail(t *testing.T) {
	if got := maskEmail("Admin@Empresa.cl"); got != "a***@empresa.cl" {
		t.Fatalf("máscara: %s", got)
	}
}
