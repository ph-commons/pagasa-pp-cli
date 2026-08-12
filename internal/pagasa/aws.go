// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package pagasa

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ph-commons/pagasa-pp-cli/internal/cliutil"
)

// PHT is Philippine Standard Time (UTC+8). Used instead of time.LoadLocation
// so parsing works on minimal Linux images without tzdata (e.g. hermes0).
var PHT = time.FixedZone("PHT", 8*3600)

// AWSStation is one row from the public Automated Weather Stations table
// (https://bagong.pagasa.dost.gov.ph/automated-weather-station/).
// The page publishes only the latest snapshot per station — not multi-hour history.
type AWSStation struct {
	StationID     string   `json:"station_id"`
	StationName   string   `json:"station_name"`
	TempC         *float64 `json:"temp_c,omitempty"`
	HumidityPct   *float64 `json:"humidity_pct,omitempty"`
	WindKmh       *float64 `json:"wind_kmh,omitempty"`
	WindDir       string   `json:"wind_dir,omitempty"`
	PrecipMmHr    *float64 `json:"precip_mm_hr,omitempty"`
	Pressure      *float64 `json:"pressure,omitempty"`
	Solar         *float64 `json:"solar,omitempty"`
	ObservedAt    string   `json:"observed_at,omitempty"` // RFC3339 UTC when Last Updated parsed
	LastUpdated   string   `json:"last_updated_raw,omitempty"`
	ParseWarnings []string `json:"parse_warnings,omitempty"`
}

// awsRowRe matches one data row (10 cells) in the AWS table body.
// Classes on <td> vary; content is what matters.
var awsRowRe = regexp.MustCompile(`(?is)<tr>\s*` +
	`<td[^>]*>\s*(\d+)\s*</td>\s*` +
	`<td[^>]*>\s*([^<]*?)\s*</td>\s*` +
	`<td[^>]*>\s*([^<]*?)\s*</td>\s*` +
	`<td[^>]*>\s*([^<]*?)\s*</td>\s*` +
	`<td[^>]*>\s*([^<]*?)\s*</td>\s*` +
	`<td[^>]*>\s*([^<]*?)\s*</td>\s*` +
	`<td[^>]*>\s*([^<]*?)\s*</td>\s*` +
	`<td[^>]*>\s*([^<]*?)\s*</td>\s*` +
	`<td[^>]*>\s*([^<]*?)\s*</td>\s*` +
	`<td[^>]*>\s*([^<]*?)\s*</td>\s*` +
	`</tr>`)

// ParseAWSStations extracts station rows from the bagong AWS HTML table.
// Malformed cells become nil fields / empty strings; rows without a site id are skipped.
// One bad row never fails the whole parse. No sensor QC is applied.
func ParseAWSStations(html string) []AWSStation {
	matches := awsRowRe.FindAllStringSubmatch(html, -1)
	if matches == nil {
		return nil
	}
	out := make([]AWSStation, 0, len(matches))
	for _, m := range matches {
		st := AWSStation{
			StationID:   strings.TrimSpace(m[1]),
			StationName: cliutil.CleanText(m[2]),
			WindDir:     cliutil.CleanText(m[6]),
			LastUpdated: cliutil.CleanText(m[10]),
		}
		if st.StationID == "" {
			continue
		}
		st.TempC = parseAWSFloat(m[3], "°C", "C")
		st.HumidityPct = parseAWSFloat(m[4], "%")
		st.WindKmh = parseAWSFloat(m[5], "km/hr", "km/h", "kph")
		st.PrecipMmHr = parseAWSFloat(m[7], "mm/hr", "mm/h")
		st.Pressure = parseAWSFloat(m[8])
		st.Solar = parseAWSFloat(m[9])
		if st.LastUpdated != "" {
			if t, err := ParseAWSLastUpdated(st.LastUpdated); err == nil {
				st.ObservedAt = t.UTC().Format(time.RFC3339)
			} else {
				st.ParseWarnings = append(st.ParseWarnings, "unparseable last_updated")
			}
		}
		out = append(out, st)
	}
	return out
}

// ParseAWSLastUpdated parses PAGASA wall-clock stamps such as
// "August 3, 2026, 7:40 am" as PHT and returns the instant (caller may .UTC()).
func ParseAWSLastUpdated(raw string) (time.Time, error) {
	s := strings.TrimSpace(cliutil.CleanText(raw))
	if s == "" {
		return time.Time{}, strconv.ErrSyntax
	}
	// Normalize am/pm to uppercase PM token Go expects in the layout.
	lower := strings.ToLower(s)
	var layout string
	var value string
	switch {
	case strings.HasSuffix(lower, " am"):
		value = strings.TrimSpace(s[:len(s)-3]) + " AM"
		layout = "January 2, 2006, 3:04 PM"
	case strings.HasSuffix(lower, " pm"):
		value = strings.TrimSpace(s[:len(s)-3]) + " PM"
		layout = "January 2, 2006, 3:04 PM"
	default:
		value = s
		layout = "January 2, 2006, 15:04"
	}
	t, err := time.ParseInLocation(layout, value, PHT)
	if err != nil {
		// Some rows use "July 28, 2026, 6:50 pm" without comma variations — already handled.
		return time.Time{}, err
	}
	return t, nil
}

func parseAWSFloat(raw string, stripSuffixes ...string) *float64 {
	s := strings.TrimSpace(cliutil.CleanText(raw))
	if s == "" || s == "-" || s == "--" || strings.EqualFold(s, "n/a") {
		return nil
	}
	lower := strings.ToLower(s)
	for _, suf := range stripSuffixes {
		lower = strings.ReplaceAll(lower, strings.ToLower(suf), "")
	}
	lower = strings.TrimSpace(lower)
	// Keep leading number only (handles "25 °C" after strip, "N 3.6" leftovers, etc.)
	num := leadingNumber(lower)
	if num == "" {
		return nil
	}
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return nil
	}
	return &f
}

var leadingNumberRe = regexp.MustCompile(`^-?\d+(?:\.\d+)?`)

func leadingNumber(s string) string {
	return leadingNumberRe.FindString(strings.TrimSpace(s))
}
