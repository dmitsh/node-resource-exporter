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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	log "k8s.io/klog/v2"

	"github.com/dmitsh/node-resource-exporter/pkg/metrics"
)

var (
	port                  int
	nodeLabels, resources string
	resourceScores        metrics.ResourceScore
)

func main() {
	flag.IntVar(&port, "p", 8080, "Prometheus target port")
	flag.StringVar(&resources, "r", "", "Comma-separated list of tracked resource names")
	flag.StringVar(&nodeLabels, "l", "", "Comma-separated list of node label names to be passed onto metrics")

	log.InitFlags(nil)
	flag.Parse()

	if err := mainInternal(); err != nil {
		log.Errorf(err.Error())
		os.Exit(1)
	}
}

func mainInternal() error {
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	metric := metrics.New(strings.Split(nodeLabels, ","))
	resourceScores = *metrics.NewResourceScore()

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
			log.Infof("Starting Node Resource Exporter on port %d", port)
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
			return startResourceSamplingLoop(ctx, kubeClient, strings.Split(resources, ","), metric)
		},
		func(err error) {
			log.Infof("Stopping sampling loop: %v", err)
			cancel()
			log.Infof("Stopped sampling loop")
		})

	return g.Run()
}

func startResourceSamplingLoop(ctx context.Context, kubeClient *kubernetes.Clientset, resources []string, metric *metrics.Metrics) error {
	defer log.Infof("Exited sampling loop")
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
		log.Infof("ERROR: failed to list the nodes: %v", err)
		return
	}

	for _, node := range nodeList.Items {
		nodeLabelValues := make([]string, len(metric.NodeLabelNames))
		for i, name := range metric.NodeLabelNames {
			nodeLabelValues[i] = node.Labels[name]
		}

		pods, err := kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + node.Name})
		if err != nil {
			log.Infof("ERROR: failed to get pods for node %s: %v", node.Name, err)
			continue
		}

		requests := corev1.ResourceList{}
		limits := corev1.ResourceList{}

		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			for _, container := range pod.Spec.Containers {
				addResourceList(requests, container.Resources.Requests)
				addResourceList(limits, container.Resources.Limits)
			}
		}

		log.Infof("Total requests on node %s: %v", node.Name, requests)
		log.Infof("Total limits on node %s: %v", node.Name, limits)

		var val float64
		for _, resource := range resources {
			scoreLabels := append([]string{resource}, nodeLabelValues...)
			labels := append([]string{node.Name}, scoreLabels...)
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
					occ := val / allocatable
					score := resourceScores.Score(resource, occ)

					log.V(4).Infof("%s occupancy: %f score: %f", resource, occ, score)
					metric.NodeResourceOccupancy.WithLabelValues(labels...).Set(occ * 100.0)
					metric.NodeResourceScore.WithLabelValues(scoreLabels...).Set(score)
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
