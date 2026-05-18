package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubRoute struct {
	method string
	path   string
	status int
	body   string
}

func newStubServer(t *testing.T, routes []stubRoute) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()

		for _, route := range routes {
			if r.Method != route.method {
				continue
			}
			if !pathMatches(r.URL.Path, r.URL.RawQuery, route.path) {
				continue
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(route.status)
			if route.body != "" {
				_, _ = w.Write([]byte(route.body))
			}
			return
		}
		t.Errorf("no stub matched %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	return server
}

func pathMatches(path, query, want string) bool {
	if want == path {
		return true
	}
	if query == "" {
		return false
	}
	return strings.HasPrefix(want, path+"?") && strings.Contains(want, query)
}

func TestWrappers_HappyPaths(t *testing.T) {
	routes := []stubRoute{
		{http.MethodGet, "/v1/templates", http.StatusOK, `[{"id":"t1","name":"hello","language":"pt_BR","body":"oi"}]`},
		{http.MethodGet, "/v1/templates/t1/status", http.StatusOK, `{"status":"APPROVED","category":"MARKETING"}`},
		{http.MethodDelete, "/v1/templates/t1", http.StatusNoContent, ""},

		{http.MethodPost, "/v1/messages", http.StatusCreated, `{"id":"m1","status":"queued"}`},
		{http.MethodGet, "/v1/messages/m1", http.StatusOK, `{"id":"m1","status":"delivered"}`},

		{http.MethodGet, "/v1/contacts?page=0&size=20", http.StatusOK, `{"contacts":[],"total":0,"page":0,"size":20}`},
		{http.MethodGet, "/v1/contacts/+5511", http.StatusOK, `{"id":"c1","phone":"+5511"}`},
		{http.MethodPost, "/v1/contacts/batch", http.StatusAccepted, `{"importId":"imp_1"}`},
		{http.MethodGet, "/v1/contacts/stats", http.StatusOK, `{"total":42}`},

		{http.MethodGet, "/v1/api-keys", http.StatusOK, `[{"id":"k1","prefix":"ara_test_","lastFour":"abcd","mode":"test"}]`},
		{http.MethodPost, "/v1/api-keys?mode=test", http.StatusCreated, `{"plainTextKey":"ara_test_xxxxxxxxxx"}`},
		{http.MethodDelete, "/v1/api-keys/k1", http.StatusNoContent, ""},

		{http.MethodGet, "/organizations/me/numbers", http.StatusOK, `[{"id":"n1","phoneNumber":"+5511"}]`},

		{http.MethodPost, "/v1/campaigns", http.StatusCreated, `{"id":"camp1","status":"queued","totalMessages":1}`},
		{http.MethodGet, "/v1/campaigns", http.StatusOK, `{"content":[{"id":"camp1","status":"queued","totalMessages":1}],"totalPages":1,"totalElements":1}`},
		{http.MethodGet, "/v1/campaigns/camp1", http.StatusOK, `{"id":"camp1","status":"running","totalMessages":1}`},
		{http.MethodGet, "/v1/campaigns/estimate?templateName=hello&count=10", http.StatusOK, `{"totalCost":1.5,"recipientCount":10}`},

		{http.MethodGet, "/dashboard/metrics?mode=live", http.StatusOK, `{"sent":10}`},
		{http.MethodGet, "/dashboard/wallet/balance?mode=live", http.StatusOK, `{"balance":500}`},
		{http.MethodGet, "/dashboard/messages?mode=live&page=0&size=20", http.StatusOK, `{"data":[]}`},
	}

	server := newStubServer(t, routes)
	client := newTestClient(server.URL)

	if templates, err := client.ListTemplates(); err != nil || len(templates) != 1 {
		t.Errorf("ListTemplates failed: err=%v len=%d", err, len(templates))
	}

	if status, err := client.GetTemplateStatus("t1"); err != nil || status.Status != "APPROVED" {
		t.Errorf("GetTemplateStatus: err=%v status=%+v", err, status)
	}
	if _, err := client.GetTemplateStatus(""); err == nil {
		t.Error("GetTemplateStatus must reject empty id")
	}

	if err := client.DeleteTemplate("t1"); err != nil {
		t.Errorf("DeleteTemplate: %v", err)
	}
	if err := client.DeleteTemplate(""); err == nil {
		t.Error("DeleteTemplate must reject empty id")
	}

	msg, err := client.SendMessage(SendMessageRequest{Receiver: "+5511", TemplateName: "hello"})
	if err != nil || msg.ID != "m1" {
		t.Errorf("SendMessage: err=%v msg=%+v", err, msg)
	}
	if _, err := client.GetMessageStatus("m1"); err != nil {
		t.Errorf("GetMessageStatus: %v", err)
	}
	if _, err := client.GetMessageStatus(""); err == nil {
		t.Error("GetMessageStatus must reject empty id")
	}

	if _, err := client.ListContacts("", -1, 0); err != nil {
		t.Errorf("ListContacts: %v", err)
	}
	if _, err := client.GetContact("+5511"); err != nil {
		t.Errorf("GetContact: %v", err)
	}
	if _, err := client.GetContact(""); err == nil {
		t.Error("GetContact must reject empty phone")
	}
	if importID, err := client.ImportContacts([]ContactRequest{{Phone: "+1"}}); err != nil || importID != "imp_1" {
		t.Errorf("ImportContacts: err=%v importID=%q", err, importID)
	}
	if _, err := client.ImportContacts(nil); err == nil {
		t.Error("ImportContacts must reject empty list")
	}
	if _, err := client.GetContactStats(); err != nil {
		t.Errorf("GetContactStats: %v", err)
	}

	if _, err := client.ListAPIKeys(); err != nil {
		t.Errorf("ListAPIKeys: %v", err)
	}
	if _, err := client.CreateAPIKey("test"); err != nil {
		t.Errorf("CreateAPIKey: %v", err)
	}
	if _, err := client.CreateAPIKey(""); err == nil {
		t.Error("CreateAPIKey must reject empty mode")
	}
	if err := client.RevokeAPIKey("k1"); err != nil {
		t.Errorf("RevokeAPIKey: %v", err)
	}
	if err := client.RevokeAPIKey(""); err == nil {
		t.Error("RevokeAPIKey must reject empty id")
	}

	if _, err := client.ListNumbers(); err != nil {
		t.Errorf("ListNumbers: %v", err)
	}

	camp, err := client.CreateCampaign(CreateCampaignRequest{Name: "n", TemplateName: "hello", Contacts: []CampaignContact{{To: "+1"}}}, "")
	if err != nil || camp.ID != "camp1" {
		t.Errorf("CreateCampaign: err=%v camp=%+v", err, camp)
	}
	if _, err := client.CreateCampaign(CreateCampaignRequest{}, ""); err == nil {
		t.Error("CreateCampaign must reject empty name")
	}
	if _, err := client.CreateCampaign(CreateCampaignRequest{Name: "n"}, ""); err == nil {
		t.Error("CreateCampaign must reject empty template")
	}
	if _, err := client.CreateCampaign(CreateCampaignRequest{Name: "n", TemplateName: "t"}, ""); err == nil {
		t.Error("CreateCampaign must reject empty contacts")
	}

	if _, err := client.ListCampaigns(); err != nil {
		t.Errorf("ListCampaigns: %v", err)
	}
	if _, err := client.GetCampaign("camp1"); err != nil {
		t.Errorf("GetCampaign: %v", err)
	}
	if _, err := client.GetCampaign(""); err == nil {
		t.Error("GetCampaign must reject empty id")
	}
	if _, err := client.EstimateCampaign("hello", 10); err != nil {
		t.Errorf("EstimateCampaign: %v", err)
	}
	if _, err := client.EstimateCampaign("", 1); err == nil {
		t.Error("EstimateCampaign must reject empty template")
	}
	if _, err := client.EstimateCampaign("hello", 0); err == nil {
		t.Error("EstimateCampaign must reject zero recipients")
	}

	if _, err := client.GetMetrics("live"); err != nil {
		t.Errorf("GetMetrics: %v", err)
	}
	if _, err := client.GetWalletBalance("live"); err != nil {
		t.Errorf("GetWalletBalance: %v", err)
	}
	if _, err := client.GetMessages("live", 0, 20); err != nil {
		t.Errorf("GetMessages: %v", err)
	}
}

func TestWrappers_PropagateAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"bad key"}}`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)

	_, err := client.ListTemplates()
	if !IsAuthError(err) {
		t.Errorf("expected wrapped 401 to be detectable as auth error, got: %v", err)
	}
}

func TestWrappers_ServerErrorBubblesUp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"down","message":"upstream"}}`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)

	checks := []struct {
		name string
		run  func() error
	}{
		{"ListTemplates", func() error { _, err := client.ListTemplates(); return err }},
		{"GetTemplateStatus", func() error { _, err := client.GetTemplateStatus("t1"); return err }},
		{"DeleteTemplate", func() error { return client.DeleteTemplate("t1") }},
		{"SendMessage", func() error {
			_, err := client.SendMessage(SendMessageRequest{Receiver: "+5511", Body: "x"})
			return err
		}},
		{"GetMessageStatus", func() error { _, err := client.GetMessageStatus("m1"); return err }},
		{"ListContacts", func() error { _, err := client.ListContacts("", 0, 20); return err }},
		{"GetContact", func() error { _, err := client.GetContact("+1"); return err }},
		{"ImportContacts", func() error {
			_, err := client.ImportContacts([]ContactRequest{{Phone: "+1"}})
			return err
		}},
		{"GetContactStats", func() error { _, err := client.GetContactStats(); return err }},
		{"ListAPIKeys", func() error { _, err := client.ListAPIKeys(); return err }},
		{"CreateAPIKey", func() error { _, err := client.CreateAPIKey("test"); return err }},
		{"RevokeAPIKey", func() error { return client.RevokeAPIKey("k1") }},
		{"ListNumbers", func() error { _, err := client.ListNumbers(); return err }},
		{"CreateCampaign", func() error {
			_, err := client.CreateCampaign(CreateCampaignRequest{
				Name: "n", TemplateName: "t",
				Contacts: []CampaignContact{{To: "+1"}},
			}, "key")
			return err
		}},
		{"ListCampaigns", func() error { _, err := client.ListCampaigns(); return err }},
		{"GetCampaign", func() error { _, err := client.GetCampaign("camp1"); return err }},
		{"EstimateCampaign", func() error { _, err := client.EstimateCampaign("hello", 5); return err }},
		{"GetMetrics", func() error { _, err := client.GetMetrics("live"); return err }},
		{"GetWalletBalance", func() error { _, err := client.GetWalletBalance("live"); return err }},
		{"GetMessages", func() error { _, err := client.GetMessages("live", 0, 20); return err }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.run(); err == nil {
				t.Errorf("%s: expected error, got nil", check.name)
			}
		})
	}
}

func TestImportContacts_SerializesPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		var contacts []ContactRequest
		if err := json.Unmarshal(bodyBytes, &contacts); err != nil {
			t.Errorf("server could not parse body: %v (raw=%s)", err, string(bodyBytes))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(contacts) != 2 || contacts[0].Phone != "+1" {
			t.Errorf("unexpected payload: %+v", contacts)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"importId":"imp_zz"}`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	got, err := client.ImportContacts([]ContactRequest{{Phone: "+1"}, {Phone: "+2"}})
	if err != nil || got != "imp_zz" {
		t.Errorf("ImportContacts: err=%v got=%q", err, got)
	}
}
