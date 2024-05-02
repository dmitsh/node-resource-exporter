# node-resource-exporter

The Node Resource Exporter is a Prometheus tool designed to gather and report statistics related to resource usage at the node level. It takes a predefined list of resources and generates three types of metrics: cumulative resource requests, cumulative resource limits, and resource usage as a percentage. The percentage is calculated by comparing allocatable resources to cumulative resource requests. Each metric is tagged with both the node and the resource name for easy identification.

For instance, to monitor GPU usage, you can use the following PromQL query:
```
sum(node_resource_utilization{resource="nvidia.com/gpu"}) by (node)
```

If you need to determine the total time (in seconds) during which resource usage was below 100%, you can use this query:
```
sum_over_time(
  (sum(node_resource_utilization{resource="nvidia.com/gpu"}) by (node) < bool 100)[24h:15s]) * 15
```
