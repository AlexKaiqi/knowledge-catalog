package hook

import "time"

// DispatchObserver is an optional observation seam. Hook delivery behavior is
// independent of whether a metrics runtime is installed.
type DispatchObserver func(phase, transport, outcome string, elapsed time.Duration)

func observeDispatch(observe DispatchObserver, phase, transport string, err error, elapsed time.Duration) {
	if observe == nil {
		return
	}
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	observe(phase, transport, outcome, elapsed)
}
