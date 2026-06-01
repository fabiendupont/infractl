// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newCapabilitiesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "capabilities",
		Short: "List server capabilities",
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := http.NewRequest("GET", server+"/api/v1/capabilities", nil)
			if err != nil {
				return err
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
			}

			if output == "json" {
				fmt.Println(string(body))
				return nil
			}
			if output == "yaml" {
				return jsonToYAML(body)
			}

			var caps map[string]string
			if err := json.Unmarshal(body, &caps); err != nil {
				return err
			}

			features := make([]string, 0, len(caps))
			for f := range caps {
				features = append(features, f)
			}
			sort.Strings(features)

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "FEATURE\tPROVIDER")
			for _, f := range features {
				fmt.Fprintf(w, "%s\t%s\n", f, caps[f])
			}
			w.Flush()

			return nil
		},
	}
}
