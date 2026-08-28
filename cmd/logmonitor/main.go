package main

import (
	"LogMonitor/internal/config"
	"LogMonitor/internal/feishu"
	"LogMonitor/internal/monitor"
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML configuration")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	client := feishu.NewClient(cfg.Feishu, cfg.Notification)
	app := monitor.New(cfg, client, log.Default())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("starting LogMonitor with %d source(s)", len(cfg.LogSources))
	if err := app.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
