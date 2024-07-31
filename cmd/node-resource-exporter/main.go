package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	log "k8s.io/klog/v2"

	"github.com/dmitsh/node-resource-exporter/pkg/metrics"
)

type args struct {
	port                  int
	nodeLabels, resources string
}

func main() {
	var args args
	flag.IntVar(&args.port, "p", 8080, "Prometheus target port")
	flag.StringVar(&args.resources, "r", "", "Comma-separated list of tracked resource names")
	flag.StringVar(&args.nodeLabels, "l", "", "Comma-separated list of node label names to be passed onto metrics")

	log.InitFlags(nil)
	flag.Parse()

	if err := mainInternal(&args); err != nil {
		log.Errorf(err.Error())
		os.Exit(1)
	}
}

func mainInternal(args *args) error {
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	nodeLabels := strings.Split(args.nodeLabels, ",")
	metrics.New(nodeLabels)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	promServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", args.port),
		Handler: mux,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var g run.Group
	// Signal handler
	g.Add(run.SignalHandler(ctx, os.Interrupt, syscall.SIGTERM))
	// Prometheus target
	g.Add(
		func() error {
			log.Infof("Starting Node Resource Exporter on port %d", args.port)
			return promServer.ListenAndServe()
		},
		func(err error) {
			log.Infof("Stopping Node Resource Exporter: %v", err)
			if err := promServer.Shutdown(ctx); err != nil {
				log.Infof("Error during server shutdown: %v", err)
			}
			log.Infof("Stopped Node Resource Exporter")
		})
	// Resource sampling loop
	g.Add(
		func() error {
			log.Infof("Starting sampling loop")
			return startResourceSamplingLoop(ctx, kubeClient, strings.Split(args.resources, ","), nodeLabels)
		},
		func(err error) {
			log.Infof("Stopping sampling loop: %v", err)
			cancel()
			log.Infof("Stopped sampling loop")
		})

	return g.Run()
}

func startResourceSamplingLoop(ctx context.Context, kubeClient *kubernetes.Clientset, resources, nodeLabels []string) error {
	defer log.Infof("Exited sampling loop")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics.ReportResourceUsage(ctx, kubeClient, resources, nodeLabels)

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
