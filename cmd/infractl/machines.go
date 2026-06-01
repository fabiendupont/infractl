// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type machineResource struct {
	OrgID           string            `json:"org_id" yaml:"org_id"`
	Name            string            `json:"name" yaml:"name"`
	Labels          map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	Generation      int64             `json:"generation" yaml:"generation"`
	ResourceVersion int64             `json:"resource_version" yaml:"resource_version"`
	Owner           *string           `json:"owner,omitempty" yaml:"owner,omitempty"`
	CreatedAt       time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at" yaml:"updated_at"`
	Spec            machineSpec       `json:"spec" yaml:"spec"`
	Status          machineStatus     `json:"status" yaml:"status"`
}

type machineSpec struct {
	Arch     string `json:"arch,omitempty" yaml:"arch,omitempty"`
	CPUs     int    `json:"cpus,omitempty" yaml:"cpus,omitempty"`
	MemoryMB int    `json:"memory_mb,omitempty" yaml:"memory_mb,omitempty"`
	DiskGB   int    `json:"disk_gb,omitempty" yaml:"disk_gb,omitempty"`
	BMCAddr  string `json:"bmc_addr,omitempty" yaml:"bmc_addr,omitempty"`
}

type machineStatus struct {
	Phase   string `json:"phase" yaml:"phase"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

type machineListResponse struct {
	Items    []machineResource `json:"items"`
	Continue string            `json:"continue,omitempty"`
	Total    int64             `json:"total"`
}

func newMachinesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "machines",
		Short: "Manage machines",
	}

	cmd.AddCommand(newMachinesListCmd())
	cmd.AddCommand(newMachinesGetCmd())
	cmd.AddCommand(newMachinesCreateCmd())
	cmd.AddCommand(newMachinesDeleteCmd())

	return cmd
}

func newMachinesListCmd() *cobra.Command {
	var limit int
	var filter string
	var continueToken string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List machines",
		RunE: func(cmd *cobra.Command, args []string) error {
			u, err := url.Parse(server + "/api/v1/machines")
			if err != nil {
				return err
			}
			q := u.Query()
			if limit > 0 {
				q.Set("limit", fmt.Sprintf("%d", limit))
			}
			if filter != "" {
				q.Set("filter", filter)
			}
			if continueToken != "" {
				q.Set("continue", continueToken)
			}
			u.RawQuery = q.Encode()

			req, err := http.NewRequest("GET", u.String(), nil)
			if err != nil {
				return err
			}
			req.Header.Set("X-Org-ID", orgID)

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

			var list machineListResponse
			if err := json.Unmarshal(body, &list); err != nil {
				return err
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tARCH\tCPUS\tMEMORY_MB\tPHASE\tAGE")
			for _, m := range list.Items {
				age := time.Since(m.CreatedAt).Truncate(time.Second)
				fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\t%s\n",
					m.Name, m.Spec.Arch, m.Spec.CPUs, m.Spec.MemoryMB, m.Status.Phase, age)
			}
			w.Flush()

			if list.Continue != "" {
				fmt.Fprintf(os.Stderr, "\nUse --continue=%s to fetch the next page\n", list.Continue)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of items to return")
	cmd.Flags().StringVar(&filter, "filter", "", "filter expression (field=value)")
	cmd.Flags().StringVar(&continueToken, "continue", "", "pagination token from a previous response")

	return cmd
}

func newMachinesGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Get a machine by name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			req, err := http.NewRequest("GET", server+"/api/v1/machines/"+url.PathEscape(name), nil)
			if err != nil {
				return err
			}
			req.Header.Set("X-Org-ID", orgID)

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

			var m machineResource
			if err := json.Unmarshal(body, &m); err != nil {
				return err
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintf(w, "Name:\t%s\n", m.Name)
			fmt.Fprintf(w, "OrgID:\t%s\n", m.OrgID)
			fmt.Fprintf(w, "Arch:\t%s\n", m.Spec.Arch)
			fmt.Fprintf(w, "CPUs:\t%d\n", m.Spec.CPUs)
			fmt.Fprintf(w, "MemoryMB:\t%d\n", m.Spec.MemoryMB)
			fmt.Fprintf(w, "DiskGB:\t%d\n", m.Spec.DiskGB)
			fmt.Fprintf(w, "BMCAddr:\t%s\n", m.Spec.BMCAddr)
			fmt.Fprintf(w, "Phase:\t%s\n", m.Status.Phase)
			fmt.Fprintf(w, "Message:\t%s\n", m.Status.Message)
			fmt.Fprintf(w, "Generation:\t%d\n", m.Generation)
			fmt.Fprintf(w, "ResourceVersion:\t%d\n", m.ResourceVersion)
			fmt.Fprintf(w, "CreatedAt:\t%s\n", m.CreatedAt.Format(time.RFC3339))
			fmt.Fprintf(w, "UpdatedAt:\t%s\n", m.UpdatedAt.Format(time.RFC3339))
			w.Flush()

			return nil
		},
	}
}

func newMachinesCreateCmd() *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a machine from a file",
		RunE: func(cmd *cobra.Command, args []string) error {
			var data []byte
			var err error

			if file == "-" {
				data, err = io.ReadAll(os.Stdin)
			} else {
				data, err = os.ReadFile(file)
			}
			if err != nil {
				return fmt.Errorf("reading input: %w", err)
			}

			// If the input looks like YAML, convert to JSON.
			var raw interface{}
			if err := yaml.Unmarshal(data, &raw); err == nil {
				if _, isMap := raw.(map[string]interface{}); isMap {
					data, err = json.Marshal(raw)
					if err != nil {
						return fmt.Errorf("converting YAML to JSON: %w", err)
					}
				}
			}

			req, err := http.NewRequest("POST", server+"/api/v1/machines", bytes.NewReader(data))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Org-ID", orgID)

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

			var m machineResource
			if err := json.Unmarshal(body, &m); err != nil {
				return err
			}
			fmt.Printf("machine/%s created\n", m.Name)
			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "file to read (use - for stdin)")
	cmd.MarkFlagRequired("file")

	return cmd
}

func newMachinesDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a machine by name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			req, err := http.NewRequest("DELETE", server+"/api/v1/machines/"+url.PathEscape(name), nil)
			if err != nil {
				return err
			}
			req.Header.Set("X-Org-ID", orgID)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
			}

			fmt.Printf("machine/%s deleted\n", name)
			return nil
		},
	}
}

func jsonToYAML(data []byte) error {
	var obj interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	out, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}
	fmt.Print(string(out))
	return nil
}
