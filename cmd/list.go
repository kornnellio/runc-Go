package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"runc-go/container"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ps"},
	Short:   "List containers",
	Long:    `List containers managed by this runtime.`,
	Args:    cobra.NoArgs,
	RunE:    runList,
}

var (
	listQuiet  bool
	listFormat string
)

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().BoolVarP(&listQuiet, "quiet", "q", false, "display only container IDs")
	listCmd.Flags().StringVarP(&listFormat, "format", "f", "table", "output format (table, json)")
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	containers, err := container.List(ctx, GetStateRoot())
	if err != nil {
		return err
	}

	if listQuiet {
		for _, c := range containers {
			fmt.Println(c.ID)
		}
		return nil
	}

	if listFormat == "json" {
		return outputJSON(containers)
	}

	return outputTable(containers)
}

func outputTable(containers []*container.Container) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPID\tSTATUS\tBUNDLE\tCREATED")

	for _, c := range containers {
		created := c.State.Created.Format("2006-01-02 15:04:05")
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n",
			c.ID, c.State.Pid, c.State.Status, c.Bundle, created)
	}

	return w.Flush()
}

func outputJSON(containers []*container.Container) error {
	type listItem struct {
		ID      string `json:"id"`
		Pid     int    `json:"pid"`
		Status  string `json:"status"`
		Bundle  string `json:"bundle"`
		Created string `json:"created"`
	}

	items := make([]listItem, len(containers))
	for i, c := range containers {
		items[i] = listItem{
			ID:      c.ID,
			Pid:     c.State.Pid,
			Status:  string(c.State.Status),
			Bundle:  c.Bundle,
			Created: c.State.Created.Format("2006-01-02T15:04:05Z"),
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(items)
}
