package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
)

// ProcessFlags validate and return the flag values.
func ProcessFlags() (string, error) {
	portFlag := flag.String("port", "8080", "Port number")
	helpFlag := flag.Bool("help", false, "Show this screen")

	flag.Usage = func() {
		printUsage()
		os.Exit(1)
	}

	flag.Parse()

	if len(flag.Args()) != 0 {
		return "", errors.New("unexpected arguments detected. Make sure you follow the format")
	}

	// Show help if the --help flag is passed
	if *helpFlag {
		if len(os.Args) != 2 {
			return "", errors.New("the --help flag must be used alone")
		}
		printUsage()
		os.Exit(0)
	}

	if isValid, err := isValidPort(*portFlag); !isValid {
		return "", err
	}

	return *portFlag, nil
}

// isValidPort validate the port number.
func isValidPort(portFlag string) (bool, error) {
	port, err := strconv.Atoi(portFlag)
	if err != nil || port < 1024 || port > 65535 {
		return false, fmt.Errorf("invalid port number %v. Only ports between 1024-65535 are allowed", port)
	}

	return true, nil
}

// printUsage prints the usage information.
func printUsage() {
	fmt.Println(`
Usage:
	wheres-my-pizza [--port <N>]
	wheres-my-pizza --help

Options:
	--port N     Port number`)
}
