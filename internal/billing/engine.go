package billing

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

const DecimalGB = int64(1_000_000_000)

type Tier struct {
	UpTo         *int64 `json:"up_to,omitempty"`
	PricePerUnit string `json:"price_per_unit"`
}
type MetricPrice struct {
	Included string `json:"included"`
	Unit     string `json:"unit"`
	Tiers    []Tier `json:"tiers"`
}
type Plan struct {
	Key         string                 `json:"key"`
	MonthlyBase string                 `json:"monthly_base"`
	Prices      map[string]MetricPrice `json:"prices"`
}
type Segment struct {
	Plan       Plan
	Start, End time.Time
}
type Event struct {
	Source   string
	EventID  string
	Metric   string
	Quantity int64
	At       time.Time
}
type StoragePoint struct {
	Bytes int64
	At    time.Time
}
type Line struct {
	Description string `json:"description"`
	Quantity    string `json:"quantity"`
	Unit        string `json:"unit"`
	Rate        string `json:"rate"`
	Amount      Money  `json:"amount_cents"`
}
type UsageSummary struct {
	Metric       string    `json:"metric"`
	Plan         string    `json:"plan"`
	SegmentStart time.Time `json:"segment_start"`
	SegmentEnd   time.Time `json:"segment_end"`
	Recorded     string    `json:"recorded_quantity"`
	Included     string    `json:"included_quantity"`
	Overage      string    `json:"overage_quantity"`
	Unit         string    `json:"unit"`
	PricingTiers []Tier    `json:"pricing_tiers"`
}
type Preview struct {
	CalculatedAt time.Time      `json:"calculated_at"`
	PeriodStart  time.Time      `json:"period_start"`
	PeriodEnd    time.Time      `json:"period_end"`
	Lines        []Line         `json:"lines"`
	Usage        []UsageSummary `json:"usage"`
	Subtotal     Money          `json:"subtotal_cents"`
	Total        Money          `json:"total_cents"`
}

func MonthBounds(t time.Time) (time.Time, time.Time) {
	u := t.UTC()
	s := time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
	return s, s.AddDate(0, 1, 0)
}
func duration(a, b time.Time) *big.Rat {
	return new(big.Rat).SetFrac(big.NewInt(b.Sub(a).Nanoseconds()), big.NewInt(int64(time.Second)))
}
func ratDecimal(s string) (*big.Rat, error) {
	if !decimalText(s) {
		return nil, fmt.Errorf("invalid nonnegative decimal %q", s)
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() < 0 {
		return nil, fmt.Errorf("invalid nonnegative decimal %q", s)
	}
	return r, nil
}
func addMoney(a, b Money) (Money, error) {
	n := new(big.Int).Add(big.NewInt(int64(a)), big.NewInt(int64(b)))
	if !n.IsInt64() {
		return 0, fmt.Errorf("invoice total overflow")
	}
	return Money(n.Int64()), nil
}
func moneyFromRat(r *big.Rat) (Money, error) {
	n := roundRat(r)
	if !n.IsInt64() {
		return 0, fmt.Errorf("money overflow")
	}
	return Money(n.Int64()), nil
}

// Explanatory quantities are rendered to 12 decimal places; all pricing uses
// the underlying exact rational, never this display representation.
func quantityString(r *big.Rat) string { return r.FloatString(12) }

func PreviewInvoice(segments []Segment, events []Event, points []StoragePoint, periodStart, periodEnd, asOf time.Time) (Preview, error) {
	p := Preview{CalculatedAt: asOf.UTC(), PeriodStart: periodStart.UTC(), PeriodEnd: periodEnd.UTC(), Lines: []Line{}, Usage: []UsageSummary{}}
	if !periodStart.Before(periodEnd) {
		return p, fmt.Errorf("invalid billing interval")
	}
	if asOf.Before(periodStart) {
		return p, fmt.Errorf("calculation precedes period")
	}
	if asOf.After(periodEnd) {
		asOf = periodEnd
	}
	seenEvents := make(map[string]struct{}, len(events))
	recognizedEvents := make([]bool, len(events))
	for _, ev := range events {
		if ev.Quantity < 0 {
			return p, fmt.Errorf("counter quantity cannot be negative")
		}
		if ev.EventID != "" || ev.Source != "" {
			if ev.EventID == "" || ev.Source == "" {
				return p, fmt.Errorf("event source and event ID must be supplied together")
			}
			identity := ev.Source + "\x00" + ev.EventID
			if _, exists := seenEvents[identity]; exists {
				return p, fmt.Errorf("duplicate usage event identity")
			}
			seenEvents[identity] = struct{}{}
		}
	}
	segments = append([]Segment(nil), segments...)
	sort.Slice(segments, func(i, j int) bool { return segments[i].Start.Before(segments[j].Start) })
	for i := 1; i < len(segments); i++ {
		if segments[i].Start.Before(segments[i-1].End) {
			return p, fmt.Errorf("subscription segments overlap")
		}
	}
	for _, segment := range segments {
		if !segment.Start.Before(segment.End) {
			return p, fmt.Errorf("subscription segment has an invalid interval")
		}
	}
	points = append([]StoragePoint(nil), points...)
	sort.Slice(points, func(i, j int) bool { return points[i].At.Before(points[j].At) })
	for i := 1; i < len(points); i++ {
		if points[i].At.Equal(points[i-1].At) {
			return p, fmt.Errorf("multiple storage changes have the same timestamp")
		}
		if points[i].Bytes < 0 {
			return p, fmt.Errorf("negative storage balance")
		}
	}
	for _, point := range points {
		if point.Bytes < 0 {
			return p, fmt.Errorf("negative storage balance")
		}
	}
	for _, s := range segments {
		a := maxTime(s.Start, periodStart)
		b := minTime(s.End, periodEnd)
		accruedEnd := minTime(b, asOf)
		if !a.Before(b) || !a.Before(accruedEnd) {
			continue
		}
		base, e := ratDecimal(s.Plan.MonthlyBase)
		if e != nil {
			return p, e
		}
		fee := new(big.Rat).Mul(base, new(big.Rat).Quo(duration(a, accruedEnd), duration(periodStart, periodEnd)))
		m, e := moneyFromRat(new(big.Rat).Mul(fee, big.NewRat(100, 1)))
		if e != nil {
			return p, e
		}
		p.Lines = append(p.Lines, Line{Description: "Base fee — " + s.Plan.Key, Quantity: quantityString(new(big.Rat).Quo(duration(a, accruedEnd), duration(periodStart, periodEnd))), Unit: "billing period", Rate: s.Plan.MonthlyBase, Amount: m})
		p.Subtotal, e = addMoney(p.Subtotal, m)
		if e != nil {
			return p, e
		}
		secs := duration(a, b)
		metrics := make([]string, 0, len(s.Plan.Prices))
		for metric := range s.Plan.Prices {
			metrics = append(metrics, metric)
		}
		sort.Strings(metrics)
		for _, metric := range metrics {
			pr := s.Plan.Prices[metric]
			if strings.TrimSpace(metric) == "" || strings.TrimSpace(pr.Unit) == "" {
				return p, fmt.Errorf("metric key and unit are required")
			}
			if err := validateTiers(pr.Tiers); err != nil {
				return p, fmt.Errorf("metric %s: %w", metric, err)
			}
			inc, e := ratDecimal(pr.Included)
			if e != nil {
				return p, e
			}
			prorated := new(big.Rat).Mul(inc, new(big.Rat).Quo(secs, duration(periodStart, periodEnd)))
			if metric == "storage_bytes" {
				storageEnd := minTime(b, asOf)
				byteSeconds, err := storageByteSeconds(points, a, storageEnd)
				if err != nil {
					return p, err
				}
				gbMonths := new(big.Rat).Quo(byteSeconds, new(big.Rat).Mul(duration(periodStart, periodEnd), big.NewRat(DecimalGB, 1)))
				average := new(big.Rat).Sub(gbMonths, prorated)
				if average.Sign() < 0 {
					average.SetInt64(0)
				}
				p.Usage = append(p.Usage, UsageSummary{Metric: metric, Plan: s.Plan.Key, SegmentStart: a.UTC(), SegmentEnd: b.UTC(), Recorded: quantityString(gbMonths), Included: quantityString(prorated), Overage: quantityString(average), Unit: "GB-month", PricingTiers: append([]Tier(nil), pr.Tiers...)})
				charges, err := tierCharges(average, pr.Tiers, "GB-month")
				if err != nil {
					return p, err
				}
				for _, charge := range charges {
					amount, err := moneyFromRat(new(big.Rat).Mul(new(big.Rat).Mul(charge.quantity, charge.rate), big.NewRat(100, 1)))
					if err != nil {
						return p, err
					}
					if amount == 0 && charge.quantity.Sign() == 0 {
						continue
					}
					description := "Storage overage (" + s.Plan.Key + ")"
					if charge.tier != "" {
						description += " — " + charge.tier
					}
					p.Lines = append(p.Lines, Line{Description: description, Quantity: quantityString(charge.quantity), Unit: "GB-month", Rate: charge.rateText, Amount: amount})
					p.Subtotal, err = addMoney(p.Subtotal, amount)
					if err != nil {
						return p, err
					}
				}
				continue
			}
			// Counter allowances use floor per effective segment. This is deterministic
			// and prevents fractional included requests from being granted.
			allowance := new(big.Int).Quo(prorated.Num(), prorated.Denom())
			used := int64(0)
			for eventIndex, ev := range events {
				if ev.Metric == metric && !ev.At.Before(a) && ev.At.Before(asOf) && ev.At.Before(b) && ev.Quantity < 0 {
					return p, fmt.Errorf("counter quantity cannot be negative")
				} else if ev.Metric == metric && !ev.At.Before(a) && ev.At.Before(asOf) && ev.At.Before(b) {
					recognizedEvents[eventIndex] = true
					if ev.Quantity == 0 {
						continue
					}
					if used > int64(^uint64(0)>>1)-ev.Quantity {
						return p, fmt.Errorf("usage overflow")
					}
					used += ev.Quantity
				}
			}
			excess := new(big.Int).Sub(big.NewInt(used), allowance)
			if excess.Sign() < 0 {
				excess.SetInt64(0)
			}
			p.Usage = append(p.Usage, UsageSummary{Metric: metric, Plan: s.Plan.Key, SegmentStart: a.UTC(), SegmentEnd: b.UTC(), Recorded: big.NewInt(used).String(), Included: allowance.String(), Overage: excess.String(), Unit: pr.Unit, PricingTiers: append([]Tier(nil), pr.Tiers...)})
			charges, e := tierCharges(new(big.Rat).SetInt(excess), pr.Tiers, pr.Unit)
			if e != nil {
				return p, e
			}
			for _, charge := range charges {
				amount, e := moneyFromRat(new(big.Rat).Mul(new(big.Rat).Mul(charge.quantity, charge.rate), big.NewRat(100, 1)))
				if e != nil {
					return p, e
				}
				if amount == 0 && charge.quantity.Sign() == 0 {
					continue
				}
				description := "Usage overage — " + metric + " (" + s.Plan.Key + ")"
				if charge.tier != "" {
					description += " — " + charge.tier
				}
				p.Lines = append(p.Lines, Line{Description: description, Quantity: quantityString(charge.quantity), Unit: pr.Unit, Rate: charge.rateText, Amount: amount})
				p.Subtotal, e = addMoney(p.Subtotal, amount)
				if e != nil {
					return p, e
				}
			}
		}
	}
	for i, ev := range events {
		if !ev.At.Before(periodStart) && ev.At.Before(asOf) && ev.At.Before(periodEnd) && !recognizedEvents[i] {
			return p, fmt.Errorf("usage event does not fall in an active priced segment")
		}
	}
	p.Total = p.Subtotal
	return p, nil
}

func storageByteSeconds(points []StoragePoint, start, end time.Time) (*big.Rat, error) {
	if !start.Before(end) {
		return new(big.Rat), nil
	}
	current := int64(0)
	found := false
	for _, point := range points {
		if point.At.After(start) {
			break
		}
		current = point.Bytes
		found = true
	}
	if !found {
		return nil, fmt.Errorf("storage history must include a point at or before segment start")
	}
	cursor := start
	area := new(big.Rat)
	for _, point := range points {
		if !point.At.After(start) {
			continue
		}
		if point.At.After(end) {
			break
		}
		area.Add(area, new(big.Rat).Mul(big.NewRat(current, 1), duration(cursor, point.At)))
		current = point.Bytes
		cursor = point.At
	}
	area.Add(area, new(big.Rat).Mul(big.NewRat(current, 1), duration(cursor, end)))
	return area, nil
}

type tierCharge struct {
	quantity *big.Rat
	rate     *big.Rat
	rateText string
	tier     string
}

func tierCharges(q *big.Rat, tiers []Tier, unit string) ([]tierCharge, error) {
	if q.Sign() < 0 {
		return nil, fmt.Errorf("billable quantity cannot be negative")
	}
	if err := validateTiers(tiers); err != nil {
		return nil, err
	}
	remaining := new(big.Rat).Set(q)
	var lower int64
	charges := make([]tierCharge, 0, len(tiers))
	for i, t := range tiers {
		r, e := ratDecimal(t.PricePerUnit)
		if e != nil {
			return nil, e
		}
		var width *big.Rat
		if t.UpTo == nil {
			width = remaining
		} else {
			if *t.UpTo < lower {
				return nil, fmt.Errorf("unordered pricing tier")
			}
			width = big.NewRat(*t.UpTo-lower, 1)
			if width.Cmp(remaining) > 0 {
				width = remaining
			}
		}
		if width.Sign() > 0 {
			name := ""
			if len(tiers) > 1 {
				if t.UpTo == nil {
					name = fmt.Sprintf("tier %d (at least %d %s)", i+1, lower, unit)
				} else {
					name = fmt.Sprintf("tier %d (%d to less than %d %s)", i+1, lower, *t.UpTo, unit)
				}
			}
			charges = append(charges, tierCharge{quantity: new(big.Rat).Set(width), rate: r, rateText: t.PricePerUnit, tier: name})
		}
		remaining.Sub(remaining, width)
		if remaining.Sign() <= 0 {
			break
		}
		if t.UpTo != nil {
			lower = *t.UpTo
		}
	}
	if remaining.Sign() > 0 {
		return nil, fmt.Errorf("pricing schedule does not cover usage")
	}
	return charges, nil
}

func validateTiers(tiers []Tier) error {
	if len(tiers) == 0 {
		return fmt.Errorf("at least one pricing tier is required")
	}
	var boundary int64
	unbounded := false
	for i, tier := range tiers {
		if _, err := ratDecimal(tier.PricePerUnit); err != nil {
			return fmt.Errorf("tier %d: %w", i+1, err)
		}
		if tier.UpTo == nil {
			if i != len(tiers)-1 {
				return fmt.Errorf("unbounded tier must be last")
			}
			unbounded = true
			continue
		}
		if unbounded || *tier.UpTo <= boundary {
			return fmt.Errorf("tier boundaries must be strictly increasing and unbounded tier must be last")
		}
		boundary = *tier.UpTo
	}
	return nil
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
