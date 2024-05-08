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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/dmitsh/node-resource-exporter/pkg/metrics"
)

func mainInternal() error {
	var port int
	var nodeLabels, resources string
	flag.IntVar(&port, "p", 8080, "Prometheus target port")
	flag.StringVar(&resources, "r", "", "Comma-separated list of tracked resource names")
	flag.StringVar(&nodeLabels, "l", "", "Comma-separated list of node label names to be passed onto metrics")
	flag.Parse()

	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	metric := metrics.New(strings.Split(nodeLabels, ","))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	promServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
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
			return startResourceSamplingLoop(ctx, kubeClient, strings.Split(resources, ","), metric)
		},
		func(err error) {
			log.Printf("Stopping sampling loop: %v", err)
			cancel()
			log.Printf("Stopped sampling loop")
		})

	return g.Run()
}

func startResourceSamplingLoop(ctx context.Context, kubeClient *kubernetes.Clientset, resources []string, metric *metrics.Metrics) error {
	defer log.Printf("Exited sampling loop")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			reportResourceUsage(ctx, kubeClient, resources, metric)

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func reportResourceUsage(ctx context.Context, kubeClient *kubernetes.Clientset, resources []string, metric *metrics.Metrics) {
	nodeList, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("ERROR: failed to list the nodes: %v", err)
		return
	}

	for _, node := range nodeList.Items {
		nodeLabelValues := make([]string, len(metric.NodeLabelNames))
		for i, name := range metric.NodeLabelNames {
			nodeLabelValues[i] = node.Labels[name]
		}

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
			labels := append([]string{node.Name, resource}, nodeLabelValues...)
			// get resource requests
			if v, ok := requests[corev1.ResourceName(resource)]; ok {
				val = v.AsApproximateFloat64()
			} else {
				val = 0
			}
			metric.NodeResourceRequests.WithLabelValues(labels...).Set(val)
			// get resource usage in percents
			if v, ok := node.Status.Allocatable[corev1.ResourceName(resource)]; ok {
				if allocatable := v.AsApproximateFloat64(); allocatable > 0 {
					metric.NodeResourceOccupancy.WithLabelValues(labels...).Set(val * 100.0 / allocatable)
				}
			}
			// get resource limits
			if v, ok := limits[corev1.ResourceName(resource)]; ok {
				val = v.AsApproximateFloat64()
			} else {
				val = 0
			}
			metric.NodeResourceLimits.WithLabelValues(labels...).Set(val)
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
