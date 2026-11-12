package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ranajahanzaib/usage-based-billing/internal/billing"
)

func TestPreviewEndpoint(t *testing.T) {
	rr := httptest.NewRecorder()
	newHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/preview", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var p billing.Preview
	if e := json.Unmarshal(rr.Body.Bytes(), &p); e != nil {
		t.Fatal(e)
	}
	if p.Total <= 375 {
		t.Fatalf("expected base fee plus counter overage, got %s", p.Total.String())
	}
	var sum billing.Money
	for _, line := range p.Lines {
		sum += line.Amount
	}
	if sum != p.Total {
		t.Fatalf("line sum %d differs from total %d", sum, p.Total)
	}
}
func TestHealthEndpoint(t *testing.T) {
	rr := httptest.NewRecorder()
	newHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
}

func TestDemoListsThreeSeededCustomersAndPreviewsEach(t *testing.T) {
	h := newHandler()
	list := httptest.NewRecorder()
	h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/customers", nil))
	var customers []map[string]string
	if err := json.Unmarshal(list.Body.Bytes(), &customers); err != nil {
		t.Fatal(err)
	}
	if len(customers) != 3 {
		t.Fatalf("expected three demo customers, got %d", len(customers))
	}
	for _, id := range []string{"startup", "growth", "scale"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/preview?customer_id="+id, nil))
		if rr.Code != http.StatusOK {
			t.Errorf("customer %s preview status %d: %s", id, rr.Code, rr.Body.String())
		}
	}
}

func TestSimulationRecalculatesWithoutPersisting(t *testing.T) {
	h := newHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/simulate", strings.NewReader(`{"customer_id":"startup","metric":"api_requests","quantity":25000}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("simulation status %d: %s", rr.Code, rr.Body.String())
	}
	var simulated billing.Preview
	if err := json.Unmarshal(rr.Body.Bytes(), &simulated); err != nil {
		t.Fatal(err)
	}
	if len(simulated.Usage) == 0 || simulated.Usage[0].Recorded != "37500" || simulated.Usage[0].Overage != "27500" {
		t.Fatalf("unexpected simulated usage summary: %#v", simulated.Usage)
	}
	preview := httptest.NewRecorder()
	h.ServeHTTP(preview, httptest.NewRequest(http.MethodGet, "/api/preview?customer_id=startup", nil))
	var actual billing.Preview
	if err := json.Unmarshal(preview.Body.Bytes(), &actual); err != nil {
		t.Fatal(err)
	}
	if len(actual.Usage) == 0 || actual.Usage[0].Recorded != "12500" {
		t.Fatalf("stateless simulation changed seeded preview: %#v", actual.Usage)
	}
}

func TestSimulationRejectsOversizedAndUnknownRequests(t *testing.T) {
	for _, body := range []string{`{"customer_id":"startup","metric":"api_requests","quantity":10000001}`, `{"customer_id":"startup","metric":"storage_bytes","quantity":10}`, `{"customer_id":"startup","metric":"api_requests","quantity":null}`} {
		rr := httptest.NewRecorder()
		newHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/simulate", strings.NewReader(body)))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("request %s returned %d", body, rr.Code)
		}
	}
}
