package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func main() {
	var upper bool
	var times int
	root := &cobra.Command{Use: "tool", Short: "A tool", SilenceUsage: true}
	greet := &cobra.Command{
		Use:   "greet [name...]",
		Short: "Greet people",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for i := 0; i < times; i++ {
				s := "hello " + strings.Join(args, ", ")
				if upper {
					s = strings.ToUpper(s)
				}
				fmt.Fprintln(cmd.OutOrStdout(), s)
			}
			return nil
		},
	}
	greet.Flags().BoolVarP(&upper, "upper", "u", false, "uppercase")
	greet.Flags().IntVarP(&times, "times", "n", 1, "repeat")
	fail := &cobra.Command{Use: "fail", RunE: func(cmd *cobra.Command, args []string) error { return fmt.Errorf("boom") }}
	root.AddCommand(greet, fail)
	root.PersistentFlags().String("config", "", "config file")
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
