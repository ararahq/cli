package api

import (
	"fmt"
	"net/http"
)

const (
	campaignsBasePath    = "/v1/campaigns"
	campaignEstimatePath = "/v1/campaigns/estimate"
)

type CampaignContact struct {
	To        string   `json:"to"`
	Variables []string `json:"variables,omitempty"`
}

type CreateCampaignRequest struct {
	Name         string            `json:"name"`
	TemplateName string            `json:"templateName"`
	Contacts     []CampaignContact `json:"contacts"`
}

// CampaignResponse is the unified shape used for create/list/get. Fields
// are a superset of what the three endpoints actually populate:
//
//   - POST /v1/campaigns returns the small CampaignResponse DTO (id, name,
//     status, totalMessages, totalCost) — extra fields stay zero.
//   - GET  /v1/campaigns returns the rich CampaignListItem (adds
//     templateName, sentCount, createdAt) inside a Spring page wrapper —
//     see ListCampaigns for the unwrap step.
//   - GET  /v1/campaigns/{id} returns CampaignDetailResponse with the full
//     funnel (delivered/read/clicked/converted) plus timestamps.
//
// Keeping a single Go struct avoids forcing every caller to choose between
// three near-identical types. Callers should treat any field they don't
// expect for their endpoint as "zero, ignore".
type CampaignResponse struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Status         string  `json:"status"`
	TemplateName   string  `json:"templateName,omitempty"`
	TemplateBody   string  `json:"templateBody,omitempty"`
	TotalMessages  int     `json:"totalMessages"`
	SentCount      int     `json:"sentCount,omitempty"`
	DeliveredCount int     `json:"deliveredCount,omitempty"`
	ReadCount      int     `json:"readCount,omitempty"`
	ClickedCount   int     `json:"clickedCount,omitempty"`
	ConvertedCount int     `json:"convertedCount,omitempty"`
	ConvertedValue float64 `json:"convertedValue,omitempty"`
	TotalCost      float64 `json:"totalCost"`
	ScheduledAt    string  `json:"scheduledAt,omitempty"`
	StartedAt      string  `json:"startedAt,omitempty"`
	FinishedAt     string  `json:"finishedAt,omitempty"`
	CreatedAt      string  `json:"createdAt,omitempty"`
}

// FailedCount reports how many messages of the campaign never reached the
// "sent" state. The backend doesn't expose failedCount directly — it's
// inferred from totalMessages minus sentCount. Returns 0 if the
// CampaignResponse came from an endpoint that doesn't populate sentCount
// (e.g. POST /v1/campaigns), so display code should guard with a check
// like `if campaign.SentCount > 0 || campaign.TotalMessages == 0`.
func (campaign *CampaignResponse) FailedCount() int {
	if campaign.TotalMessages <= 0 || campaign.SentCount <= 0 {
		return 0
	}
	failed := campaign.TotalMessages - campaign.SentCount
	if failed < 0 {
		return 0
	}
	return failed
}

// campaignListEnvelope mirrors Spring's Page<T> shape used by
// GET /v1/campaigns: { content: [...], totalPages, totalElements }.
// Decoding directly into []CampaignResponse failed silently (object →
// slice mismatch) — the symptom was `arara campaigns list` returning
// nothing in production.
type campaignListEnvelope struct {
	Content       []CampaignResponse `json:"content"`
	TotalPages    int                `json:"totalPages"`
	TotalElements int64              `json:"totalElements"`
}

type CampaignEstimateResponse struct {
	TemplateCost     float64 `json:"templateCost"`
	AraraFee         float64 `json:"araraFee"`
	UnitPrice        float64 `json:"unitPrice"`
	TotalCost        float64 `json:"totalCost"`
	TemplateCategory string  `json:"templateCategory"`
	RecipientCount   int     `json:"recipientCount"`
}

func (client *Client) CreateCampaign(request CreateCampaignRequest, idempotencyKey string) (*CampaignResponse, error) {
	if request.Name == "" {
		return nil, fmt.Errorf("campaign name cannot be empty")
	}

	if request.TemplateName == "" {
		return nil, fmt.Errorf("campaign template name cannot be empty")
	}

	if len(request.Contacts) == 0 {
		return nil, fmt.Errorf("campaign must have at least one contact")
	}

	headers := map[string]string{
		idempotencyKeyHeader: idempotencyKey,
	}

	var response CampaignResponse
	if postError := client.DoWithHeaders(http.MethodPost, campaignsBasePath, request, &response, headers); postError != nil {
		return nil, fmt.Errorf("failed to create campaign: %w", postError)
	}

	return &response, nil
}

func (client *Client) ListCampaigns() ([]CampaignResponse, error) {
	var envelope campaignListEnvelope
	if getError := client.Get(campaignsBasePath, &envelope); getError != nil {
		return nil, fmt.Errorf("failed to list campaigns: %w", getError)
	}

	return envelope.Content, nil
}

func (client *Client) GetCampaign(campaignID string) (*CampaignResponse, error) {
	if campaignID == "" {
		return nil, fmt.Errorf("campaign ID cannot be empty")
	}

	path := fmt.Sprintf("%s/%s", campaignsBasePath, campaignID)

	var response CampaignResponse
	if getError := client.Get(path, &response); getError != nil {
		return nil, fmt.Errorf("failed to get campaign: %w", getError)
	}

	return &response, nil
}

func (client *Client) EstimateCampaign(templateName string, recipientCount int) (*CampaignEstimateResponse, error) {
	if templateName == "" {
		return nil, fmt.Errorf("template name cannot be empty for campaign estimate")
	}

	if recipientCount <= 0 {
		return nil, fmt.Errorf("recipient count must be greater than zero")
	}

	path := fmt.Sprintf("%s?templateName=%s&count=%d", campaignEstimatePath, templateName, recipientCount)

	var response CampaignEstimateResponse
	if getError := client.Get(path, &response); getError != nil {
		return nil, fmt.Errorf("failed to estimate campaign cost: %w", getError)
	}

	return &response, nil
}
