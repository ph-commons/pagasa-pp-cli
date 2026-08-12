// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live  (obs live scrape; history is local — see subcommand annotations)

package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ph-commons/pagasa-pp-cli/internal/client"
	"github.com/ph-commons/pagasa-pp-cli/internal/config"
	"github.com/ph-commons/pagasa-pp-cli/internal/pagasa"
	"github.com/ph-commons/pagasa-pp-cli/internal/store"
)

// bagongAWSBaseURL is the host that serves the Automated Weather Stations table.
// Distinct from the default CLI BaseURL (www.pagasa.dost.gov.ph).
const bagongAWSBaseURL = "https://bagong.pagasa.dost.gov.ph"

const awsStationsPath = "/automated-weather-station/"

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.AddCommand(newObsCmd(flags))
	})
}

type obsCaptureResult struct {
	Written  int                 `json:"written"`
	Skipped  int                 `json:"skipped"`
	Pruned   int64               `json:"pruned"`
	Stations []pagasa.AWSStation `json:"stations,omitempty"`
	Source   string              `json:"source"`
	Fetched  int                 `json:"fetched"`
}

func newObsCmd(flags *rootFlags) *cobra.Command {
	var (
		station string
		capture bool
		limit   int
	)
	cmd := &cobra.Command{
		Use:   "obs",
		Short: "PAGASA automated weather station observations (latest live table)",
		Long: `Fetch the latest Automated Weather Stations (AWS) table from bagong.pagasa.dost.gov.ph.

PAGASA publishes only the latest snapshot per station — not multi-hour history.
Local series require --capture on a schedule (e.g. cron every 10–15 minutes), then
query with "obs history". Default "obs" is live (no store write); --capture writes
and prunes local aws_obs (not MCP read-only).

Not a 60-minute METAR product; station Last Updated stamps are typically ~5–10 minutes.`,
		Example: `  pagasa-pp-cli obs --json
  pagasa-pp-cli obs --station 98 --json
  pagasa-pp-cli obs --station "Science Garden" --json
  pagasa-pp-cli obs --capture --agent
  pagasa-pp-cli obs history --station 98 --limit 24 --json`,
		// No mcp:read-only: --capture writes/prunes local store and hits open-world HTTP.
		// obs history remains read-only (see subcommand annotations). mcp:open-world: #24.
		Annotations: map[string]string{"mcp:open-world": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if capture && limit > 0 {
				return fmt.Errorf("--limit is for live display only; omit --limit when using --capture (capture stores all matched stations)")
			}
			if dryRunOK(flags) {
				if capture {
					fmt.Fprintln(cmd.OutOrStdout(), "would fetch bagong AWS table and capture to local aws_obs")
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "would fetch bagong AWS table (live, no store write)")
				}
				return nil
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			stations, err := fetchAWSStations(ctx, flags)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			stations = filterAWSStations(stations, station)
			// --limit applies only to live display, never to capture (footgun: partial series).
			if !capture && limit > 0 && len(stations) > limit {
				stations = stations[:limit]
			}

			if capture {
				res, err := captureAWSStations(ctx, stations)
				if err != nil {
					return err
				}
				if machineOut(cmd, flags) {
					return printJSONFiltered(cmd.OutOrStdout(), res, flags)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "captured %d stations (%d skipped, %d pruned)\n", res.Written, res.Skipped, res.Pruned)
				return nil
			}

			if machineOut(cmd, flags) {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"source":   "live",
					"count":    len(stations),
					"stations": stations,
				}, flags)
			}
			if len(stations) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No stations matched.")
				return nil
			}
			for _, st := range stations {
				temp := "-"
				if st.TempC != nil {
					temp = fmt.Sprintf("%.1f°C", *st.TempC)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %-36s  %s  %s\n", st.StationID, st.StationName, temp, st.LastUpdated)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&station, "station", "", "filter by site id or station name substring")
	cmd.Flags().BoolVar(&capture, "capture", false, "write scraped rows to local aws_obs and prune retention (for cron)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max stations for live display only; incompatible with --capture (0 = all matched)")

	cmd.AddCommand(newObsHistoryCmd(flags))
	return cmd
}

func newObsHistoryCmd(flags *rootFlags) *cobra.Command {
	var (
		station string
		limit   int
	)
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Local AWS observation history (requires prior obs --capture)",
		Long: `Read station observations from the local aws_obs table.

Empty list (not an error) when no captures have been stored yet, or when the
table has not been created (read-only open before any RW migrate).
Series grain follows PAGASA Last Updated stamps, not capture interval.`,
		Example:     `  pagasa-pp-cli obs history --station 98 --limit 24 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would read local aws_obs history")
				return nil
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			rows, err := loadAWSObsHistory(ctx, station, limit)
			if err != nil {
				return err
			}
			if rows == nil {
				rows = []store.AWSObsRow{}
			}
			if machineOut(cmd, flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No local AWS observations yet. Run 'pagasa-pp-cli obs --capture' on a schedule to build history.")
				return nil
			}
			for _, r := range rows {
				temp := "-"
				if r.TempC != nil {
					temp = fmt.Sprintf("%.1f°C", *r.TempC)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %-28s  %s\n", r.ObservedAt, r.StationID, r.StationName, temp)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&station, "station", "", "filter by site id or station name substring")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum rows (newest first)")
	return cmd
}

func fetchAWSStations(ctx context.Context, flags *rootFlags) ([]pagasa.AWSStation, error) {
	c, err := flags.newBagongClient()
	if err != nil {
		return nil, err
	}
	// Bypass HTTP cache so live/cron always see the current AWS table
	// (default Get may serve a 5-minute cached body).
	raw, err := c.GetNoCache(ctx, awsStationsPath, nil)
	if err != nil {
		return nil, err
	}
	stations := pagasa.ParseAWSStations(string(raw))
	if len(stations) == 0 {
		return nil, fmt.Errorf("no AWS stations parsed from %s%s; PAGASA may have restructured the page", bagongAWSBaseURL, awsStationsPath)
	}
	return stations, nil
}

// newBagongClient returns a client pointed at bagong for the AWS table only.
// Does not change the process-wide default BaseURL for other commands.
func (f *rootFlags) newBagongClient() (*client.Client, error) {
	cfg, err := config.Load(f.configPath)
	if err != nil {
		return nil, configErr(err)
	}
	if f.insecure {
		cfg.SetSkipTLSVerify(true)
	}
	cfg.BaseURL = bagongAWSBaseURL
	c := client.New(cfg, f.timeout, f.rateLimit)
	c.DryRun = f.dryRun
	c.NoCache = f.noCache
	if err := bindPlatformClient(c, f); err != nil {
		return nil, err
	}
	for _, hook := range clientHooks {
		if err := hook(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func filterAWSStations(stations []pagasa.AWSStation, filter string) []pagasa.AWSStation {
	f := strings.TrimSpace(filter)
	if f == "" {
		return stations
	}
	fl := strings.ToLower(f)
	out := make([]pagasa.AWSStation, 0, len(stations))
	for _, st := range stations {
		if st.StationID == f || strings.Contains(strings.ToLower(st.StationName), fl) {
			out = append(out, st)
		}
	}
	return out
}

func captureAWSStations(ctx context.Context, stations []pagasa.AWSStation) (obsCaptureResult, error) {
	res := obsCaptureResult{Source: "live", Fetched: len(stations), Stations: stations}
	rows := make([]store.AWSObsRow, 0, len(stations))
	capturedAt := time.Now().UTC().Format(time.RFC3339)
	for _, st := range stations {
		if st.ObservedAt == "" {
			res.Skipped++
			continue
		}
		rows = append(rows, store.AWSObsRow{
			StationID:   st.StationID,
			StationName: st.StationName,
			ObservedAt:  st.ObservedAt,
			CapturedAt:  capturedAt,
			TempC:       st.TempC,
			HumidityPct: st.HumidityPct,
			WindKmh:     st.WindKmh,
			WindDir:     st.WindDir,
			PrecipMmHr:  st.PrecipMmHr,
			Pressure:    st.Pressure,
			Solar:       st.Solar,
		})
	}

	db, err := store.OpenWithContext(ctx, defaultDBPath("pagasa-pp-cli"))
	if err != nil {
		return res, err
	}
	defer db.Close()

	written, skipped, err := db.UpsertAWSObsBatch(ctx, rows)
	if err != nil {
		return res, err
	}
	res.Written = written
	res.Skipped += skipped
	pruned, err := db.PruneAWSObs(ctx, store.DefaultAWSObsRetention)
	if err != nil {
		return res, err
	}
	res.Pruned = pruned
	// Don't echo full station list in capture JSON by default for token size —
	// clear stations unless caller wants them (keep Fetched/Written only).
	res.Stations = nil
	return res, nil
}

func loadAWSObsHistory(ctx context.Context, station string, limit int) ([]store.AWSObsRow, error) {
	dbPath := defaultDBPath("pagasa-pp-cli")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return []store.AWSObsRow{}, nil
	}
	// Prefer RO so we don't force a migrate/write when the user only queries.
	// Missing aws_obs table → empty list (ListAWSObs contract).
	db, err := store.OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		// Fall back to RW open (creates table via migrateExtras) then list.
		db, err = store.OpenWithContext(ctx, dbPath)
		if err != nil {
			return nil, err
		}
	}
	defer db.Close()
	rows, err := db.ListAWSObs(ctx, station, limit)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return []store.AWSObsRow{}, nil
	}
	return rows, nil
}
