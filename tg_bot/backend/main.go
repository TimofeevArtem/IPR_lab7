package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"

	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var (
	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests",
		},
		[]string{"path", "method"},
	)

	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path"},
	)

	supportRequests = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "support_requests_total",
			Help: "Total support requests",
		},
	)
)

func init() {
	prometheus.MustRegister(httpRequests)
	prometheus.MustRegister(httpDuration)
	prometheus.MustRegister(supportRequests)
}

func initTracer() {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")

	if endpoint == "" {
		log.Println("OTLP endpoint not configured, tracing disabled")
		return
	}

	ctx := context.Background()

	exporter, err := otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)

	if err != nil {
		log.Printf("failed to create OTLP exporter: %v", err)
		return
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.Empty()),
	)

	otel.SetTracerProvider(tracerProvider)

	log.Println("OpenTelemetry tracing enabled")
}

func metricsMiddleware(path string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		httpRequests.WithLabelValues(path, r.Method).Inc()

		next(w, r)

		duration := time.Since(start).Seconds()

		httpDuration.WithLabelValues(path).Observe(duration)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"status":  "healthy",
		"service": "backend",
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(response)
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	supportRequests.Inc()

	response := map[string]string{
		"message": "stats collected",
		"service": "backend",
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(response)
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"message": "Hello from Kubernetes Backend!",
		"service": "backend",
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(response)
}

func main() {
	initTracer()

	http.Handle("/metrics", promhttp.Handler())

	http.Handle(
		"/health",
		otelhttp.NewHandler(
			metricsMiddleware("/health", healthHandler),
			"health-handler",
		),
	)

	http.Handle(
		"/api/stats",
		otelhttp.NewHandler(
			metricsMiddleware("/api/stats", statsHandler),
			"stats-handler",
		),
	)

	http.Handle(
		"/api",
		otelhttp.NewHandler(
			metricsMiddleware("/api", apiHandler),
			"api-handler",
		),
	)

	port := "8080"

	if os.Getenv("PORT") != "" {
		port = os.Getenv("PORT")
	}

	fmt.Printf("Backend started on :%s\n", port)

	log.Fatal(http.ListenAndServe(":"+port, nil))
}