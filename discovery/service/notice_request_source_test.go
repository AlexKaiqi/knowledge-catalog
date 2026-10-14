package service

import (
	"context"
	"reflect"
	"testing"

	index "kc/discovery/projection"
	"kc/kernel"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"
)

type requestNoticeController struct {
	*controllerPort
	notice index.ChangeNotice
	source index.ObservationSource
}

func (c *requestNoticeController) ApplyNotice(_ context.Context, notice index.ChangeNotice, source index.ObservationSource) (index.StateSync, error) {
	c.calls = append(c.calls, "apply-local-notice")
	c.notice, c.source = notice, source
	return index.StateSync{Repository: notice.Repository, BasisCommit: "fixed-during-notice", Revision: "notice-revision"}, nil
}

func TestProjectionNoticeExecutorPassesRequestSourceOnlyToLocalNotice(t *testing.T) {
	repo := &operationsRepository{id: "kr://app/local-notice", head: "advanced-after-notice"}
	controller := &requestNoticeController{controllerPort: &controllerPort{}}
	state := stateLookupPort{}
	request := objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "request-user"}, RequestID: "request-7"}
	notice := index.ChangeNotice{Repository: repo.ID(), SourceRevision: "source-r7"}
	result, err := (ProjectionNoticeExecutor{Repositories: operationsLookup{repo: repo}, Projection: &projectionPort{}, Controller: controller}).Execute(context.Background(), ProjectionNoticeRequest{Notice: notice, State: state, Request: request})
	if err != nil || result.Repository != notice.Repository || result.BasisCommit != "fixed-during-notice" || result.Revision != "notice-revision" {
		t.Fatalf("local result lost frozen notice basis: %#v %v", result, err)
	}
	if !reflect.DeepEqual(controller.calls, []string{"apply-local-notice"}) || !reflect.DeepEqual(controller.notice, notice) {
		t.Fatalf("request source entered global configuration or catch-up: %#v %#v", controller.calls, controller.notice)
	}
	source, ok := controller.source.(sourcebridge.Source)
	if !ok || source.Lookup != state || !reflect.DeepEqual(source.Request, request) {
		t.Fatalf("request-local runtime/correlation was lost: %#v", controller.source)
	}
}
