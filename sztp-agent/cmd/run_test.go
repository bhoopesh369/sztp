// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2022-2023 Red Hat.

// Package cmd implements the CLI commands
package cmd

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestNewRunCommand(t *testing.T) {
	tests := []struct {
		name string
		want *cobra.Command
	}{
		{
			name: "TestNewRunCommand",
			want: &cobra.Command{
				Use:   "run",
				Short: "Exec the run command",
				RunE: func(c *cobra.Command, _ []string) error {
					err := c.Help()
					cobra.CheckErr(err)
					return nil
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewRunCommand(); !reflect.DeepEqual(got.Commands(), tt.want.Commands()) {
				t.Errorf("NewRunCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}
