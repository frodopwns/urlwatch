package cmd

import (
	"fmt"
	"log"

	"github.com/frodopwns/urlwatch/pkg/watchops"
	"github.com/spf13/cobra"
)

var (
	urls     []string
	interval string
	timeout  string
	port     int
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "starts the url watching service",
	Long:  `creates a concurrent thread for each url provided and sends a GET request every interval.`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// ensure that at least one url has been provided
		if len(urls) == 0 {
			return fmt.Errorf("must provide urls to watch")
		}

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("starting watcher service")
		ctx := cmd.Context()

		// initialize wanter manager service to track concurrent watcher services
		watcherman, err := watchops.NewWatcherManager(interval, timeout, port)
		if err != nil {
			log.Fatalf("failed creating watcher manager: %v", err)
		}

		fmt.Println("watching urls:")
		for _, u := range urls {
			fmt.Println(u)
			// register each url with the manager
			watcherman.AddWatcher(ctx, u)
		}

		// start watching urls and serving metrics
		watcherman.WaitAndWatch(ctx)
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.Flags().StringSliceVar(&urls, "url", []string{}, "provide each url that needs to be tracked")
	startCmd.Flags().StringVarP(&interval, "interval", "i", "30s", "how long to wait between checks")
	startCmd.Flags().StringVarP(&timeout, "timeout", "t", "2s", "how long to wait before cancelling a connectino request")
	startCmd.Flags().IntVarP(&port, "port", "p", 80, "port the metrics will be servied from")
}
