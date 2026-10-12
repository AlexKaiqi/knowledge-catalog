package server

import (
	"context"
	"encoding/json"
	"strings"

	"go.opentelemetry.io/otel/trace"

	"kc/identity"
	inv "kc/invocation"
	"kc/invoke"
	"kc/kernel"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
	snapshotservice "kc/snapshot/service"
)

// snapshotStoreService supplies application-owned resources and policy to the
// independent Snapshot Store HTTP surface. It never forwards those requests
// to an application command executor or duplicates its Dataset/file engine.
func (f *httpFacade) snapshotStoreService() *snapshotservice.Handler {
	return snapshotservice.NewHandler(snapshotservice.Config{
		Identity: f.snapshotIdentityOptions(),
		Acquire:  f.acquireSnapshotResources,
		Observe:  f.observeSnapshotOperation,
		Around:   f.aroundSnapshotOperation,
	})
}

func (f *httpFacade) snapshotIdentityOptions() snapshotservice.IdentityOptions {
	options := snapshotservice.IdentityOptions{
		Mode: f.options.AuthModeLabel(), Local: f.options.localAssertion(),
		Authenticator: f.options.Authenticator,
	}
	if login := f.browserLoginConfig(); login != nil {
		options.BrowserLogin = login
	}
	return options
}

func snapshotOperationAction(operation string) (string, error) {
	switch operation {
	case "catalog-list", "catalog-repo-list", "dataset-list", "dataset-show":
		return "catalog.read", nil
	case "dataset-define", "dataset-retire":
		return "dataset.manage", nil
	case "pin", "pin-check", "pin-source", "file-mounts", "file-list", "file-read":
		return "dataset.resolve", nil
	case "admin-grant-list":
		return "admin.grants.read", nil
	case "admin-grant-add", "admin-grant-remove":
		return "admin.grants.manage", nil
	case "admission-show", "whoami":
		return "identity.read", nil
	default:
		return "", kernel.Fail(kernel.ErrUsageInvalid, "unknown Snapshot Store operation %s", operation)
	}
}

func (f *httpFacade) acquireSnapshotResources(ctx context.Context, caller identity.HTTPIdentity, operation string) (snapshotservice.Resources, func(), error) {
	action, err := snapshotOperationAction(operation)
	if err != nil {
		return snapshotservice.Resources{}, nil, err
	}
	unlock := f.lockTypedInvocation(action)
	opened, err := f.readHomeForRequest()
	if err != nil {
		return snapshotservice.Resources{}, unlock, err
	}
	flags := snapshotRequestFlags(ctx, caller, access.Query{Action: action})
	flags["home"] = opened.Dir
	requestJournal, err := inv.RequestHomeJournal(opened.Dir, operation, flags)
	if err != nil {
		return snapshotservice.Resources{}, unlock, err
	}
	readOnly := inv.TypedInvocationReadOnly(action)
	immutable := readOnly && opened.Deployment != nil
	if immutable {
		opened = opened.ReadView(requestJournal)
	} else {
		opened.SetJournal(requestJournal)
	}
	if operation == "catalog-list" {
		if err := inv.AuthorizeCatalogInventory(opened.Dir, flags, f.snapshotAuthorizationObserver(ctx, operation), opened); err != nil {
			return snapshotservice.Resources{}, unlock, err
		}
	}
	if lease, ok := ctx.Value(snapshotLeaseKey{}).(*snapshotLease); ok {
		lease.opened = opened
	}
	return f.snapshotResources(opened, caller, operation, readOnly, immutable), unlock, nil
}

func (f *httpFacade) snapshotResources(opened *Home, caller identity.HTTPIdentity, operation string, readOnly, immutable bool) snapshotservice.Resources {
	resources := snapshotservice.Resources{
		Catalogs: snapshotCatalogProvider{opened: opened}, Sources: opened.Store,
		Access: &access.Service{Store: snapshotGrantStore{home: opened.Dir, writable: !readOnly}},
	}
	if opened.Deployment != nil && opened.Deployment.Admission != nil {
		resources.RequestURL = opened.Deployment.Admission.RequestURL
	}
	resources.Authorize = func(ctx context.Context, request snapshotservice.Authorization) error {
		if request.Caller.Principal != caller.Principal || request.Caller.OnBehalfOf != caller.OnBehalfOf {
			return kernel.Fail(kernel.ErrUnauthenticated, "Snapshot Store authorization caller changed within one lease")
		}
		return f.authorizeSnapshotRequest(ctx, opened, operation, immutable, request)
	}
	resources.DescribeRepository = func(_ context.Context, _ identity.HTTPIdentity, id kernel.RepositoryID) (map[string]any, error) {
		return inv.DescribeCatalogRepository(opened, id), nil
	}
	resources.BeforePublish = func(ctx context.Context, caller identity.HTTPIdentity, cat *dataset.Catalog, definition dataset.KnowledgeSet) (dataset.KnowledgeSet, map[string]any, error) {
		flags := snapshotRequestFlags(ctx, caller, access.Query{Action: "dataset.manage", Catalog: cat.DumpState().CatalogID, Dataset: definition.SetID})
		flags["home"] = opened.Dir
		flags["revision"] = definition.Revision
		prepared, err := inv.PublishKnowledgeSetRecipe(&inv.Invocation{
			Home: opened.Dir, WS: opened, Context: ctx, Flags: flags, Command: operation,
		}, definition)
		if err != nil || prepared == nil {
			return definition, nil, err
		}
		receipt := map[string]any{}
		for name, value := range map[string]string{
			"recipeFile": prepared.File, "recipeRepository": prepared.Repository,
			"recipeCommit": prepared.Commit, "recipeLocation": prepared.Location,
			"recipeSkipped": prepared.Skipped,
		} {
			if value != "" {
				receipt[name] = value
			}
		}
		return definition, receipt, nil
	}
	resources.PreparePublication = func(ctx context.Context, _ identity.HTTPIdentity, definition dataset.KnowledgeSet) error {
		return opened.PrepareDataset(ctx, definition)
	}
	resources.EnsureDataset = func(_ context.Context, _ identity.HTTPIdentity, cat *dataset.Catalog, id string) error {
		_, err := inv.EnsureWorkspace(opened, opened.Dir, cat, id)
		return err
	}
	resources.RecordRead = func(ctx context.Context, caller identity.HTTPIdentity, evidence snapshotservice.FileReadEvidence) error {
		return f.snapshotRecordFileRead(ctx, opened, caller, evidence)
	}
	resources.OpenView = func(ctx context.Context, caller identity.HTTPIdentity, coordinate snapshotservice.FileCoordinate, cat *dataset.Catalog, definition dataset.KnowledgeSet, pin dataset.ResolvedKnowledgeSet) (*snapshotservice.FileView, error) {
		return f.snapshotSemanticView(ctx, opened, caller, coordinate, cat, definition, pin)
	}
	return resources
}

func (f *httpFacade) authorizeSnapshotRequest(ctx context.Context, opened *Home, operation string, immutable bool, request snapshotservice.Authorization) error {
	flags := snapshotRequestFlags(ctx, request.Caller, request.Query)
	flags["home"] = opened.Dir
	// The legacy API permits an omitted Catalog. A declared deployment's
	// default is already in the borrowed Home, rather than fixture home.json.
	if request.Query.Catalog == "" && len(opened.File.Catalogs) > 0 {
		flags["_default-catalog"] = opened.File.Catalogs[0].ID
	}
	if request.Definition != nil {
		flags[inv.WorkspaceDefinitionFlag] = request.Definition
	}
	if request.Pin != nil {
		raw, err := json.Marshal(request.Pin)
		if err != nil {
			return err
		}
		flags["pin"] = string(raw)
	}
	if request.CatalogDiscovery {
		flags[inv.CatalogDiscoveryFlag] = true
		if err := inv.PrepareCatalogDiscovery(opened, request.Query.Action, flags); err != nil {
			return err
		}
	}
	var err error
	switch {
	case request.Query.Action == "file.read" && request.Definition != nil && request.Pin != nil:
		err = inv.RequireCompleteWorkspaceRead(opened.Dir, flags, inv.KnowledgeSetPin(*request.Pin), request.Query.Object)
	case request.Query.Action == "file.read" && request.Query.Repo != "":
		var allowed bool
		allowed, err = inv.DatasetFSMayReadRepository(opened.Dir, flags, request.Query.Repo, opened)
		if err == nil && !allowed {
			err = kernel.Fail(kernel.ErrForbidden, "%s is not allowed to file.read", request.Caller.Principal)
		}
	default:
		err = inv.Authorize(opened.Dir, request.Query.Action, flags, nil, opened)
	}
	f.snapshotAuthorizationObserver(ctx, operation)(authorizationDecision(err))
	if err != nil {
		return err
	}
	if immutable {
		requestJournal, err := inv.RequestHomeJournal(opened.Dir, operation, flags)
		if err != nil {
			return err
		}
		// Only the private Catalog/Reader instances may be stamped on a read
		// lease. Home.ReadView deliberately borrows the shared Writers.
		if opened.Reader != nil {
			opened.Reader.SetJournal(requestJournal)
		}
		for _, cat := range opened.Catalogs {
			if cat != nil {
				cat.SetJournal(requestJournal)
			}
		}
	} else {
		inv.ObserveHome(opened, request.Query.Action, flags)
	}
	return nil
}

func authorizationDecision(err error) string {
	if err != nil {
		return "deny"
	}
	return "allow"
}

func (f *httpFacade) snapshotAuthorizationObserver(ctx context.Context, operation string) invoke.AuthorizationObserver {
	return func(decision string) {
		if f.runtime != nil {
			f.runtime.RecordAuthorization(ctx, operation, decision)
		}
	}
}

func snapshotRequestFlags(ctx context.Context, caller identity.HTTPIdentity, query access.Query) map[string]FlagValue {
	metadata := snapshotservice.ContextMetadata(ctx)
	flags := compactFlags(map[string]FlagValue{
		"as": caller.Principal, "on-behalf-of": caller.OnBehalfOf,
		"auth-provider": caller.Provider, "auth-subject": caller.Subject, "auth-login": caller.Login,
		"_action": query.Action, "repo": query.Repo, "catalog": query.Catalog,
		"dataset": query.Dataset, "ref": query.Ref, "object": query.Object, "aspect": query.Aspect,
		"request-id": metadata.RequestID, "trace-id": metadata.TraceID,
		"span-id": metadata.SpanID, "parent-span-id": metadata.ParentSpanID,
	})
	if caller.User != nil {
		flags["_identity-provider"] = caller.User.Provider
		flags["_identity-issuer"] = caller.User.Issuer
		flags["_identity-subject"] = caller.User.Subject
	}
	if state, ok := ctx.Value(httpTelemetryContextKey{}).(httpTelemetryContext); ok && !state.UseLegacyTrace {
		current := trace.SpanContextFromContext(ctx)
		if current.IsValid() {
			flags["trace-id"], flags["span-id"] = current.TraceID().String(), current.SpanID().String()
		} else {
			flags["trace-id"], flags["span-id"] = state.TraceID, state.SpanID
		}
		if state.ParentSpanID != "" {
			flags["parent-span-id"] = state.ParentSpanID
		} else if state.SpanID != "" {
			flags["parent-span-id"] = state.SpanID
		}
	}
	return flags
}

func (f *httpFacade) observeSnapshotOperation(ctx context.Context, caller identity.HTTPIdentity, operation string) (context.Context, func(any, error)) {
	recordHTTPIdentity(f.runtime, ctx, f.options, caller)
	ctx = context.WithValue(ctx, snapshotLeaseKey{}, &snapshotLease{})
	if f.runtime == nil {
		return ctx, nil
	}
	group := inv.TelemetryFace(operation)
	ctx, span, started := f.runtime.StartOperation(ctx, group, operation)
	return ctx, func(result any, err error) {
		outcome, errorType := inv.TelemetryResult(err)
		if strings.HasPrefix(operation, "file-") {
			bytes, entries := vfsVolume(result)
			f.runtime.RecordVFSVolume(ctx, operation, outcome, bytes, entries)
		}
		f.runtime.EndOperation(ctx, span, started, group, operation, outcome, errorType)
	}
}

// The private holder connects Acquire to the external hook aspect without
// making Home part of the Snapshot module's ports. Its lifetime is one request,
// and the facade lock remains held until the module releases its resources.
type snapshotLease struct{ opened *Home }
type snapshotLeaseKey struct{}

func (f *httpFacade) aroundSnapshotOperation(ctx context.Context, caller identity.HTTPIdentity, operation string, run func() (any, error)) (any, error) {
	lease, ok := ctx.Value(snapshotLeaseKey{}).(*snapshotLease)
	if !ok || lease.opened == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Snapshot Store hook aspect requires acquired resources")
	}
	action, err := snapshotOperationAction(operation)
	if err != nil {
		return nil, err
	}
	metadata := snapshotservice.ContextMetadata(ctx)
	flags := snapshotRequestFlags(ctx, caller, access.Query{Action: action, Catalog: metadata.Catalog, Dataset: metadata.Dataset})
	flags["home"] = lease.opened.Dir
	observation := invoke.NewObservationTelemetry(ctx, f.runtime, operation, func() string { return inv.TelemetryProvider(flags) })
	return inv.WithHooks(lease.opened, lease.opened.Dir, action, flags, observation, run)
}

type snapshotCatalogProvider struct{ opened *Home }

func (p snapshotCatalogProvider) CatalogIDs() []string {
	ids := make([]string, 0, len(p.opened.File.Catalogs))
	for _, catalog := range p.opened.File.Catalogs {
		ids = append(ids, catalog.ID)
	}
	return ids
}

func (p snapshotCatalogProvider) OpenCatalog(id string) (*dataset.Catalog, error) {
	cat, _, err := p.opened.UseCatalog(id)
	return cat, err
}

// snapshotGrantStore preserves the application's complete allow.json ledger.
// The facade holds its write lock across UpdateRules and the HTTP operation,
// so a module update cannot drop private admission/provisioning receipts or
// race another HTTP policy mutation.
type snapshotGrantStore struct {
	home     string
	writable bool
}

func (s snapshotGrantStore) ReadRules(ctx context.Context) ([]access.Rule, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := inv.ReadAllow(s.home)
	return file.Rules, err
}

func (s snapshotGrantStore) UpdateRules(ctx context.Context, update func([]access.Rule) ([]access.Rule, error)) error {
	if !s.writable {
		return kernel.Fail(kernel.ErrPreconditionFailed, "grant updates require a write lease")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := inv.ReadAllow(s.home)
	if err != nil {
		return err
	}
	rules, err := update(file.Rules)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	file.Rules = rules
	return inv.WriteAllow(s.home, file)
}
