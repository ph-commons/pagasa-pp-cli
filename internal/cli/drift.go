// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ph-commons/pagasa-pp-cli/internal/pagasa"
)

type driftDay struct {
	CapturedAt string `json:"captured_at"`
	Date       string `json:"date"`
	MinC       int    `json:"min_c"`
	MaxC       int    `json:"max_c"`
	RainChance int    `json:"rain_chance_pct"`
}

type driftView struct {
	City      string     `json:"city"`
	Snapshots int        `json:"snapshots"`
	Series    []driftDay `json:"series"`
	Note      string     `json:"note,omitempty"`
}

func newNovelDriftCmd(flags *rootFlags) *cobra.Command {
	var flagCity string
	cmd := &cobra.Command{
		Use:         "drift",
		Short:       "How a city's forecast changed across recorded snapshots",
		Long:        "Compare a city's forecast across the local snapshots that `digest --city` persists over time, so you can see whether a given day's outlook is stabilizing or swinging. Returns an honest empty series when fewer than two snapshots for the city exist.",
		Example:     "  pagasa-pp-cli drift --city \"Metro Manila\" --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would compare city forecast across local snapshots")
				return nil
			}
			if strings.TrimSpace(flagCity) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--city is required"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			snaps, err := loadSnapshots(ctx, defaultDBPath("pagasa-pp-cli"))
			if err != nil {
				return err
			}
			view := driftView{City: flagCity}
			for _, s := range snaps {
				if len(s.Cities) == 0 {
					continue
				}
				var cities []pagasa.CityForecast
				if json.Unmarshal(s.Cities, &cities) != nil {
					continue
				}
				match := pickCity(cities, flagCity)
				if match == nil {
					continue
				}
				view.Snapshots++
				for _, d := range match.Days {
					view.Series = append(view.Series, driftDay{
						CapturedAt: s.CapturedAt, Date: d.Date,
						MinC: d.MinC, MaxC: d.MaxC, RainChance: d.RainChance,
					})
				}
			}
			if view.Snapshots < 2 {
				view.Note = "need at least 2 snapshots with this city to show drift; run 'pagasa-pp-cli digest --city \"" + flagCity + "\"' over time"
			}
			if machineOut(cmd, flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if view.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s — %d snapshots\n", view.City, view.Snapshots)
			for _, d := range view.Series {
				fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s  %d–%d°C  rain %d%%\n",
					d.CapturedAt, d.Date, d.MinC, d.MaxC, d.RainChance)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagCity, "city", "", "city to track across snapshots")
	return cmd
}
