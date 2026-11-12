package billing

import (
	"reflect"
	"testing"
	"time"
)

func ts(s string) time.Time {
	t, e := time.Parse(time.RFC3339, s)
	if e != nil {
		panic(e)
	}
	return t
}
func starter() Plan {
	return Plan{Key: "starter", MonthlyBase: "19.00", Prices: map[string]MetricPrice{"api_requests": {Included: "10000", Unit: "requests", Tiers: []Tier{{PricePerUnit: "0.0015"}}}}}
}
func TestStarterCounterOverage(t *testing.T) {
	start, end := MonthBounds(ts("2026-01-15T00:00:00Z"))
	got, e := PreviewInvoice([]Segment{{Plan: starter(), Start: start, End: end}}, []Event{{Metric: "api_requests", Quantity: 12500, At: start.Add(time.Hour)}}, nil, start, end, end)
	if e != nil {
		t.Fatal(e)
	}
	if got.Subtotal != 2275 {
		t.Fatalf("subtotal %s", got.Subtotal.String())
	}
	if got.Lines[1].Amount != 375 {
		t.Fatalf("overage %s", got.Lines[1].Amount.String())
	}
}
func TestGraduatedOverage(t *testing.T) {
	start, end := MonthBounds(ts("2026-01-15T00:00:00Z"))
	p := starter()
	p.Prices["api_requests"] = MetricPrice{Included: "0", Unit: "requests", Tiers: []Tier{{UpTo: ptr(1000), PricePerUnit: "0.01"}, {PricePerUnit: "0.005"}}}
	got, e := PreviewInvoice([]Segment{{Plan: p, Start: start, End: end}}, []Event{{Metric: "api_requests", Quantity: 1500, At: start}}, nil, start, end, end)
	if e != nil {
		t.Fatal(e)
	}
	if len(got.Lines) != 3 || got.Lines[1].Amount+got.Lines[2].Amount != 1250 {
		t.Fatalf("expected graduated lines totaling $12.50, got %#v", got.Lines)
	}
	if got.Lines[1].Rate != "0.01" || got.Lines[2].Rate != "0.005" {
		t.Fatalf("rates not explained on lines: %#v", got.Lines)
	}
}
func TestStorageHalfMonth(t *testing.T) {
	start, end := MonthBounds(ts("2026-01-15T00:00:00Z"))
	p := starter()
	p.Prices = map[string]MetricPrice{"storage_bytes": {Included: "10", Unit: "GB-month", Tiers: []Tier{{PricePerUnit: "0.20"}}}}
	mid := start.Add(end.Sub(start) / 2)
	points := []StoragePoint{{At: start, Bytes: 0}, {At: mid, Bytes: 50 * DecimalGB}}
	got, e := PreviewInvoice([]Segment{{Plan: p, Start: start, End: end}}, nil, points, start, end, end)
	if e != nil {
		t.Fatal(e)
	}
	if got.Lines[len(got.Lines)-1].Amount != 300 {
		t.Fatalf("expected $3.00, got %s", got.Lines[len(got.Lines)-1].Amount.String())
	}
}
func TestHalfOpenPeriodAndProration(t *testing.T) {
	start, end := MonthBounds(ts("2026-01-15T00:00:00Z"))
	p := starter()
	got, e := PreviewInvoice([]Segment{{Plan: p, Start: start, End: end}}, []Event{{Metric: "api_requests", Quantity: 12000, At: end}}, nil, start, end, end)
	if e != nil {
		t.Fatal(e)
	}
	if len(got.Lines) != 1 {
		t.Fatalf("end boundary event produced an overage: %#v", got.Lines)
	}
}
func TestRoundingHalfUp(t *testing.T) {
	m, e := ParseMoney("1.005")
	if e != nil || m != 101 {
		t.Fatalf("got %v %v", m, e)
	}
}
func ptr(v int64) *int64 { return &v }

func TestPartialPreviewUsesFullAllowanceForActiveSegment(t *testing.T) {
	start, end := MonthBounds(ts("2026-01-15T00:00:00Z"))
	mid := start.Add(end.Sub(start) / 2)
	got, err := PreviewInvoice([]Segment{{Plan: starter(), Start: start, End: end}}, []Event{{Metric: "api_requests", Quantity: 12_500, At: start.Add(time.Hour)}}, nil, start, end, mid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Lines[len(got.Lines)-1].Amount != 375 {
		t.Fatalf("expected $3.75 overage, got %s", got.Lines[len(got.Lines)-1].Amount.String())
	}
	if got.Lines[0].Amount != 950 {
		t.Fatalf("expected half-month base fee of $9.50, got %s", got.Lines[0].Amount.String())
	}
}

func TestDecimalInputsRejectFractionsAndScientificNotation(t *testing.T) {
	for _, value := range []string{"1/2", "1e3", "NaN", ""} {
		if _, err := ParseMoney(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
}

func TestInvoiceTotalOverflowIsReported(t *testing.T) {
	if _, err := addMoney(Money(1<<63-1), Money(1)); err == nil {
		t.Fatal("expected overflow error")
	}
}

func TestPreviewRejectsUnpricedAndDuplicateEvents(t *testing.T) {
	start, end := MonthBounds(ts("2026-01-15T00:00:00Z"))
	segment := []Segment{{Plan: starter(), Start: start, End: end}}
	unknown := []Event{{Metric: "unpriced_metric", Quantity: 1, At: start.Add(time.Hour)}}
	if _, err := PreviewInvoice(segment, unknown, nil, start, end, end); err == nil {
		t.Fatal("expected unpriced usage to be rejected")
	}
	duplicate := []Event{{Source: "worker", EventID: "evt-1", Metric: "api_requests", Quantity: 1, At: start}, {Source: "worker", EventID: "evt-1", Metric: "api_requests", Quantity: 1, At: start}}
	if _, err := PreviewInvoice(segment, duplicate, nil, start, end, end); err == nil {
		t.Fatal("expected duplicate event identity to be rejected")
	}
}

func TestPreviewLineOrderingIsDeterministic(t *testing.T) {
	start, end := MonthBounds(ts("2026-01-15T00:00:00Z"))
	plan := starter()
	plan.Prices["processed_documents"] = MetricPrice{Included: "0", Unit: "documents", Tiers: []Tier{{PricePerUnit: "0.10"}}}
	events := []Event{{Metric: "processed_documents", Quantity: 1, At: start}, {Metric: "api_requests", Quantity: 10001, At: start}}
	one, err := PreviewInvoice([]Segment{{Plan: plan, Start: start, End: end}}, events, nil, start, end, end)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		next, err := PreviewInvoice([]Segment{{Plan: plan, Start: start, End: end}}, events, nil, start, end, end)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(one, next) {
			t.Fatalf("preview ordering changed: %#v != %#v", one.Lines, next.Lines)
		}
	}
}

func TestStorageRatesFollowHistoricalPlanSegments(t *testing.T) {
	start, end := MonthBounds(ts("2026-01-15T00:00:00Z"))
	mid := start.Add(end.Sub(start) / 2)
	mkPlan := func(key, rate string) Plan {
		return Plan{Key: key, MonthlyBase: "0", Prices: map[string]MetricPrice{"storage_bytes": {Included: "0", Unit: "GB-month", Tiers: []Tier{{PricePerUnit: rate}}}}}
	}
	segments := []Segment{{Plan: mkPlan("old", "0.20"), Start: start, End: mid}, {Plan: mkPlan("new", "0.10"), Start: mid, End: end}}
	points := []StoragePoint{{At: start, Bytes: 20 * DecimalGB}}
	got, err := PreviewInvoice(segments, nil, points, start, end, end)
	if err != nil {
		t.Fatal(err)
	}
	var storageLines int
	var storageTotal Money
	for _, line := range got.Lines {
		if line.Unit == "GB-month" {
			storageLines++
			storageTotal += line.Amount
		}
	}
	if storageLines != 2 || storageTotal != 300 {
		t.Fatalf("expected two historical-rate lines totaling $3.00; lines=%#v", got.Lines)
	}
	if len(got.Usage) != 2 || got.Usage[0].Recorded != "10.000000000000" || got.Usage[1].Recorded != "10.000000000000" {
		t.Fatalf("storage segment usage not explained: %#v", got.Usage)
	}
}

func TestInvalidTierSchedulesFailBeforeBilling(t *testing.T) {
	start, end := MonthBounds(ts("2026-01-15T00:00:00Z"))
	p := starter()
	p.Prices["api_requests"] = MetricPrice{Included: "10000", Unit: "requests", Tiers: []Tier{{PricePerUnit: "0.01"}, {UpTo: ptr(100), PricePerUnit: "0.02"}}}
	if _, err := PreviewInvoice([]Segment{{Plan: p, Start: start, End: end}}, nil, nil, start, end, end); err == nil {
		t.Fatal("expected unbounded tier before a bounded tier to be rejected")
	}
	p.Prices["api_requests"] = MetricPrice{Included: "10000", Unit: "requests", Tiers: []Tier{{UpTo: ptr(100), PricePerUnit: "0.01"}, {UpTo: ptr(100), PricePerUnit: "0.02"}}}
	if _, err := PreviewInvoice([]Segment{{Plan: p, Start: start, End: end}}, nil, nil, start, end, end); err == nil {
		t.Fatal("expected repeated tier boundary to be rejected")
	}
}
