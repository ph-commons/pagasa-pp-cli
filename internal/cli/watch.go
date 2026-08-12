// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ph-commons/pagasa-pp-cli/internal/pagasa"
)

type watchView struct {
	Area          string              `json:"area"`
	StormActive   bool                `json:"storm_active"`
	StormName     string              `json:"storm_name,omitempty"`
	StormKind     string              `json:"storm_kind,omitempty"`
	Signal        int                 `json:"signal,omitempty"`
	SignalMatched bool                `json:"signal_matched"`
	AffectedAreas string              `json:"affected_areas,omitempty"`
	WindSignals   []pagasa.WindSignal `json:"wind_signals,omitempty"`
	SignalMap     string              `json:"signal_map,omitempty"`
	Bulletins     []string            `json:"bulletin_pdfs,omitempty"`
	Note          string              `json:"note"`
	Source        string              `json:"source"`
}

func newNovelWatchCmd(flags *rootFlags) *cobra.Command {
	var flagArea string
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Wind-signal state relevant to a locality (honest when unconfirmed)",
		Long: strings.Trim(`
Report whether a tropical cyclone is active and, when the bulletin HTML lists
wind signals, whether --area appears in any signal's affected areas (substring
match; highest signal wins). An empty match is "not confirmed," not "confirmed
clear" — area names may differ from the bulletin's province/locality list.
Falls back to signal-map URL + bulletin PDFs when HTML has no row for the area.
Independent pages are fetched in parallel.`, "\n"),
		Example:     "  pagasa-pp-cli watch --area Mandaluyong --json",
		Annotations: map[string]string{"mcp:read-only": "true", "mcp:open-world": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch synopsis and bulletin index")
				return nil
			}
			if strings.TrimSpace(flagArea) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--area is required"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			view := watchView{Area: flagArea, Source: "live"}

			paths := []pageFetch{
				{Path: "/weather", Required: false},
				{Path: "/tropical-cyclone/severe-weather-bulletin", Required: false},
			}
			bodies := fetchPages(ctx, c, paths)

			if bodies[0].Err == nil {
				if syn, ok := pagasa.ParseSynopsis(string(bodies[0].Body)); ok && syn.StormName != "" {
					view.StormActive = true
					view.StormName = syn.StormName
					view.StormKind = syn.StormKind
				}
			}
			if bodies[1].Err == nil {
				braw := string(bodies[1].Body)
				b := pagasa.ParseBulletin(braw)
				view.SignalMap = b.SignalMap
				view.Bulletins = b.PDFs
				if len(b.PDFs) > 0 {
					view.StormActive = true
				}
				detail := pagasa.ParseStormDetail(braw)
				view.WindSignals = detail.WindSignals
				if n, areas, ok := matchWindSignal(detail.WindSignals, flagArea); ok {
					view.Signal = n
					view.SignalMatched = true
					view.AffectedAreas = areas
				}
			}

			switch {
			case view.SignalMatched:
				view.Note = fmt.Sprintf("%s appears under TCWS Signal #%d", flagArea, view.Signal)
			case view.StormActive:
				view.Note = fmt.Sprintf("%s %q is active; %s not listed in HTML wind-signal table (not confirmed clear — check PDF/map)",
					view.StormKind, view.StormName, flagArea)
				if view.StormName == "" {
					view.Note = fmt.Sprintf("tropical cyclone bulletin active; %s not listed in HTML wind-signal table (not confirmed clear — check PDF/map)", flagArea)
				}
			default:
				view.Note = fmt.Sprintf("no wind signal is in effect for %s (no tropical cyclone active)", flagArea)
			}

			if machineOut(cmd, flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			fmt.Fprintln(cmd.OutOrStdout(), view.Note)
			if view.SignalMap != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "signal map: %s\n", view.SignalMap)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagArea, "area", "", "locality name to watch (e.g. Mandaluyong)")
	return cmd
}
