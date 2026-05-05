package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/agendash/AgenLeash/internal/app"
)

func main() {
	if err := app.BootstrapEnvironment(); err != nil {
		log.Fatalf("load environment: %v", err)
	}

	cfg := app.LoadConfig()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc, err := app.New(cfg)
	if err != nil {
		log.Fatalf("initialize app: %v", err)
	}

	fmt.Fprintf(os.Stdout, "agenleash listening on %s\n", cfg.Addr)
	if cfg.Token == "" {
		fmt.Fprintln(os.Stdout, "agenleash authentication disabled via AGENLEASH_ALLOW_NO_TOKEN=true")
	} else {
		fmt.Fprintln(os.Stdout, "agenleash authentication enabled")
	}
	if cfg.EnableWeb {
		fmt.Fprintln(os.Stdout, "agenleash web dashboard enabled via AGENLEASH_ENABLE_WEB=true")
	} else {
		fmt.Fprintln(os.Stdout, "agenleash web dashboard disabled by default")
	}

	if err := svc.Run(ctx); err != nil {
		log.Fatalf("run app: %v", err)
	}
}
