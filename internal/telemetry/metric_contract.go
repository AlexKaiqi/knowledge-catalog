package telemetry

// requestDurationSecondsBuckets is the stable histogram resolution for HTTP,
// application operations, workspace resolution and complete SEARCH latency.
// The dense 0.75s-4s region brackets the local/shared SEARCH objectives and
// prevents histogram_quantile from interpolating a 1.2s request as ~2.5s.
var requestDurationSecondsBuckets = []float64{
	.001, .005, .01, .025, .05, .1, .25, .5,
	.75, 1, 1.25, 1.5, 2, 2.5, 3, 4, 5, 10,
}

// searchPhaseDurationSecondsBuckets keeps sub-millisecond probe/evidence-scale
// resolution while using the same SLO-adjacent resolution for slow phases.
var searchPhaseDurationSecondsBuckets = []float64{
	.0001, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5,
	.75, 1, 1.25, 1.5, 2, 2.5, 3, 4, 5, 10,
}

var countBuckets = []float64{0, 1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 10_000, 100_000, 1_000_000}

var byteBuckets = []float64{0, 1 << 10, 4 << 10, 16 << 10, 64 << 10, 256 << 10, 1 << 20, 4 << 20, 16 << 20, 64 << 20}

var observationAgeSecondsBuckets = []float64{0, 1, 5, 10, 30, 60, 300, 900, 3600, 21600, 86400}
