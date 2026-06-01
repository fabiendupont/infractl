// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:          "infractl",
		Short:        "CLI for the infractl API server",
		SilenceUsage: true,
	}

	root.PersistentFlags().StringVar(&server, "server", "http://localhost:8080", "API server address")
	root.PersistentFlags().StringVar(&orgID, "org-id", "00000000-0000-0000-0000-000000000001", "organization ID")
	root.PersistentFlags().StringVarP(&output, "output", "o", "table", "output format: table, json, yaml")

	root.AddCommand(newMachinesCmd())
	root.AddCommand(newCapabilitiesCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
