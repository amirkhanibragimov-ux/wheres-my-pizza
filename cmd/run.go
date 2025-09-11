package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"git.platform.alem.school/amibragim/wheres-my-pizza/config"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/adapter/postgres"
)

func Run() {
	// initialize the config setup
	cfg, err := config.LoadFromFile("config/config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	// create a context that is canceled when an interrupt or termination signal is received
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		log.Fatal("db connect: %v", err)
	}
	defer pool.Close()

	fmt.Println(cfg)
}
