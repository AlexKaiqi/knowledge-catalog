package hook

import (
	"encoding/json"
	"errors"
	"time"

	"kc/kernel"
)

// Event is the stdin/HTTP body. Post events carry pointers only, not object bodies.
type Event struct {
	Action      string `json:"action"`
	Phase       string `json:"phase"`
	As          string `json:"as,omitempty"`
	Repo        string `json:"repo,omitempty"`
	Catalog     string `json:"catalog,omitempty"`
	CommandID   string `json:"commandId,omitempty"`
	Receipt     string `json:"receipt,omitempty"`
	NewCommit   string `json:"newCommit,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	Disposition string `json:"disposition,omitempty"`
}

func (e Event) JSON() ([]byte, error) {
	return json.Marshal(e)
}

func Dispatch(home, phase string, event Event) error {
	return DispatchObserved(home, phase, event, nil)
}

func DispatchObserved(home, phase string, event Event, observe DispatchObserver) error {
	file, err := Read(home)
	if err != nil {
		return err
	}
	var persistenceErr error
	event.Phase = phase
	if phase == PhasePost {
		persistenceErr = FlushOutboxObserved(home, observe)
	}
	matched := file.Match(event.Action, phase, event.Repo, event.Catalog)
	for _, b := range matched {
		transport := "other"
		if b.Run != "" {
			transport = "exec"
		} else if b.URL != "" {
			transport = "http"
		}
		started := time.Now()
		deliveryErr := deliver(home, b, event)
		observeDispatch(observe, phase, transport, deliveryErr, time.Since(started))
		if deliveryErr != nil {
			if phase == PhasePre {
				return deliveryErr
			}
			if appendErr := AppendOutbox(home, b, event, deliveryErr); appendErr != nil {
				persistenceErr = errors.Join(persistenceErr, appendErr)
			}
		}
	}
	return persistenceErr
}

func Pre(home string, event Event) error {
	return Dispatch(home, PhasePre, event)
}

func PreObserved(home string, event Event, observe DispatchObserver) error {
	return DispatchObserved(home, PhasePre, event, observe)
}

func Post(home string, event Event) error {
	return Dispatch(home, PhasePost, event)
}

func PostObserved(home string, event Event, observe DispatchObserver) error {
	return DispatchObserved(home, PhasePost, event, observe)
}

func deliver(home string, b Binding, event Event) error {
	body, err := event.JSON()
	if err != nil {
		return err
	}
	if b.Run != "" {
		return runExec(home, b.Run, body)
	}
	if b.URL != "" {
		return postURL(b.URL, body)
	}
	return kernel.Fail(kernel.ErrUsageInvalid, "hook %s has no --run or --url", b.ID)
}
