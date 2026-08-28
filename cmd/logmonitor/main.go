package main

import (
	"LogMonitor/internal/config"
	"LogMonitor/internal/feishu"
	"LogMonitor/internal/monitor"
	"context"
	"flag"
	"log"

	"github.com/kardianos/service"
)

var version = "dev"

type program struct {
	monitor *monitor.Monitor
	cancel  context.CancelFunc
}

func (p *program) Start(s service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go func() {
		if err := p.monitor.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("monitor error: %v", err)
		}
	}()
	return nil
}

func (p *program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

func main() {
	configPath := flag.String("c", "config.yaml", "path to YAML configuration")
	foreground := flag.Bool("s", false, "run in foreground (console)")
	daemon := flag.Bool("d", false, "install and run as a background system service")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	client := feishu.NewClient(cfg.Feishu, cfg.Notification)
	app := monitor.New(cfg, client, log.Default())

	svcConfig := &service.Config{
		Name:        "LogMonitor",
		DisplayName: "LogMonitor",
		Description: "Cross-platform log monitoring with Feishu notifications",
		Arguments:   []string{"-s", "-c", *configPath},
	}

	prg := &program{monitor: app}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatalf("create service: %v", err)
	}

	if *daemon {
		if err := service.Control(s, "install"); err != nil {
			log.Printf("install skipped: %v", err)
		}
		if err := service.Control(s, "start"); err != nil {
			log.Fatalf("start service: %v", err)
		}
		log.Printf("LogMonitor installed and started as a background service")
		return
	}

	if *foreground {
		log.Printf("running in foreground")
	}
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
