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
