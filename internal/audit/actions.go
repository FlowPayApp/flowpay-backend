package audit

const (
	ActionChargeCreated    = "charge_created"
	ActionChargeUpdated    = "charge_updated"
	ActionChargeMarkedPaid = "charge_marked_paid"
	ActionChargeReopened   = "charge_reopened"
	ActionChargeDeleted    = "charge_deleted"

	ActionClientsExported = "clients_exported"

	ActionCredentialChangeRequested = "credential_change_requested"
	ActionCredentialChangeConfirmed = "credential_change_confirmed"
	ActionCredentialLinkRejected    = "credential_link_rejected"
	ActionTransferGatewayChanged    = "transfer_gateway_changed"
	ActionMailboxChanged            = "mailbox_changed"
	ActionWhatsAppNumberChanged     = "whatsapp_number_changed"

	ActionMessagingSettingsChanged  = "messaging_settings_changed"
	ActionReminderSent              = "reminder_sent"
	ActionReminderFailed            = "reminder_failed"
	ActionWhatsAppSignatureRejected = "whatsapp_signature_rejected"
	ActionWhatsAppOutbound          = "whatsapp_outbound"
	ActionAttachmentUploaded        = "attachment_uploaded"
)
