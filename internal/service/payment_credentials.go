package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/flowpay/flowpay-backend/internal/secretbox"
)

// ErrPaymentCredentialKey el servidor no puede cifrar. No incluye la clave.
var ErrPaymentCredentialKey = errors.New("el servidor no tiene configurado el cifrado de credenciales")

// PaymentCredentialError es un rechazo de los datos que envió el administrador.
type PaymentCredentialError struct{ msg string }

func (e *PaymentCredentialError) Error() string { return e.msg }

// PaymentCredentialStatus medios que tienen credencial. Nunca incluye el secreto.
type PaymentCredentialStatus struct {
	Khipu    bool `json:"khipu"`
	Webpay   bool `json:"webpay"`
	Oneclick bool `json:"oneclick"`
	Fintoc   bool `json:"fintoc"`
}

// SavePaymentCredentialsInput un campo ausente no se toca. Vacío borra. Un valor reemplaza.
type SavePaymentCredentialsInput struct {
	KhipuAPIKey        *string `json:"khipu_api_key"`
	WebpayCommerceCode *string `json:"webpay_commerce_code"`
	WebpayAPIKey       *string `json:"webpay_api_key"`
	OneclickMallCode   *string `json:"oneclick_mall_code"`
	OneclickStoreCode  *string `json:"oneclick_store_code"`
	OneclickAPIKey     *string `json:"oneclick_api_key"`
	FintocSecretKey    *string `json:"fintoc_secret_key"`
}

type credentialWrite struct {
	column string
	plain  string
}

func (s *Service) PaymentCredentialStatus(ctx context.Context, companyID int64) (PaymentCredentialStatus, error) {
	flags, err := s.Repo.PaymentCredentialFlags(ctx, companyID)
	if err != nil {
		return PaymentCredentialStatus{}, err
	}
	return PaymentCredentialStatus{
		Khipu:    flags.Khipu,
		Webpay:   flags.Webpay,
		Oneclick: flags.Oneclick,
		Fintoc:   flags.Fintoc,
	}, nil
}

func (s *Service) SavePaymentCredentials(ctx context.Context, companyID int64, in SavePaymentCredentialsInput) error {
	writes, err := planCredentialWrites(in)
	if err != nil {
		return err
	}
	if len(writes) == 0 {
		return &PaymentCredentialError{msg: "no hay credenciales para guardar"}
	}
	if len(s.PaymentKey) != 32 {
		return ErrPaymentCredentialKey
	}
	fields := make(map[string]*string, len(writes))
	for _, write := range writes {
		if write.plain == "" {
			fields[write.column] = nil
			continue
		}
		buf := []byte(write.plain)
		sealed, err := secretbox.Seal(s.PaymentKey, buf, credentialAAD(companyID, write.column))
		if err != nil {
			return ErrPaymentCredentialKey
		}
		fields[write.column] = &sealed
	}
	return s.Repo.SavePaymentCredentialFields(ctx, companyID, fields)
}

func credentialAAD(companyID int64, column string) []byte {
	return []byte(fmt.Sprintf("%d|%s", companyID, column))
}

func planCredentialWrites(in SavePaymentCredentialsInput) ([]credentialWrite, error) {
	var writes []credentialWrite
	if in.KhipuAPIKey != nil {
		value, err := secretOrClear(*in.KhipuAPIKey, "la API key de Khipu")
		if err != nil {
			return nil, err
		}
		writes = append(writes, credentialWrite{column: "khipu_api_key", plain: value})
	}
	if in.FintocSecretKey != nil {
		value, err := secretOrClear(*in.FintocSecretKey, "la secret key de Fintoc")
		if err != nil {
			return nil, err
		}
		writes = append(writes, credentialWrite{column: "fintoc_secret_key", plain: value})
	}
	pair, err := pairedSecrets(
		[]*string{in.WebpayCommerceCode, in.WebpayAPIKey},
		[]string{"webpay_commerce_code", "webpay_api_key"},
		[]string{"el código de comercio de Webpay", "la llave de Webpay"},
		"Webpay",
		true,
	)
	if err != nil {
		return nil, err
	}
	writes = append(writes, pair...)
	trio, err := pairedSecrets(
		[]*string{in.OneclickMallCode, in.OneclickStoreCode, in.OneclickAPIKey},
		[]string{"oneclick_mall_code", "oneclick_store_code", "oneclick_api_key"},
		[]string{"el código mall de Oneclick", "el código tienda de Oneclick", "la llave de Oneclick"},
		"Oneclick",
		false,
	)
	if err != nil {
		return nil, err
	}
	return append(writes, trio...), nil
}

func pairedSecrets(values []*string, columns, labels []string, name string, firstIsCode bool) ([]credentialWrite, error) {
	present := 0
	for _, value := range values {
		if value != nil {
			present++
		}
	}
	if present == 0 {
		return nil, nil
	}
	if present != len(values) {
		return nil, &PaymentCredentialError{msg: "completa todos los datos de " + name + " o déjalos vacíos para quitarlos"}
	}
	writes := make([]credentialWrite, 0, len(values))
	empty := 0
	for i, value := range values {
		trimmed := strings.TrimSpace(*value)
		if trimmed == "" {
			empty++
			writes = append(writes, credentialWrite{column: columns[i]})
			continue
		}
		var err error
		if firstIsCode && i == 0 || !firstIsCode && i < len(values)-1 {
			err = validateCode(trimmed, labels[i])
		} else {
			_, err = secretOrClear(trimmed, labels[i])
		}
		if err != nil {
			return nil, err
		}
		writes = append(writes, credentialWrite{column: columns[i], plain: trimmed})
	}
	if empty != 0 && empty != len(values) {
		return nil, &PaymentCredentialError{msg: "completa todos los datos de " + name + " o déjalos vacíos para quitarlos"}
	}
	if empty == len(values) {
		for i := range writes {
			writes[i].plain = ""
		}
	}
	return writes, nil
}

func secretOrClear(raw, label string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if err := validateSecret(value, label); err != nil {
		return "", err
	}
	return value, nil
}

func validateSecret(value, label string) error {
	if len(value) < 8 || len(value) > 512 {
		return &PaymentCredentialError{msg: label + " debe tener entre 8 y 512 caracteres"}
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7e {
			return &PaymentCredentialError{msg: label + " solo puede usar letras, números y símbolos, sin espacios"}
		}
	}
	return nil
}

func validateCode(value, label string) error {
	if len(value) < 4 || len(value) > 32 {
		return &PaymentCredentialError{msg: label + " debe tener entre 4 y 32 caracteres"}
	}
	for _, r := range value {
		if !unicode.IsDigit(r) && !unicode.IsLetter(r) && r != '-' && r != '_' {
			return &PaymentCredentialError{msg: label + " solo puede usar letras, números, guion y guion bajo"}
		}
	}
	return nil
}
