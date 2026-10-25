package service

import "testing"

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
}
