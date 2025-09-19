package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/cmd/kitchenworker"
	"git.platform.alem.school/amibragim/wheres-my-pizza/cmd/notificationservice"
	"git.platform.alem.school/amibragim/wheres-my-pizza/cmd/orderservice"
	"git.platform.alem.school/amibragim/wheres-my-pizza/cmd/trackingservice"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/cli"
)

func main() {
	// check for help flag first
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		cli.PrintUsage(os.Stdout)
		os.Exit(0)
	}

	// parse all command-line arguments
	mode, svcArgs, err := cli.ParseMode(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		cli.PrintUsage(os.Stderr)
		os.Exit(2)
	}

	// ensure that mode is not empty
	if mode == "" {
		cli.PrintUsage(os.Stderr)
		os.Exit(2)
	}

	// create context cancelled on SIGINT/SIGTERM signals ensuring graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// run the service specified by the mode flag
	switch mode {
	case cli.ModeOrder:
		fs := flag.NewFlagSet(cli.ModeOrder, flag.ContinueOnError)
		port := fs.Int("port", 3000, "HTTP port for the API")
		maxConc := fs.Int("max-concurrent", 50, "Maximum number of concurrent orders to process")
		cli.AttachUsage(fs, cli.ModeOrder)

		if err := fs.Parse(svcArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(2)
		}

		if *port <= 0 || *port > 65535 {
			fmt.Fprintln(os.Stderr, "Error: --port must be between 1 and 65535")
			fs.Usage()
			os.Exit(2)
		}
		if *maxConc <= 0 {
			fmt.Fprintln(os.Stderr, "Error: --max-concurrent must be > 0")
			fs.Usage()
			os.Exit(2)
		}

		if err := orderservice.Run(ctx, *port, *maxConc); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

	case cli.ModeKitchen:
		fs := flag.NewFlagSet(cli.ModeKitchen, flag.ContinueOnError)
		workerName := fs.String("worker-name", "", "Unique name for the worker (required)")
		orderTypes := fs.String("order-types", "", "Comma-separated list of order types (e.g. dine_in,takeout). If omitted, handles all")
		heartbeat := fs.Int("heartbeat-interval", 30, "Heartbeat interval in seconds")
		prefetch := fs.Int("prefetch", 1, "RabbitMQ prefetch count")
		cli.AttachUsage(fs, cli.ModeKitchen)

		if err := fs.Parse(svcArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(2)
		}

		if *workerName == "" {
			fmt.Fprintln(os.Stderr, "Error: --worker-name is required")
			fs.Usage()
			os.Exit(2)
		}
		if *heartbeat <= 0 {
			fmt.Fprintln(os.Stderr, "Error: --heartbeat-interval must be > 0")
			fs.Usage()
			os.Exit(2)
		}
		if *prefetch <= 0 {
			fmt.Fprintln(os.Stderr, "Error: --prefetch must be > 0")
			fs.Usage()
			os.Exit(2)
		}

		if err := kitchenworker.Run(ctx, *workerName, orderTypes, *heartbeat, *prefetch); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

	case cli.ModeTrack:
		fs := flag.NewFlagSet(cli.ModeTrack, flag.ContinueOnError)
		port := fs.Int("port", 3002, "HTTP port for the API")
		cli.AttachUsage(fs, cli.ModeTrack)

		if err := fs.Parse(svcArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(2)
		}

		if *port <= 0 || *port > 65535 {
			fmt.Fprintln(os.Stderr, "Error: --port must be between 1 and 65535")
			fs.Usage()
			os.Exit(2)
		}

		if err := trackingservice.Run(ctx, *port); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

	case cli.ModeNotify:
		if err := notificationservice.Run(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	}

	select {
	case <-ctx.Done():
	case <-time.After(10 * time.Millisecond):
	}
}
