// sparrow
// (C) 2024, Deutsche Telekom IT GmbH
//
// Deutsche Telekom IT GmbH and all other contributors /
// copyright owners license this file to you under the Apache
// License, Version 2.0 (the "License"); you may not use this
// file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/caas-team/sparrow/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

var _ Provider = (*manager)(nil)

//go:generate moq -out metrics_moq.go . Provider
type Provider interface {
	// GetRegistry returns the prometheus registry instance
	// containing the registered prometheus collectors
	GetRegistry() *prometheus.Registry
	// InitTracing initializes the OpenTelemetry tracing
	InitTracing(ctx context.Context) error
	// Shutdown closes the metrics and tracing
	Shutdown(ctx context.Context) error
}

type manager struct {
	config   Config
	registry *prometheus.Registry
	tp       *sdktrace.TracerProvider
}

// New initializes the metrics and returns the PrometheusMetrics
//
//nolint:gocritic
func New(config Config) Provider {
	registry := prometheus.NewRegistry()

	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return &manager{
		config:   config,
		registry: registry,
	}
}

// GetRegistry returns the registry to register prometheus metrics
func (m *manager) GetRegistry() *prometheus.Registry {
	return m.registry
}

// InitTracing initializes the OpenTelemetry tracing
func (m *manager) InitTracing(ctx context.Context) error {
	log := logger.FromContext(ctx)
	res, err := resource.New(ctx,
		resource.WithHost(),
		resource.WithContainer(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String("sparrow-metrics-api"),
			// TODO: Maybe we should use the version that is set on build time in the main package
			semconv.ServiceVersionKey.String("0.1.0"),
		),
	)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create resource", "error", err)
		return fmt.Errorf("failed to create resource: %v", err)
	}

	exporter, err := m.config.Exporter.Create(ctx, &m.config)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create exporter", "error", err)
		return fmt.Errorf("failed to create exporter: %v", err)
	}

	const (
		batchTimeout = 5 * time.Second
		maxQueueSize = 1000
		maxBatchSize = 100
	)
	bsp := sdktrace.NewBatchSpanProcessor(exporter,
		sdktrace.WithBatchTimeout(batchTimeout),
		sdktrace.WithMaxQueueSize(maxQueueSize),
		sdktrace.WithMaxExportBatchSize(maxBatchSize),
	)
	tp := sdktrace.NewTracerProvider(
		// TODO: Keep track of the sampler if we run into traffic issues due to the high volume of data.
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	m.tp = tp
	log.DebugContext(ctx, "Tracing initialized with new provider", "provider", m.config.Exporter)
	return nil
}

// Shutdown closes the metrics and tracing
func (m *manager) Shutdown(ctx context.Context) error {
	log := logger.FromContext(ctx)
	if m.tp != nil {
		err := m.tp.Shutdown(ctx)
		if err != nil {
			log.ErrorContext(ctx, "Failed to shutdown tracer provider", "error", err)
			return fmt.Errorf("failed to shutdown tracer provider: %w", err)
		}
	}

	log.DebugContext(ctx, "Tracing shutdown")
	return nil
}
