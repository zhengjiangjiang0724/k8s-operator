/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package metrics defines Prometheus metrics exposed by the WebApp operator.
//
// Naming conventions follow Prometheus / OpenMetrics best practices:
//   - counters end with `_total`;
//   - durations end with `_seconds` and use Histogram;
//   - gauges have no suffix;
//   - all metrics share the `webapp_` prefix for easy filtering.
//
// All metrics are registered against controller-runtime's metrics registry
// (sigs.k8s.io/controller-runtime/pkg/metrics), so they are scraped from the
// same `/metrics` endpoint as the built-in controller-runtime metrics
// (workqueue depth, reconcile rate, etc.). Both Prometheus and
// VictoriaMetrics scrape this endpoint natively.
//
// Label cardinality is intentionally bounded: we never use pod IPs, UIDs,
// or generation numbers as labels. Cardinality scales as
// O(num_webapps × num_unique_phases × num_unique_images).
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Subsystem prefix for every metric in this package. Combined with the
// metric name, the exposed series look like: webapp_reconcile_total{...}.
const subsystem = "webapp"

// Standard histogram buckets for sub-second operations (SSA, status patch).
// Range: 1 ms — 10 s, log-spaced. Picked to capture both fast in-cluster
// API calls and pathological long-tail latencies without exploding
// cardinality.
var fastOpBuckets = []float64{
	0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}

// ---------------------------------------------------------------------------
// Reconcile loop metrics
// ---------------------------------------------------------------------------

var (
	// ReconcileTotal counts every reconcile invocation, partitioned by result.
	//
	// Use: alert on (rate of errors) / (rate of all) > threshold.
	// PromQL: sum(rate(webapp_reconcile_total{result="error"}[5m]))
	//       / sum(rate(webapp_reconcile_total[5m]))
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "reconcile_total",
			Help:      "Total number of reconcile invocations, partitioned by result (success|error).",
		},
		[]string{"name", "namespace", "result"},
	)

	// ReconcileDuration tracks per-reconcile wall-clock latency.
	//
	// `result` is one of: success | error. Separating them lets you alert
	// on error-path latency (which often hides retries) without polluting
	// the happy-path histogram.
	ReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: subsystem,
			Name:      "reconcile_duration_seconds",
			Help:      "Duration of WebApp reconcile loop in seconds.",
			Buckets:   fastOpBuckets,
		},
		[]string{"name", "namespace", "result"},
	)

	// ReconcileErrors is kept for backward compatibility with the original
	// alerting rules. Prefer ReconcileTotal{result="error"} for new queries.
	ReconcileErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "reconcile_errors_total",
			Help:      "Total number of reconcile errors (DEPRECATED: use reconcile_total{result=\"error\"}).",
		},
		[]string{"name", "namespace"},
	)
)

// ---------------------------------------------------------------------------
// Sub-resource operation metrics
// ---------------------------------------------------------------------------

var (
	// SubresourceOperations counts SSA applies and deletes against owned
	// sub-resources (Deployment / Service / Ingress).
	//
	// `operation`: apply | delete
	// `kind`:      Deployment | Service | Ingress
	// `result`:    success | error
	//
	// Use: detect a sudden spike in apply errors per kind, indicating
	// API Server / RBAC / admission webhook regressions.
	SubresourceOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "subresource_operations_total",
			Help:      "Total operations performed on owned sub-resources.",
		},
		[]string{"kind", "operation", "result"},
	)

	// SubresourceOperationDuration measures latency of a single API call
	// to a sub-resource (SSA Patch / Delete). This excludes the time spent
	// building the desired object — that is part of ReconcileDuration.
	SubresourceOperationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: subsystem,
			Name:      "subresource_operation_duration_seconds",
			Help:      "Latency of API calls to owned sub-resources.",
			Buckets:   fastOpBuckets,
		},
		[]string{"kind", "operation"},
	)
)

// ---------------------------------------------------------------------------
// Webhook metrics
// ---------------------------------------------------------------------------

var (
	// WebhookRequests counts admission webhook invocations.
	//
	// `webhook`:   mutating | validating
	// `operation`: create | update | delete (admission verb)
	// `result`:    allow | deny | error
	WebhookRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "webhook_requests_total",
			Help:      "Total number of admission webhook invocations.",
		},
		[]string{"webhook", "operation", "result"},
	)

	// WebhookDuration measures admission webhook handler latency.
	// Important because the API Server blocks on this synchronously:
	// slow webhooks impact every kubectl apply against WebApp CRs.
	WebhookDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: subsystem,
			Name:      "webhook_duration_seconds",
			Help:      "Latency of admission webhook handlers.",
			// Webhook handlers should be sub-millisecond; tighter buckets.
			Buckets: []float64{
				0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1,
			},
		},
		[]string{"webhook", "operation"},
	)
)

// ---------------------------------------------------------------------------
// Per-WebApp state gauges
// ---------------------------------------------------------------------------

var (
	// WebAppInfo emits a constant 1 per WebApp with descriptive labels.
	// Useful for joining other metrics with current state via PromQL:
	//   sum by (namespace) (webapp_info{phase="Failed"})
	WebAppInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "info",
			Help:      "Constant 1 per WebApp with current state labels.",
		},
		[]string{"name", "namespace", "phase", "image"},
	)

	// WebAppReplicasDesired tracks the spec'd replica count.
	WebAppReplicasDesired = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "replicas_desired",
			Help:      "Desired number of replicas for WebApp.",
		},
		[]string{"name", "namespace"},
	)

	// WebAppReplicasReady tracks the observed ready replica count.
	// Combined with WebAppReplicasDesired, this drives availability SLIs:
	//   sum by (namespace) (webapp_replicas_ready)
	// / sum by (namespace) (webapp_replicas_desired)
	WebAppReplicasReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "replicas_ready",
			Help:      "Ready number of replicas for WebApp.",
		},
		[]string{"name", "namespace"},
	)

	// WebAppPhase emits a 1 for the current phase of each WebApp and 0
	// for all other phases. Lets you count objects per phase efficiently:
	//   sum by (phase) (webapp_phase)
	WebAppPhase = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "phase",
			Help:      "Per-phase indicator (1 = current phase, 0 = other).",
		},
		[]string{"name", "namespace", "phase"},
	)
)

// AllPhases lists every phase value emitted as a WebAppPhase label. Used
// by the controller to zero out non-current phases.
var AllPhases = []string{
	"Pending", "Creating", "Running", "Updating", "Failed", "Deleting",
}

func init() {
	metrics.Registry.MustRegister(
		ReconcileTotal,
		ReconcileDuration,
		ReconcileErrors,
		SubresourceOperations,
		SubresourceOperationDuration,
		WebhookRequests,
		WebhookDuration,
		WebAppInfo,
		WebAppReplicasDesired,
		WebAppReplicasReady,
		WebAppPhase,
	)
}
