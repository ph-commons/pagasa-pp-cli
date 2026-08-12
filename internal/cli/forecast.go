// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ph-commons/pagasa-pp-cli/internal/pagasa"
)

func newForecastCmd(flags *rootFlags) *cobra.Command {
	var city string
	var listCities bool
	cmd := &cobra.Command{
		Use:   "forecast",
		Short: "5-day forecast for a selected Philippine city (temp + rain chance)",
		Long:  "Fetch the PAGASA Weather Outlook for Selected Philippine Cities and return the multi-day forecast for one city, or all cities. Use --list-cities to see valid names.",
		Example: strings.Trim(`
  pagasa-pp-cli forecast --city "Metro Manila" --json
  pagasa-pp-cli forecast --list-cities
  pagasa-pp-cli forecast --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "mcp:open-world": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch /weather/weather-outlook-selected-philippine-cities")
				return nil
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			raw, err := c.Get(ctx, "/weather/weather-outlook-selected-philippine-cities", nil)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			cities := pagasa.ParseCityForecasts(string(raw))
			if len(cities) == 0 {
				return fmt.Errorf("no city forecasts found; PAGASA may have restructured the page")
			}

			if listCities {
				names := make([]string, 0, len(cities))
				for _, cf := range cities {
					names = append(names, cf.City)
				}
				sort.Strings(names)
				if machineOut(cmd, flags) {
					return printJSONFiltered(cmd.OutOrStdout(), names, flags)
				}
				for _, n := range names {
					fmt.Fprintln(cmd.OutOrStdout(), n)
				}
				return nil
			}

			if city != "" {
				match := pickCity(cities, city)
				if match == nil {
					return fmt.Errorf("city %q not found; run 'pagasa-pp-cli forecast --list-cities'", city)
				}
				if machineOut(cmd, flags) {
					return printJSONFiltered(cmd.OutOrStdout(), match, flags)
				}
				printCity(cmd, *match)
				return nil
			}

			if machineOut(cmd, flags) {
				return printJSONFiltered(cmd.OutOrStdout(), cities, flags)
			}
			for _, cf := range cities {
				printCity(cmd, cf)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&city, "city", "", "city name (case-insensitive substring, e.g. \"Metro Manila\")")
	cmd.Flags().BoolVar(&listCities, "list-cities", false, "list the valid city names and exit")
	return cmd
}

// pickCity matches on case-insensitive substring so "manila" finds "Metro Manila".
func pickCity(cities []pagasa.CityForecast, q string) *pagasa.CityForecast {
	ql := strings.ToLower(strings.TrimSpace(q))
	for i := range cities {
		if strings.Contains(strings.ToLower(cities[i].City), ql) {
			return &cities[i]
		}
	}
	return nil
}

func printCity(cmd *cobra.Command, cf pagasa.CityForecast) {
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", cf.City)
	for _, d := range cf.Days {
		fmt.Fprintf(cmd.OutOrStdout(), "  %-9s %-14s %2d–%2d°C  rain %d%%  %s\n",
			d.Day, d.Date, d.MinC, d.MaxC, d.RainChance, d.Condition)
	}
}
