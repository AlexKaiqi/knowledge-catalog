package hook

import (
	"encoding/json"
	"errors"

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
	file, err := Read(home)
	if err != nil {
		return err
	}
	var persistenceErr error
	event.Phase = phase
	if phase == PhasePost {
		persistenceErr = FlushOutbox(home)
	}
	matched := file.Match(event.Action, phase, event.Repo, event.Catalog)
	for _, b := range matched {
		if err := deliver(home, b, event); err != nil {
			if phase == PhasePre {
				return err
			}
			if appendErr := AppendOutbox(home, b, event, err); appendErr != nil {
				persistenceErr = errors.Join(persistenceErr, appendErr)
			}
		}
	}
	return persistenceErr
}

func Pre(home string, event Event) error {
	return Dispatch(home, PhasePre, event)
}

func Post(home string, event Event) error {
	return Dispatch(home, PhasePost, event)
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
