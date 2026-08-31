module github.com/DataDog/datadog-pgo

go 1.25.0

require (
	github.com/DataDog/datadog-pgo/pgo v0.0.0
	github.com/lmittmann/tint v1.0.4
	github.com/mattn/go-isatty v0.0.21
)

require (
	github.com/google/pprof v0.0.0-20240227163752-401108e1b7e7 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.9.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
)

replace github.com/DataDog/datadog-pgo/pgo => ./pgo
