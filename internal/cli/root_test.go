package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootShowsUsage(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "skiletto") {
		t.Errorf("help output does not mention skiletto:\n%s", out.String())
	}
}

func TestGlobalFlagIsPersistentAndInterspersed(t *testing.T) {
	for _, args := range [][]string{
		{"-g", "probe", "value"},
		{"probe", "-g", "value"},
		{"probe", "value", "-g"},
		{"group", "-g", "probe", "value"},
		{"group", "probe", "value", "--global"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd := newRootCmd()
			var gotGlobal bool
			probe := &cobra.Command{
				Use:  "probe <value>",
				Args: cobra.ExactArgs(1),
				RunE: func(cmd *cobra.Command, _ []string) error {
					var err error
					gotGlobal, err = cmd.Flags().GetBool("global")
					return err
				},
			}
			if args[0] == "group" {
				group := &cobra.Command{Use: "group"}
				group.AddCommand(probe)
				cmd.AddCommand(group)
			} else {
				cmd.AddCommand(probe)
			}
			cmd.SetArgs(args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute %v: %v", args, err)
			}
			if !gotGlobal {
				t.Errorf("global = false for %v", args)
			}
		})
	}
}

func TestRootShowsVersion(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), version) {
		t.Errorf("version output %q does not contain version %q", out.String(), version)
	}
	if !strings.Contains(out.String(), "skiletto") {
		t.Errorf("version output %q does not mention skiletto", out.String())
	}
}
