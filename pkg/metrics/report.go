package metrics

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	log "k8s.io/klog/v2"
)

func ReportResourceUsage(ctx context.Context, kubeClient *kubernetes.Clientset, resources, nodeLabels []string) {
	nodeList, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Infof("ERROR: failed to list the nodes: %v", err)
		return
	}

	for _, node := range nodeList.Items {
		nodeLabelValues := make([]string, len(nodeLabels))
		for i, name := range nodeLabels {
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
			nodeResourceRequests.WithLabelValues(labels...).Set(val)
			// get resource usage in percents
			if v, ok := node.Status.Allocatable[corev1.ResourceName(resource)]; ok {
				if allocatable := v.AsApproximateFloat64(); allocatable > 0 {
					occ := val / allocatable

					log.V(4).Infof("%s occupancy: %f", resource, occ)
					nodeResourceOccupancy.WithLabelValues(labels...).Set(occ * 100.0)
				}
			}
			// get resource limits
			if v, ok := limits[corev1.ResourceName(resource)]; ok {
				val = v.AsApproximateFloat64()
			} else {
				val = 0
			}
			nodeResourceLimits.WithLabelValues(labels...).Set(val)
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
