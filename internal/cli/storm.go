// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ph-commons/pagasa-pp-cli/internal/pagasa"
)

type stormView struct {
	Active      bool                `json:"active"`
	StormName   string              `json:"storm_name,omitempty"`
	StormKind   string              `json:"storm_kind,omitempty"`
	Synopsis    string              `json:"synopsis,omitempty"`
	Bulletins   []string            `json:"bulletin_pdfs,omitempty"`
	SignalMap   string              `json:"signal_map,omitempty"`
	Location    string              `json:"location,omitempty"`
	LatDeg      float64             `json:"lat_deg,omitempty"`
	LonDeg      float64             `json:"lon_deg,omitempty"`
	Movement    string              `json:"movement,omitempty"`
	Strength    string              `json:"strength,omitempty"`
	MaxWindKmh  int                 `json:"max_wind_kmh,omitempty"`
	GustKmh     int                 `json:"gust_kmh,omitempty"`
	Forecast    []pagasa.TrackPoint `json:"forecast,omitempty"`
	WindSignals []pagasa.WindSignal `json:"wind_signals,omitempty"`
	Source      string              `json:"source"`
}

func newStormCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "storm",
		Short:       "Active tropical cyclone: position, intensity, movement, forecast, and wind signals",
		Long:        "Combine the PAGASA synopsis with the severe-weather-bulletin index so agents get the active cyclone name, center coordinates, intensity, movement, forecast track, bulletin PDFs, and per-locality wind-signal breakdown in one call. Independent pages are fetched in parallel. Reports active:false when no cyclone is being tracked.",
		Example:     "  pagasa-pp-cli storm --json",
		Annotations: map[string]string{"mcp:read-only": "true", "mcp:open-world": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch /weather and /tropical-cyclone/severe-weather-bulletin")
				return nil
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			view := stormView{Source: "live"}

			paths := []pageFetch{
				{Path: "/weather", Required: false},
				{Path: "/tropical-cyclone/severe-weather-bulletin", Required: true},
			}
			bodies := fetchPages(ctx, c, paths)
			if err := firstRequiredError(paths, bodies); err != nil {
				return classifyAPIError(err, flags)
			}

			if bodies[0].Err == nil {
				if syn, ok := pagasa.ParseSynopsis(string(bodies[0].Body)); ok {
					view.Synopsis = syn.Text
					view.StormName = syn.StormName
					view.StormKind = syn.StormKind
				}
			}

			raw := bodies[1].Body
			b := pagasa.ParseBulletin(string(raw))
			view.Bulletins = b.PDFs
			view.SignalMap = b.SignalMap
			view.Active = view.StormName != "" || len(b.PDFs) > 0

			detail := pagasa.ParseStormDetail(string(raw))
			view.Location = detail.Location
			view.LatDeg = detail.LatDeg
			view.LonDeg = detail.LonDeg
			view.Movement = detail.Movement
			view.Strength = detail.Strength
			view.MaxWindKmh = detail.MaxWindKmh
			view.GustKmh = detail.GustKmh
			view.Forecast = detail.Forecast
			view.WindSignals = detail.WindSignals

			if machineOut(cmd, flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if !view.Active {
				fmt.Fprintln(cmd.OutOrStdout(), "No tropical cyclone is currently being tracked.")
				return nil
			}
			if view.StormName != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %q\n\n%s\n\n", view.StormKind, view.StormName, view.Synopsis)
			}
			if view.Location != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  center:   %s\n", view.Location)
			}
			if view.Movement != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  movement: %s\n", view.Movement)
			}
			if view.Strength != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  strength: %s\n", view.Strength)
			}
			for _, t := range view.Forecast {
				fmt.Fprintf(cmd.OutOrStdout(), "  forecast: %s - %s\n", t.ValidAt, t.Position)
			}
			for _, p := range view.Bulletins {
				fmt.Fprintf(cmd.OutOrStdout(), "  bulletin: %s\n", p)
			}
			if view.SignalMap != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  signals:  %s\n", view.SignalMap)
			}
			for _, ws := range view.WindSignals {
				fmt.Fprintf(cmd.OutOrStdout(), "  signal %d: %s\n", ws.Signal, ws.AffectedAreas)
			}
			return nil
		},
	}
	return cmd
}
