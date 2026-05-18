package cmd

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/output"
)

func TestColorizeCampaignStatus(t *testing.T) {
	cases := []struct {
		input    string
		contains string
	}{
		{"COMPLETED", "COMPLETED"},
		{"completed", "COMPLETED"},
		{"PROCESSING", "PROCESSING"},
		{"PENDING", "PENDING"},
		{"FAILED", "FAILED"},
		{"CANCELLED", "CANCELLED"},
		{"weird", "WEIRD"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := colorizeCampaignStatus(tc.input); !strings.Contains(got, tc.contains) {
				t.Errorf("colorize(%q) should contain %q, got %q", tc.input, tc.contains, got)
			}
		})
	}
}

func TestValidateCampaignCreateFlags_RequiresAllFields(t *testing.T) {
	originalName := campaignCreateNameFlag
	originalTemplate := campaignCreateTemplateFlag
	originalContacts := campaignCreateContactsFlag
	t.Cleanup(func() {
		campaignCreateNameFlag = originalName
		campaignCreateTemplateFlag = originalTemplate
		campaignCreateContactsFlag = originalContacts
	})

	cases := []struct {
		name     string
		nameFlag string
		template string
		contacts string
		wantErr  string
	}{
		{"missing name", "", "tmpl", "[]", "--name"},
		{"missing template", "promo", "", "[]", "--template"},
		{"missing contacts", "promo", "tmpl", "", "--contacts"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			campaignCreateNameFlag = tc.nameFlag
			campaignCreateTemplateFlag = tc.template
			campaignCreateContactsFlag = tc.contacts

			err := validateCampaignCreateFlags()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error should mention %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateCampaignCreateFlags_AllPresent(t *testing.T) {
	originalName := campaignCreateNameFlag
	originalTemplate := campaignCreateTemplateFlag
	originalContacts := campaignCreateContactsFlag
	t.Cleanup(func() {
		campaignCreateNameFlag = originalName
		campaignCreateTemplateFlag = originalTemplate
		campaignCreateContactsFlag = originalContacts
	})

	campaignCreateNameFlag = "ok"
	campaignCreateTemplateFlag = "tmpl"
	campaignCreateContactsFlag = "[]"
	if err := validateCampaignCreateFlags(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestParseCampaignContacts_FromInlineJSON(t *testing.T) {
	contacts, err := parseCampaignContacts(`[{"to":"+5511999999999","variables":["alice"]}]`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(contacts) != 1 {
		t.Errorf("expected 1 contact, got %d", len(contacts))
	}
	if contacts[0].To != "+5511999999999" {
		t.Errorf("to: %q", contacts[0].To)
	}
}

func TestParseCampaignContacts_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "contacts.json")
	body := `[{"to":"+551199","variables":["a"]},{"to":"+551188","variables":["b"]}]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	contacts, err := parseCampaignContacts("@" + path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(contacts) != 2 {
		t.Errorf("expected 2 contacts, got %d", len(contacts))
	}
}

func TestParseCampaignContacts_FileNotFound(t *testing.T) {
	_, err := parseCampaignContacts("@/nonexistent/path/contacts.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseCampaignContacts_InvalidJSON(t *testing.T) {
	_, err := parseCampaignContacts(`not json`)
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestParseCampaignContacts_EmptyArrayRejected(t *testing.T) {
	_, err := parseCampaignContacts(`[]`)
	if err == nil {
		t.Fatal("expected error on empty array")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention empty, got: %v", err)
	}
}

func TestCampaignsTableData_RowsAndHeaders(t *testing.T) {
	// SentCount=98 + TotalMessages=100 → FailedCount()=2 (matches the
	// "FAILED" column the table renders).
	campaigns := []api.CampaignResponse{
		{Name: "promo-q1", Status: "COMPLETED", TotalMessages: 100, SentCount: 98, TotalCost: 12.34},
		{Name: "promo-q2", Status: "PROCESSING", TotalMessages: 50, SentCount: 50, TotalCost: 5.00},
	}

	headers, rows := campaignsTableData(campaigns)

	if len(headers) != 5 {
		t.Errorf("headers: want 5, got %d", len(headers))
	}
	if len(rows) != 2 {
		t.Fatalf("rows: want 2, got %d", len(rows))
	}
	if rows[0][0] != "promo-q1" {
		t.Errorf("name: %q", rows[0][0])
	}
	if rows[0][2] != "100" {
		t.Errorf("messages: %q", rows[0][2])
	}
	if rows[0][4] != "$12.34" {
		t.Errorf("cost: %q", rows[0][4])
	}
}

func TestPrintCampaignEstimate_RendersAllFields(t *testing.T) {
	estimate := &api.CampaignEstimateResponse{
		TemplateCategory: "MARKETING",
		RecipientCount:   1000,
		TemplateCost:     0.05,
		AraraFee:         0.01,
		UnitPrice:        0.06,
		TotalCost:        60.00,
	}
	buffer := &bytes.Buffer{}

	printCampaignEstimate(buffer, estimate)

	rendered := buffer.String()
	for _, expected := range []string{"MARKETING", "1000", "$60.00", "Recipients", "Total Cost"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestPrintCampaignStatus_ShowsProgressAndFailures(t *testing.T) {
	// SentCount=50 of 200 → 25% progress, FailedCount()=200-50=150.
	// Test asserts the output mentions the progress and a non-zero
	// failure count; exact "150 failed" string isn't asserted because
	// FailedCount semantics are inferred (see CampaignResponse.FailedCount).
	campaign := &api.CampaignResponse{
		ID:            "camp_1",
		Name:          "promo",
		Status:        "PROCESSING",
		TotalMessages: 200,
		SentCount:     50,
		TotalCost:     9.99,
	}
	buffer := &bytes.Buffer{}

	printCampaignStatus(buffer, campaign)

	rendered := buffer.String()
	for _, expected := range []string{"camp_1", "promo", "Progress:", "50/200", "25.0%", "150 failed", "$9.99"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestPrintCampaignStatus_NoProgressWhenZeroMessages(t *testing.T) {
	campaign := &api.CampaignResponse{
		Name:          "queued",
		Status:        "PENDING",
		TotalMessages: 0,
	}
	buffer := &bytes.Buffer{}

	printCampaignStatus(buffer, campaign)

	if strings.Contains(buffer.String(), "Progress:") {
		t.Errorf("Progress should not appear when TotalMessages=0, got:\n%s", buffer.String())
	}
}

func TestPrintCampaignCreated_IncludesAllFields(t *testing.T) {
	campaign := &api.CampaignResponse{
		ID:            "camp_xyz",
		Name:          "launch",
		Status:        "PENDING",
		TotalMessages: 500,
		TotalCost:     25.00,
	}
	buffer := &bytes.Buffer{}

	printCampaignCreated(buffer, campaign)

	rendered := buffer.String()
	for _, expected := range []string{"camp_xyz", "launch", "PENDING", "500", "25.00"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestRunCampaignsListImpl_GoldenPath(t *testing.T) {
	// Backend wraps the list in Spring's Page<T> envelope.
	body := `{"content":[
		{"id":"c1","name":"promo-q1","status":"COMPLETED","totalMessages":100,"sentCount":98,"totalCost":12.34},
		{"id":"c2","name":"promo-q2","status":"PROCESSING","totalMessages":50,"sentCount":50,"totalCost":5.00}
	],"totalPages":1,"totalElements":2}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	buffer := &bytes.Buffer{}

	if err := runCampaignsListImpl(client, output.FormatText, buffer); err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, expected := range []string{"promo-q1", "promo-q2"} {
		if !strings.Contains(buffer.String(), expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, buffer.String())
		}
	}
}

func TestRunCampaignsListImpl_EmptyShowsHint(t *testing.T) {
	_, client := fakeAPIServerJSON(t, http.StatusOK, `{"content":[],"totalPages":0,"totalElements":0}`)
	buffer := &bytes.Buffer{}

	if err := runCampaignsListImpl(client, output.FormatText, buffer); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), "No campaigns found") {
		t.Errorf("expected hint, got: %q", buffer.String())
	}
}

func TestRunCampaignsListImpl_JSON(t *testing.T) {
	body := `{"content":[{"id":"c1","name":"x","status":"COMPLETED","totalMessages":1,"sentCount":1,"totalCost":0.10}],"totalPages":1,"totalElements":1}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	buffer := &bytes.Buffer{}

	if err := runCampaignsListImpl(client, output.FormatJSON, buffer); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(buffer.String(), `"name"`) {
		t.Errorf("JSON output missing name: %q", buffer.String())
	}
}

func TestRunCampaignsStatusImpl_GoldenPath(t *testing.T) {
	body := `{"id":"camp_1","name":"promo","status":"PROCESSING","totalMessages":200,"sentCount":50,"deliveredCount":47,"totalCost":9.99}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	buffer := &bytes.Buffer{}

	if err := runCampaignsStatusImpl(client, output.FormatText, buffer, "camp_1"); err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, expected := range []string{"camp_1", "promo", "Progress:", "50/200"} {
		if !strings.Contains(buffer.String(), expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, buffer.String())
		}
	}
}

func TestRunCampaignsEstimateImpl_GoldenPath(t *testing.T) {
	body := `{"templateCategory":"MARKETING","recipientCount":1000,"templateCost":0.05,"araraFee":0.01,"unitPrice":0.06,"totalCost":60.00}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	buffer := &bytes.Buffer{}

	if err := runCampaignsEstimateImpl(client, output.FormatText, buffer, "promo_template", 1000); err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, expected := range []string{"MARKETING", "1000", "$60.00"} {
		if !strings.Contains(buffer.String(), expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, buffer.String())
		}
	}
}

func TestRunCampaignsEstimateImpl_AuthErrorIsClassified(t *testing.T) {
	_, client := fakeAPIServerJSON(t, http.StatusUnauthorized, `{"error":{"code":"invalid_key","message":"bad"}}`)
	err := runCampaignsEstimateImpl(client, output.FormatText, &bytes.Buffer{}, "x", 1)
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !api.IsAuthError(err) {
		t.Errorf("expected auth error classification, got: %v", err)
	}
}

func TestRunCampaignsCreateImpl_GoldenPath(t *testing.T) {
	body := `{"id":"camp_new","name":"q1-promo","status":"PENDING","totalMessages":3,"totalCost":0.30}`
	_, client := fakeAPIServerJSON(t, http.StatusCreated, body)
	buffer := &bytes.Buffer{}

	request := api.CreateCampaignRequest{
		Name:         "q1-promo",
		TemplateName: "promo_template",
		Contacts: []api.CampaignContact{
			{To: "+551199", Variables: []string{"a"}},
			{To: "+551188", Variables: []string{"b"}},
			{To: "+551177", Variables: []string{"c"}},
		},
	}

	if err := runCampaignsCreateImpl(client, output.FormatText, buffer, request, "fixed-key-for-test"); err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, expected := range []string{"camp_new", "q1-promo", "PENDING"} {
		if !strings.Contains(buffer.String(), expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, buffer.String())
		}
	}
}

func TestRunCampaignsCreateImpl_ServerErrorReturnsError(t *testing.T) {
	_, client := fakeAPIServerJSON(t, http.StatusInternalServerError, `{"error":{"code":"x","message":"y"}}`)
	request := api.CreateCampaignRequest{Name: "x", TemplateName: "y", Contacts: []api.CampaignContact{{To: "+1"}}}
	if err := runCampaignsCreateImpl(client, output.FormatText, &bytes.Buffer{}, request, "k"); err == nil {
		t.Fatal("expected error from 5xx")
	}
}

func TestRunCampaignsStatusImpl_NotFoundReturnsError(t *testing.T) {
	_, client := fakeAPIServerJSON(t, http.StatusNotFound, `{"error":{"code":"not_found","message":"missing"}}`)
	if err := runCampaignsStatusImpl(client, output.FormatText, &bytes.Buffer{}, "missing"); err == nil {
		t.Fatal("expected 404 error")
	}
}

func TestRunCampaignsList_WrapperUsesInjectedClient(t *testing.T) {
	body := `{"content":[{"id":"c1","name":"x","status":"COMPLETED","totalMessages":1,"sentCount":1,"totalCost":0.10}],"totalPages":1,"totalElements":1}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	withFakeAPIClient(t, client)

	if err := runCampaignsList(nil, nil); err != nil {
		t.Errorf("runCampaignsList: %v", err)
	}
}

func TestRunCampaignsStatus_WrapperUsesInjectedClient(t *testing.T) {
	body := `{"id":"c1","name":"x","status":"COMPLETED","totalMessages":1}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	withFakeAPIClient(t, client)

	if err := runCampaignsStatus(nil, []string{"c1"}); err != nil {
		t.Errorf("runCampaignsStatus: %v", err)
	}
}

func TestRunCampaignsEstimate_RequiresTemplate(t *testing.T) {
	originalT := campaignEstimateTemplate
	originalC := campaignEstimateCount
	t.Cleanup(func() {
		campaignEstimateTemplate = originalT
		campaignEstimateCount = originalC
	})
	campaignEstimateTemplate = ""
	campaignEstimateCount = 5

	err := runCampaignsEstimate(nil, nil)
	if err == nil {
		t.Fatal("expected error when --template missing")
	}
}

func TestRunCampaignsEstimate_RequiresCount(t *testing.T) {
	originalT := campaignEstimateTemplate
	originalC := campaignEstimateCount
	t.Cleanup(func() {
		campaignEstimateTemplate = originalT
		campaignEstimateCount = originalC
	})
	campaignEstimateTemplate = "promo"
	campaignEstimateCount = 0

	err := runCampaignsEstimate(nil, nil)
	if err == nil {
		t.Fatal("expected error when --count missing")
	}
}

func TestRunCampaignsEstimate_WrapperUsesInjectedClient(t *testing.T) {
	body := `{"templateCategory":"MARKETING","recipientCount":10,"templateCost":0.05,"araraFee":0.01,"unitPrice":0.06,"totalCost":0.60}`
	_, client := fakeAPIServerJSON(t, http.StatusOK, body)
	withFakeAPIClient(t, client)

	originalT := campaignEstimateTemplate
	originalC := campaignEstimateCount
	t.Cleanup(func() {
		campaignEstimateTemplate = originalT
		campaignEstimateCount = originalC
	})
	campaignEstimateTemplate = "promo"
	campaignEstimateCount = 10

	if err := runCampaignsEstimate(nil, nil); err != nil {
		t.Errorf("runCampaignsEstimate: %v", err)
	}
}

func TestRunCampaignsCreate_RequiresFlags(t *testing.T) {
	originalName := campaignCreateNameFlag
	originalTemplate := campaignCreateTemplateFlag
	originalContacts := campaignCreateContactsFlag
	t.Cleanup(func() {
		campaignCreateNameFlag = originalName
		campaignCreateTemplateFlag = originalTemplate
		campaignCreateContactsFlag = originalContacts
	})

	campaignCreateNameFlag = ""
	campaignCreateTemplateFlag = ""
	campaignCreateContactsFlag = ""

	err := runCampaignsCreate(nil, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRunCampaignsCreate_WrapperGoldenPath(t *testing.T) {
	body := `{"id":"camp_new","name":"q1","status":"PENDING","totalMessages":1,"totalCost":0.10}`
	_, client := fakeAPIServerJSON(t, http.StatusCreated, body)
	withFakeAPIClient(t, client)

	originalName := campaignCreateNameFlag
	originalTemplate := campaignCreateTemplateFlag
	originalContacts := campaignCreateContactsFlag
	t.Cleanup(func() {
		campaignCreateNameFlag = originalName
		campaignCreateTemplateFlag = originalTemplate
		campaignCreateContactsFlag = originalContacts
	})

	campaignCreateNameFlag = "q1"
	campaignCreateTemplateFlag = "promo"
	campaignCreateContactsFlag = `[{"to":"+551199","variables":["a"]}]`

	if err := runCampaignsCreate(nil, nil); err != nil {
		t.Errorf("runCampaignsCreate: %v", err)
	}
}
