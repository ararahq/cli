package mcp

import (
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

const (
	toolListTemplates     = "arara_list_templates"
	toolGetTemplateStatus = "arara_get_template_status"
	toolGetMessageStatus  = "arara_get_message_status"
	toolListContacts      = "arara_list_contacts"
	toolGetContact        = "arara_get_contact"
	toolGetContactStats   = "arara_get_contact_stats"
	toolGetMetrics        = "arara_get_metrics"
	toolGetWalletBalance  = "arara_get_wallet_balance"
	toolListNumbers       = "arara_list_numbers"
	toolEstimateCampaign  = "arara_estimate_campaign"

	toolSendMessage    = "arara_send_message"
	toolCreateCampaign = "arara_create_campaign"
	toolImportContacts = "arara_import_contacts"

	argMessageID      = "messageId"
	argTemplateID     = "templateId"
	argPhone          = "phone"
	argMode           = "mode"
	argPage           = "page"
	argSize           = "size"
	argQuery          = "query"
	argTemplateName   = "templateName"
	argRecipientCount = "recipientCount"
	argTo             = "to"
	argBody           = "body"
	argVariables      = "variables"
	argScheduledAt    = "scheduledAt"
	argDryRun         = "dryRun"
	argIdempotencyKey = "idempotencyKey"
	argCampaignName   = "campaignName"
	argContacts       = "contacts"

	defaultPage     = 0
	defaultPageSize = 20
	modeLive        = "live"
	modeTest        = "test"
)

func (server *Server) registerReadTools() {
	server.registerTool(
		mcpsdk.NewTool(
			toolListTemplates,
			mcpsdk.WithDescription("List all WhatsApp templates available to the account, including category, language, and approval status."),
			mcpsdk.WithReadOnlyHintAnnotation(true),
			mcpsdk.WithDestructiveHintAnnotation(false),
			mcpsdk.WithIdempotentHintAnnotation(true),
		),
		server.handleListTemplates,
	)

	server.registerTool(
		mcpsdk.NewTool(
			toolGetTemplateStatus,
			mcpsdk.WithDescription("Get the approval status of a specific WhatsApp template by ID."),
			mcpsdk.WithString(argTemplateID,
				mcpsdk.Required(),
				mcpsdk.Description("Template ID returned by arara_list_templates."),
			),
			mcpsdk.WithReadOnlyHintAnnotation(true),
			mcpsdk.WithDestructiveHintAnnotation(false),
			mcpsdk.WithIdempotentHintAnnotation(true),
		),
		server.handleGetTemplateStatus,
	)

	server.registerTool(
		mcpsdk.NewTool(
			toolGetMessageStatus,
			mcpsdk.WithDescription("Look up the delivery status of a previously sent WhatsApp message by ID."),
			mcpsdk.WithString(argMessageID,
				mcpsdk.Required(),
				mcpsdk.Description("Message ID returned by arara_send_message or visible in arara logs."),
			),
			mcpsdk.WithReadOnlyHintAnnotation(true),
			mcpsdk.WithDestructiveHintAnnotation(false),
			mcpsdk.WithIdempotentHintAnnotation(true),
		),
		server.handleGetMessageStatus,
	)

	server.registerTool(
		mcpsdk.NewTool(
			toolListContacts,
			mcpsdk.WithDescription("List contacts, paginated. Optionally filter by a substring query against name/phone/email."),
			mcpsdk.WithString(argQuery, mcpsdk.Description("Substring filter against name/phone/email (optional).")),
			mcpsdk.WithNumber(argPage, mcpsdk.Description("Zero-indexed page number (default 0).")),
			mcpsdk.WithNumber(argSize, mcpsdk.Description("Page size, max 100 (default 20).")),
			mcpsdk.WithReadOnlyHintAnnotation(true),
			mcpsdk.WithDestructiveHintAnnotation(false),
			mcpsdk.WithIdempotentHintAnnotation(true),
		),
		server.handleListContacts,
	)

	server.registerTool(
		mcpsdk.NewTool(
			toolGetContact,
			mcpsdk.WithDescription("Fetch a single contact by phone number."),
			mcpsdk.WithString(argPhone,
				mcpsdk.Required(),
				mcpsdk.Description("Phone number in E.164 format, e.g. +5511999999999."),
			),
			mcpsdk.WithReadOnlyHintAnnotation(true),
			mcpsdk.WithDestructiveHintAnnotation(false),
			mcpsdk.WithIdempotentHintAnnotation(true),
		),
		server.handleGetContact,
	)

	server.registerTool(
		mcpsdk.NewTool(
			toolGetContactStats,
			mcpsdk.WithDescription("Aggregate stats over the contact list: totals, opt-ins, recent activity."),
			mcpsdk.WithReadOnlyHintAnnotation(true),
			mcpsdk.WithDestructiveHintAnnotation(false),
			mcpsdk.WithIdempotentHintAnnotation(true),
		),
		server.handleGetContactStats,
	)

	server.registerTool(
		mcpsdk.NewTool(
			toolGetMetrics,
			mcpsdk.WithDescription("Dashboard metrics: send volume, delivery rate, recent activity. Filtered by mode."),
			mcpsdk.WithString(argMode,
				mcpsdk.Description("Mode: 'live' or 'test' (defaults to the CLI's active mode)."),
				mcpsdk.WithStringEnumItems([]string{modeLive, modeTest}),
			),
			mcpsdk.WithReadOnlyHintAnnotation(true),
			mcpsdk.WithDestructiveHintAnnotation(false),
			mcpsdk.WithIdempotentHintAnnotation(true),
		),
		server.handleGetMetrics,
	)

	server.registerTool(
		mcpsdk.NewTool(
			toolGetWalletBalance,
			mcpsdk.WithDescription("Current wallet balance and recent burn rate for the active account."),
			mcpsdk.WithString(argMode,
				mcpsdk.Description("Mode: 'live' or 'test' (defaults to the CLI's active mode)."),
				mcpsdk.WithStringEnumItems([]string{modeLive, modeTest}),
			),
			mcpsdk.WithReadOnlyHintAnnotation(true),
			mcpsdk.WithDestructiveHintAnnotation(false),
			mcpsdk.WithIdempotentHintAnnotation(true),
		),
		server.handleGetWalletBalance,
	)

	server.registerTool(
		mcpsdk.NewTool(
			toolListNumbers,
			mcpsdk.WithDescription("List WhatsApp phone numbers registered to the account."),
			mcpsdk.WithReadOnlyHintAnnotation(true),
			mcpsdk.WithDestructiveHintAnnotation(false),
			mcpsdk.WithIdempotentHintAnnotation(true),
		),
		server.handleListNumbers,
	)

	server.registerTool(
		mcpsdk.NewTool(
			toolEstimateCampaign,
			mcpsdk.WithDescription("Estimate the cost of sending a template to N recipients before launching a campaign."),
			mcpsdk.WithString(argTemplateName,
				mcpsdk.Required(),
				mcpsdk.Description("Template name (not ID) as registered in WhatsApp Business."),
			),
			mcpsdk.WithNumber(argRecipientCount,
				mcpsdk.Required(),
				mcpsdk.Description("Number of recipients (positive integer)."),
			),
			mcpsdk.WithReadOnlyHintAnnotation(true),
			mcpsdk.WithDestructiveHintAnnotation(false),
			mcpsdk.WithIdempotentHintAnnotation(true),
		),
		server.handleEstimateCampaign,
	)
}

func (server *Server) registerWriteTools() {
	server.registerTool(
		mcpsdk.NewTool(
			toolSendMessage,
			mcpsdk.WithDescription("Send a WhatsApp message. Use dryRun=true to preview the payload + cost without sending."),
			mcpsdk.WithString(argTo,
				mcpsdk.Required(),
				mcpsdk.Description("Recipient phone in E.164 (+5511999999999) or 'whatsapp:+E.164'."),
			),
			mcpsdk.WithString(argTemplateName, mcpsdk.Description("Template name. Mutually exclusive with body.")),
			mcpsdk.WithString(argBody, mcpsdk.Description("Freeform message body. Mutually exclusive with template.")),
			mcpsdk.WithArray(argVariables,
				mcpsdk.Description("Template variables in declaration order."),
				mcpsdk.WithStringItems(),
			),
			mcpsdk.WithString(argScheduledAt, mcpsdk.Description("ISO 8601 timestamp for scheduled delivery (optional).")),
			mcpsdk.WithBoolean(argDryRun, mcpsdk.Description("If true, return payload + estimated cost without sending.")),
			mcpsdk.WithString(argIdempotencyKey, mcpsdk.Description("Client-supplied idempotency key; auto-generated if omitted.")),
			mcpsdk.WithReadOnlyHintAnnotation(false),
			mcpsdk.WithDestructiveHintAnnotation(true),
			mcpsdk.WithIdempotentHintAnnotation(true),
		),
		server.handleSendMessage,
	)

	server.registerTool(
		mcpsdk.NewTool(
			toolCreateCampaign,
			mcpsdk.WithDescription("Create a campaign that sends a template to a list of contacts. Returns a campaign ID."),
			mcpsdk.WithString(argCampaignName,
				mcpsdk.Required(),
				mcpsdk.Description("Human-readable campaign name."),
			),
			mcpsdk.WithString(argTemplateName,
				mcpsdk.Required(),
				mcpsdk.Description("Approved template name."),
			),
			mcpsdk.WithArray(argContacts,
				mcpsdk.Required(),
				mcpsdk.Description("Array of {to, variables} objects."),
			),
			mcpsdk.WithString(argIdempotencyKey, mcpsdk.Description("Client-supplied idempotency key; auto-generated if omitted.")),
			mcpsdk.WithReadOnlyHintAnnotation(false),
			mcpsdk.WithDestructiveHintAnnotation(true),
			mcpsdk.WithIdempotentHintAnnotation(true),
		),
		server.handleCreateCampaign,
	)

	server.registerTool(
		mcpsdk.NewTool(
			toolImportContacts,
			mcpsdk.WithDescription("Bulk-import contacts. Returns an import ID for tracking."),
			mcpsdk.WithArray(argContacts,
				mcpsdk.Required(),
				mcpsdk.Description("Array of {name, phone, email?, attributes?} objects."),
			),
			mcpsdk.WithReadOnlyHintAnnotation(false),
			mcpsdk.WithDestructiveHintAnnotation(false),
			mcpsdk.WithIdempotentHintAnnotation(true),
		),
		server.handleImportContacts,
	)
}
