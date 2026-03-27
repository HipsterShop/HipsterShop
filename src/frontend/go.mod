module github.com/GoogleCloudPlatform/microservices-demo/src/frontend

go 1.25.0

toolchain go1.26.1

require (
	cloud.google.com/go/compute/metadata v0.9.0
	cloud.google.com/go/profiler v0.4.3
	github.com/go-playground/validator/v10 v10.30.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.9.4
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.67.0
	go.opentelemetry.io/otel v1.42.0
)
