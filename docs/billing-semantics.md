# Billing semantics

The Go billing package is the executable reference for these policies. Billing periods use UTC calendar months and half-open intervals `[start,end)`. An event exactly at `end` belongs to the next period. The caller supplies `asOf`; calculations are reproducible for equal inputs. Base fees accrue by elapsed seconds divided by seconds in that calendar month. Every plan segment is priced using its plan value. Counter allowances are prorated by the full effective segment duration within the billing period and floored per segment, even when a preview is calculated before that segment ends; base fees accrue only through the preview time; usage is assigned to the segment containing its timestamp. Overage tiers are graduated: a tier with `up_to: N` covers the next units up to but excluding cumulative boundary N; the following tier begins at N. Each applied tier is emitted as a separate invoice line with its exact quantity and decimal unit rate.

Money input and rates are decimal strings. Calculations use `math/big.Rat`; each line amount rounds half up to cents. The persisted/returned invoice total is the sum of rounded line amounts. Explanatory quantities are rendered to 12 decimal places; calculations always use the exact rational quantities, not those display strings. Currency currently defaults to USD; currencies with other minor-unit rules are not implemented.

Storage is a gauge: the engine integrates bytes over time separately for each historical plan segment, carries a point at or before segment start forward, and divides by the billing period's exact seconds and 1,000,000,000 bytes per decimal GB. Each segment uses its own historical rate and a GB-month allowance prorated by segment duration. The preview returns per-segment recorded, included, and overage quantities for counters and storage. Storage history is supplied directly to the calculation package; the HTTP demo does not persist it.

Examples:

- Starter: 12,500 requests less 10,000 included = 2,500 × $0.0015 = **$3.75**, plus the $19 base fee.
- Graduated: first 1,000 overage units at $0.01 and next 500 at $0.005 = **$12.50**.
- Storage: 50 GB for half a 30-day month = 25 GB-months. Against a full-period 10 GB-month allowance, $0.20/GB-month yields **$3.00** overage.
- Rounding: a line calculation of $1.005 rounds to $1.01 (ROUND_HALF_UP).
- Late events: the current library previews supplied events and does not enforce period closure or a 48-hour finalization grace. The schema records period/grace metadata, but the demo has no ingestion or invoice-finalization service.

Preview calculations are not finalized invoices. The runnable API is a nonpersistent demonstration and does not expose invoice finalization.
