package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	nodeResourceRequests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "node_resource_requests",
			Help: "Gauge of node resource requests.",
		},
		[]string{"node", "resource"})

	nodeResourceLimits = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "node_resource_limits",
			Help: "Gauge of node resource limits.",
		},
		[]string{"node", "resource"})

	nodeResourceUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "node_resource_usage",
			Help: "Usage of node resource in percents.",
		},
		[]string{"node", "resource"})
)

func mainInternal() error {
	var port int
	var resources string
	flag.IntVar(&port, "p", 8080, "Prometheus target port")
	flag.StringVar(&resources, "r", "", "Comma-separated list of tracked resource names")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	promServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	var g run.Group
	// Signal handler
	g.Add(run.SignalHandler(ctx, os.Interrupt, syscall.SIGTERM))
	// Prometheus target
	g.Add(
		func() error {
			log.Printf("Starting Node Resource Exporter on port %d", port)
			return promServer.ListenAndServe()
		},
		func(err error) {
			log.Printf("Stopping Node Resource Exporter: %v", err)
			if err := promServer.Shutdown(ctx); err != nil {
				log.Printf("Error during server shutdown: %v", err)
			}
			log.Printf("Stopped Node Resource Exporter")
		})
	// Resource sampling loop
	g.Add(
		func() error {
			log.Printf("Starting sampling loop")
			return startResourceSamplingLoop(ctx, kubeClient, strings.Split(resources, ","))
		},
		func(err error) {
			log.Printf("Stopping sampling loop: %v", err)
			cancel()
			log.Printf("Stopped sampling loop")
		})

	return g.Run()
}

func startResourceSamplingLoop(ctx context.Context, kubeClient *kubernetes.Clientset, resources []string) error {
	defer log.Printf("Exited sampling loop")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			reportResourceUsage(ctx, kubeClient, resources)

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func reportResourceUsage(ctx context.Context, kubeClient *kubernetes.Clientset, resources []string) {
	nodeList, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("ERROR: failed to list the nodes: %v", err)
		return
	}

	for _, node := range nodeList.Items {
		pods, err := kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + node.Name})
		if err != nil {
			log.Printf("ERROR: failed to get pods for node %s: %v", node.Name, err)
			continue
		}

		requests := corev1.ResourceList{}
		limits := corev1.ResourceList{}

		for _, pod := range pods.Items {
			for _, container := range pod.Spec.Containers {
				addResourceList(requests, container.Resources.Requests)
				addResourceList(limits, container.Resources.Limits)
			}
		}

		log.Printf("Total requests on node %s: %v", node.Name, requests)
		log.Printf("Total limits on node %s: %v", node.Name, limits)

		var val float64
		for _, resource := range resources {
			// get resource requests
			if v, ok := requests[corev1.ResourceName(resource)]; ok {
				val = v.AsApproximateFloat64()
			} else {
				val = 0
			}
			nodeResourceRequests.WithLabelValues(node.Name, resource).Set(val)
			// get resource usage in percents
			if v, ok := node.Status.Allocatable[corev1.ResourceName(resource)]; ok {
				if allocatable := v.AsApproximateFloat64(); allocatable > 0 {
					nodeResourceUsage.WithLabelValues(node.Name, resource).Set(val * 100.0 / allocatable)
				}
			}
			// get resource limits
			if v, ok := limits[corev1.ResourceName(resource)]; ok {
				val = v.AsApproximateFloat64()
			} else {
				val = 0
			}
			nodeResourceLimits.WithLabelValues(node.Name, resource).Set(val)
		}
	}
}

func addResourceList(total, addition corev1.ResourceList) {
	for resourceName, quantity := range addition {
		if curr, found := total[resourceName]; found {
			curr.Add(quantity)
			total[resourceName] = curr
		} else {
			total[resourceName] = quantity.DeepCopy()
		}
	}
}

func main() {
	if err := mainInternal(); err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}
}
