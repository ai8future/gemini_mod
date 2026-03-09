module ai_gemini_mod

go 1.25.5

require (
	github.com/ai8future/chassis-go-addons/llm v0.0.0
	github.com/ai8future/chassis-go/v9 v9.0.0
)

replace github.com/ai8future/chassis-go/v9 => ../../chassis_suite/chassis-go

replace github.com/ai8future/chassis-go-addons/llm => ../../chassis_suite/chassis-go-addons/llm

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.40.0 // indirect
	go.opentelemetry.io/otel/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/trace v1.40.0 // indirect
)
