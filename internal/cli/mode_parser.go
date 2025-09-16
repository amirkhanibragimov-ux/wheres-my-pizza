package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

const (
	ModeOrder   = "order-service"
	ModeTrack   = "tracking-service"
	ModeKitchen = "kitchen-worker"
	ModeNotify  = "notification-subscriber"
)

// isKnownMode checks if the provided mode name is known.
func isKnownMode(s string) (string, bool) {
	switch s {
	case ModeOrder, "order":
		return ModeOrder, true
	case ModeTrack, "tracking", "track":
		return ModeTrack, true
	case ModeKitchen, "kitchen":
		return ModeKitchen, true
	case ModeNotify, "notify":
		return ModeNotify, true
	default:
		return "", false
	}
}

// ParseMode supports:
//
//	--mode=<value>
//	<value> (subcommand shorthand), e.g., `order-service --port=3001`
func ParseMode(args []string) (string, []string, error) {
	var mode string
	var out []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--mode=") {
			mode = strings.TrimPrefix(arg, "--mode=")
			continue
		}

		if mode == "" {
			if m, ok := isKnownMode(arg); ok {
				mode = m
				continue
			}
		}
		out = append(out, arg)
	}

	if mode == "" {
		return "", out, nil
	}

	if m, ok := isKnownMode(mode); ok {
		mode = m
	}

	return mode, out, nil
}

// PrintUsage prints the usage information with examples.
func PrintUsage(w io.Writer) {
	fmt.Fprint(w, "\033[36m") // switch the color to cyan

	fmt.Fprintln(w, `Usage:
  ./restaurant-system --mode=<service> [flags]

Services (modes):
  order-service              HTTP API for placing orders
  kitchen-worker             RabbitMQ consumer that processes orders
  tracking-service           HTTP API for tracking orders/workers
  notification-subscriber    RabbitMQ subscriber that publishes notifications

Examples:
  ./restaurant-system --mode=order-service --port=3000 --max-concurrent=50
  ./restaurant-system --mode=kitchen-worker --worker-name=chef1 --order-types=dine_in --heartbeat-45 --prefetch=4
  ./restaurant-system --mode=tracking-service --port=3002
  ./restaurant-system --mode=notification-subscriber`)

	fmt.Fprint(w, "\033[0m") // switch back to normal
}

func AttachUsage(fs *flag.FlagSet, mode string) {
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ./restaurant-system --mode=%s [flags]\n", mode)
		fs.PrintDefaults()
	}
}
