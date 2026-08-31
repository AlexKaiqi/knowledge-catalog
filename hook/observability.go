package hook

// DispatchObserver is an optional observation seam. Hook delivery behavior is
// independent of whether a metrics runtime is installed.
type DispatchObserver func(phase, transport, outcome string)

func observeDispatch(observe DispatchObserver, phase, transport string, err error) {
	if observe == nil {
		return
	}
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	observe(phase, transport, outcome)
}
