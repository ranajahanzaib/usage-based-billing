package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/ranajahanzaib/usage-based-billing/internal/billing"
)

func main() {
	if os.Getenv("DEMO_MODE") != "1" {
		slog.Error("set DEMO_MODE=1 to run the local demonstration server; it has no authentication or durable storage")
		os.Exit(2)
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	slog.Info("demo server listening", "address", addr)
	if e := http.ListenAndServe(addr, newHandler()); e != nil {
		slog.Error("server stopped", "error", e)
		os.Exit(1)
	}
}

type demoCustomer struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Plan    billing.Plan           `json:"plan"`
	Events  []billing.Event        `json:"-"`
	Storage []billing.StoragePoint `json:"-"`
}

func demoPlans() []demoCustomer {
	starter := billing.Plan{Key: "starter", MonthlyBase: "19.00", Prices: map[string]billing.MetricPrice{
		"api_requests":        {Included: "10000", Unit: "requests", Tiers: []billing.Tier{{PricePerUnit: "0.0015"}}},
		"processed_documents": {Included: "100", Unit: "documents", Tiers: []billing.Tier{{PricePerUnit: "0.10"}}},
		"storage_bytes":       {Included: "10", Unit: "GB-month", Tiers: []billing.Tier{{PricePerUnit: "0.20"}}},
	}}
	growth := billing.Plan{Key: "growth", MonthlyBase: "79.00", Prices: map[string]billing.MetricPrice{
		"api_requests":        {Included: "100000", Unit: "requests", Tiers: []billing.Tier{{PricePerUnit: "0.001"}}},
		"processed_documents": {Included: "1000", Unit: "documents", Tiers: []billing.Tier{{PricePerUnit: "0.07"}}},
		"storage_bytes":       {Included: "100", Unit: "GB-month", Tiers: []billing.Tier{{PricePerUnit: "0.15"}}},
	}}
	scale := billing.Plan{Key: "scale", MonthlyBase: "249.00", Prices: map[string]billing.MetricPrice{
		"api_requests":        {Included: "1000000", Unit: "requests", Tiers: []billing.Tier{{PricePerUnit: "0.0006"}}},
		"processed_documents": {Included: "10000", Unit: "documents", Tiers: []billing.Tier{{PricePerUnit: "0.04"}}},
		"storage_bytes":       {Included: "1000", Unit: "GB-month", Tiers: []billing.Tier{{PricePerUnit: "0.10"}}},
	}}
	return []demoCustomer{
		{ID: "startup", Name: "Northstar Studio", Plan: starter, Events: []billing.Event{{Metric: "api_requests", Quantity: 12500}}, Storage: []billing.StoragePoint{}},
		{ID: "growth", Name: "Brightpath Labs", Plan: growth, Events: []billing.Event{{Metric: "api_requests", Quantity: 130000}, {Metric: "processed_documents", Quantity: 1200}}, Storage: []billing.StoragePoint{}},
		{ID: "scale", Name: "Atlas Systems", Plan: scale, Events: []billing.Event{{Metric: "api_requests", Quantity: 1050000}, {Metric: "processed_documents", Quantity: 10200}}, Storage: []billing.StoragePoint{}},
	}
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","mode":"demo"}`))
	})
	mux.HandleFunc("GET /api/customers", func(w http.ResponseWriter, r *http.Request) {
		rows := make([]map[string]string, 0, 3)
		for _, c := range demoPlans() {
			rows = append(rows, map[string]string{"id": c.ID, "name": c.Name, "plan": c.Plan.Key})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rows)
	})
	mux.HandleFunc("GET /api/preview", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		id := r.URL.Query().Get("customer_id")
		if id == "" {
			id = "startup"
		}
		var selected *demoCustomer
		customers := demoPlans()
		for i := range customers {
			if customers[i].ID == id {
				selected = &customers[i]
				break
			}
		}
		if selected == nil {
			http.Error(w, "customer not found", http.StatusNotFound)
			return
		}
		p, e := calculatePreview(selected, now, nil)
		if e != nil {
			http.Error(w, "preview unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(p)
	})
	mux.HandleFunc("POST /api/simulate", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var request struct {
			CustomerID string `json:"customer_id"`
			Metric     string `json:"metric"`
			Quantity   *int64 `json:"quantity"`
		}
		if err := dec.Decode(&request); err != nil {
			http.Error(w, "invalid simulation request", http.StatusBadRequest)
			return
		}
		if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			http.Error(w, "request must contain one JSON object", http.StatusBadRequest)
			return
		}
		if request.CustomerID == "" || request.Metric == "" || request.Quantity == nil || *request.Quantity < 0 || *request.Quantity > 10_000_000 {
			http.Error(w, "customer_id is required and quantity must be between 0 and 10000000", http.StatusBadRequest)
			return
		}
		var selected *demoCustomer
		customers := demoPlans()
		for i := range customers {
			if customers[i].ID == request.CustomerID {
				selected = &customers[i]
				break
			}
		}
		if selected == nil {
			http.Error(w, "customer not found", http.StatusNotFound)
			return
		}
		if _, ok := selected.Plan.Prices[request.Metric]; !ok || request.Metric == "storage_bytes" {
			http.Error(w, "simulation supports configured counter metrics only", http.StatusBadRequest)
			return
		}
		now := time.Now().UTC()
		extra := &billing.Event{Metric: request.Metric, Quantity: *request.Quantity, At: now.Add(-time.Second)}
		preview, err := calculatePreview(selected, now, extra)
		if err != nil {
			http.Error(w, "simulation unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(preview)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><meta name="viewport" content="width=device-width"><title>Usage billing demo</title><style>body{font:16px system-ui;max-width:850px;margin:4rem auto;padding:0 1rem;color:#182230}h1{font-size:2.2rem}button,select{padding:.7rem 1rem;border:1px solid #9aa;border-radius:6px}pre{background:#f4f6f5;padding:1rem;overflow:auto}small{color:#667}li{margin:.4rem 0}</style></head><body><h1>Usage billing</h1><p>Three seeded customers on the Starter, Growth and Scale plans.</p><ul id="customers"></ul><label for="customer">Customer</label> <select id="customer"><option value="startup">Northstar Studio — Starter</option><option value="growth">Brightpath Labs — Growth</option><option value="scale">Atlas Systems — Scale</option></select> <button id="load">Preview invoice</button> <label for="metric">Metric</label> <select id="metric"><option value="api_requests">API requests</option><option value="processed_documents">Processed documents</option></select> <label for="quantity">Add usage for preview</label> <input id="quantity" type="number" min="0" max="10000000" value="25000"> <button id="sim">Calculate simulation</button><h2>Accrued charges</h2><pre id="out">Choose a customer and load a preview.</pre><small>Local demo only. Simulations affect this preview calculation only and are not saved. This server has no authentication.</small><script>async function load(){const o=document.querySelector('#out');o.textContent='Loading…';try{o.textContent=JSON.stringify(await (await fetch('/api/preview?customer_id='+encodeURIComponent(document.querySelector('#customer').value)).then(r=>{if(!r.ok)throw Error();return r.json()})),null,2)}catch(e){o.textContent='Unable to load preview.'}}document.querySelector('#load').onclick=load;document.querySelector('#sim').onclick=async()=>{const o=document.querySelector('#out');o.textContent='Calculating simulation…';try{const r=await fetch('/api/simulate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({customer_id:document.querySelector('#customer').value,metric:document.querySelector('#metric').value,quantity:Number(document.querySelector('#quantity').value)})});if(!r.ok)throw Error();o.textContent=JSON.stringify(await r.json(),null,2)}catch(e){o.textContent='Simulation rejected or unavailable.'}};fetch('/api/customers').then(r=>r.json()).then(xs=>{document.querySelector('#customers').innerHTML=xs.map(x=>'<li>'+x.name+' · '+x.plan+'</li>').join('')})</script></body></html>`))
	})
	return mux
}

func calculatePreview(customer *demoCustomer, now time.Time, extra *billing.Event) (billing.Preview, error) {
	start, end := billing.MonthBounds(now)
	events := make([]billing.Event, 0, len(customer.Events)+1)
	for _, event := range customer.Events {
		event.At = now.Add(-time.Minute)
		events = append(events, event)
	}
	if extra != nil {
		events = append(events, *extra)
	}
	storage := []billing.StoragePoint{{At: start, Bytes: 0}}
	return billing.PreviewInvoice([]billing.Segment{{Plan: customer.Plan, Start: start, End: end}}, events, storage, start, end, now)
}
