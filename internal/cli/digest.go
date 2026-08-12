// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ph-commons/pagasa-pp-cli/internal/pagasa"
)

type digestView struct {
	CapturedAt string               `json:"captured_at"`
	Synopsis   string               `json:"synopsis,omitempty"`
	StormName  string               `json:"storm_name,omitempty"`
	StormKind  string               `json:"storm_kind,omitempty"`
	City       *pagasa.CityForecast `json:"city_forecast,omitempty"`
	Bulletins  []string             `json:"bulletin_pdfs,omitempty"`
	Source     string               `json:"source"`
}

func newNovelDigestCmd(flags *rootFlags) *cobra.Command {
	var flagCity string
	cmd := &cobra.Command{
		Use:         "digest",
		Short:       "One payload: synopsis + a city's forecast + active-storm bulletins",
		Long:        "Compose the PAGASA synopsis, one city's multi-day forecast, and the active tropical-cyclone bulletins into a single agent-native payload — replacing three separate page scrapes. Fetches independent pages in parallel. Persists a local snapshot for history and drift.",
		Example: "  pagasa-pp-cli digest --city \"Metro Manila\" --json",
		// No mcp:read-only: persists a local snapshot for history/drift.
		// Not mcp:local-write (openWorldHint=false would lie). See #23 / #24.
		Annotations: map[string]string{"mcp:open-world": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch synopsis, city forecast, and bulletin index")
				return nil
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			view := digestView{CapturedAt: time.Now().UTC().Format(time.RFC3339), Source: "live"}

			paths := []pageFetch{
				{Path: "/weather", Required: true},
				{Path: "/tropical-cyclone/severe-weather-bulletin", Required: false},
			}
			if flagCity != "" {
				paths = append(paths, pageFetch{
					Path: "/weather/weather-outlook-selected-philippine-cities", Required: false,
				})
			}
			bodies := fetchPages(ctx, c, paths)
			if err := firstRequiredError(paths, bodies); err != nil {
				return classifyAPIError(err, flags)
			}

			if syn, ok := pagasa.ParseSynopsis(string(bodies[0].Body)); ok {
				view.Synopsis = syn.Text
				view.StormName = syn.StormName
				view.StormKind = syn.StormKind
			}
			if bodies[1].Err == nil {
				view.Bulletins = pagasa.ParseBulletin(string(bodies[1].Body)).PDFs
			}
			if flagCity != "" && len(bodies) > 2 && bodies[2].Err == nil {
				if match := pickCity(pagasa.ParseCityForecasts(string(bodies[2].Body)), flagCity); match != nil {
					view.City = match
				}
			}

			// Best-effort snapshot for history/drift. A cache-write failure must
			// not fail the live read; log to stderr and continue.
			var cityJSON json.RawMessage
			if view.City != nil {
				cityJSON, _ = json.Marshal([]pagasa.CityForecast{*view.City})
			}
			if err := saveSnapshot(ctx, defaultDBPath("pagasa-pp-cli"), snapshot{
				CapturedAt: view.CapturedAt, Synopsis: view.Synopsis,
				StormName: view.StormName, StormKind: view.StormKind, Cities: cityJSON,
			}); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not persist snapshot: %v\n", err)
			}

			if machineOut(cmd, flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if view.StormName != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Active: %s %q\n", view.StormKind, view.StormName)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", view.Synopsis)
			if view.City != nil {
				fmt.Fprintln(cmd.OutOrStdout())
				printCity(cmd, *view.City)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagCity, "city", "", "include this city's forecast in the digest")
	return cmd
}
