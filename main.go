package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/acquia/sumo-bridge/pkg/bridge"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	opsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bridge_counter",
		Help: "The total number of processed events",
	}, []string{"name"})
)

func main() {
	var (
		addr     = flag.String("addr", "", "Sumologic collector url")
		interval = flag.Duration("interval", time.Duration(6*time.Second), "interval to submit metrics to Sumologic")
	)

	flag.Parse()
	reg := prometheus.NewRegistry()
	reg.MustRegister(opsProcessed)

	if addr == nil {
		log.Println("Please provide  `-addr` flag")
		os.Exit(1)
	}

	bridge, err := bridge.NewBridge(&bridge.Config{
		URL:           *addr,
		Gatherer:      reg,
		ErrorHandling: bridge.AbortOnError,
		Logger:        log.New(os.Stdout, "bridge: ", log.Lshortfile),
		Interval:      *interval,
	})

	if err != nil {
		log.Fatal(err)
	}

	ctx, closer := signalCtx()
	defer closer()

	go recordMetrics(ctx)

	fmt.Println("Starting bridge")
	bridge.Run(ctx)
}

func signalCtx() (context.Context, func()) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-signalChan:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

func recordMetrics(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// just to make unique name by current time
			name := fmt.Sprintf("%d-%d", time.Now().Hour(), time.Now().Minute())
			opsProcessed.WithLabelValues(name).Inc()
		case <-ctx.Done():
			return
		}
	}
}
