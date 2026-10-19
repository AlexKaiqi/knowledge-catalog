package server

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"

	kchome "kc/home"
	"kc/identity"
	"kc/internal/journal"
	inv "kc/invocation"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
	snapshotservice "kc/snapshot/service"
)

// snapshotStoreService supplies application-owned resources and policy to the
// independent Snapshot Store HTTP surface. It never forwards those requests
// to an application command executor or duplicates its Dataset/file engine.
func (f *httpFacade) snapshotStoreService() snapshotservice.Handler {
	return snapshotservice.NewHandler(snapshotservice.Config{
		Identity:              f.snapshotIdentityOptions(),
		NativeOrigin:          f.options.NativeAPIOrigin,
		NativeCredential:      f.snapshotNativeCredential(),
		NativeManagedCreation: f.deployment != nil && (f.deployment.ManagedRepositories != nil || len(f.deployment.ManagedStores) > 0),
		Acquire:               f.acquireSnapshotResources,
		Observe:               f.observeSnapshotOperation,
		Around:                f.aroundSnapshotOperation,
	})
}

func (f *httpFacade) snapshotNativeCredential() func(context.Context) (string, error) {
	// The implicit local seam (HTTPHandler(home)) has no deployment and no
	// private storage credential; an explicit --auth local or a verifying
	// pairing uses the operator-supplied one. Caller proof is never forwarded.
	if f.options.authenticator() == nil || strings.TrimSpace(f.options.AuthMode) == "" && !f.options.Authenticated() {
		return nil
	}
	return func(context.Context) (string, error) {
		return os.Getenv(snapshot.LakeFSCredentialEnv), nil
	}
}

func (f *httpFacade) snapshotIdentityOptions() snapshotservice.IdentityOptions {
	options := snapshotservice.IdentityOptions{Mode: f.options.AuthModeLabel(), Authenticator: f.options.authenticator()}
	if login := f.browserLoginConfig(); login != nil {
		options.BrowserLogin = login
	}
	if f.deployment != nil {
		options.ProviderURL = strings.TrimSuffix(strings.TrimRight(f.deployment.AuthURL, "/"), "/oauth2")
		options.LogoutURL = f.deployment.AuthLogoutURL
	}
	return options
}

func snapshotOperationAction(operation string) (string, error) {
	if action, ok := strings.CutPrefix(operation, "native:"); ok && access.IsRepositoryAction(action) {
		return action, nil
	}
	switch operation {
	case "inventory-show", "repository-list", "dataset-list", "dataset-show":
		return "inventory.read", nil
	case "registry-audit":
		return "registry.audit.read", nil
	case "repository-attach", "detach":
		return "repositories.manage", nil
	case "dataset-define", "dataset-retire":
		return "dataset.manage", nil
	case "pin", "pin-check", "pin-source", "file-mounts", "file-list", "file-read":
		return "dataset.resolve", nil
	case "admin-grant-list":
		return "admin.grants.read", nil
	case "admin-grant-add", "admin-grant-remove", "admin-grant-update":
		return "admin.grants.manage", nil
	case "admission-show", "whoami":
		return "identity.read", nil
	default:
		return "", kernel.Fail(kernel.ErrUsageInvalid, "unknown Snapshot Store operation %s", operation)
	}
}

func (f *httpFacade) acquireSnapshotResources(ctx context.Context, caller identity.HTTPIdentity, operation string) (snapshotservice.Resources, func(), error) {
	if strings.TrimSpace(caller.Principal) == "" {
		return snapshotservice.Resources{}, nil, kernel.Fail(kernel.ErrUnauthenticated, "a verified Server principal is required")
	}
	action, err := snapshotOperationAction(operation)
	if err != nil {
		return snapshotservice.Resources{}, nil, err
	}
	release := f.enterRequest()
	opened, err := f.readHomeForRequest()
	if err != nil {
		return snapshotservice.Resources{}, release, err
	}
	flags := snapshotRequestFlags(ctx, caller, access.Query{Action: action})
	flags["home"] = opened.Dir
	requestJournal, err := inv.RequestHomeJournal(opened.Dir, operation, flags)
	if err != nil {
		return snapshotservice.Resources{}, release, err
	}
	readOnly := inv.TypedInvocationReadOnly(action)
	immutable := readOnly && opened.Deployment != nil
	// The served Home is shared by concurrent leases; every lease stamps only
	// its own view. Version conflicts are decided by the authority's CAS.
	if immutable {
		opened = opened.ReadView(requestJournal)
	} else {
		opened = opened.RequestView(requestJournal, "", "", "")
	}
	if operation == "inventory-show" || operation == "repository-list" || operation == "dataset-list" || operation == "dataset-show" {
		if err := inv.AuthorizeInventory(opened.Dir, flags, f.snapshotAuthorizationObserver(ctx, operation), opened); err != nil {
			return snapshotservice.Resources{}, release, err
		}
	}
	if lease, ok := ctx.Value(snapshotLeaseKey{}).(*snapshotLease); ok {
		lease.opened = opened
	}
	return f.snapshotResources(opened, caller, operation, readOnly, immutable), release, nil
}

func (f *httpFacade) snapshotResources(opened *Home, caller identity.HTTPIdentity, operation string, readOnly, immutable bool) snapshotservice.Resources {
	resources := snapshotservice.Resources{
		Datasets: opened.Datasets, Sources: opened.Store,
		Access: access.NewService(snapshotGrantStore{home: opened.Dir, writable: !readOnly}),
	}
	if strings.HasPrefix(operation, "native:") {
		resources.NativeRepositories = snapshotNativeRepositories(opened.BindingFile().Repos, f.options.NativeAPIOrigin)
		resources.CreateNativeRepository = f.nativeRepositoryCreator(opened)
	}
	if opened.Deployment != nil && opened.Deployment.Admission != nil {
		resources.RequestURL = opened.Deployment.Admission.RequestURL
	}
	resources.Authorize = func(ctx context.Context, request snapshotservice.Authorization) error {
		if request.Caller.Principal != caller.Principal || request.Caller.OnBehalfOf != caller.OnBehalfOf {
			return kernel.Fail(kernel.ErrUnauthenticated, "Snapshot Store authorization caller changed within one lease")
		}
		if operation == "detach" && request.Query.Repo == string(objectmodel.SystemRepositoryID) {
			return kernel.Fail(kernel.ErrForbidden, "System Repository %s cannot be detached", request.Query.Repo)
		}
		return f.authorizeSnapshotRequest(ctx, opened, operation, immutable, request)
	}
	resources.DiscoveryWorkspaceID = inv.ConfiguredDiscoveryWorkspace(opened)
	resources.BeforeAttach = func(_ context.Context, _ identity.HTTPIdentity, id kernel.RepositoryID) error {
		return opened.ValidateRepositoryAttachment(id)
	}
	resources.DescribeRepository = func(_ context.Context, caller identity.HTTPIdentity, id kernel.RepositoryID) (map[string]any, error) {
		view := inv.DescribeRepository(opened, id)
		if repository := snapshotConsoleRepository(opened.BindingFile().Repos, caller, id, f.options.NativeAPIOrigin); repository != "" {
			view["consoleRepository"] = repository
		}
		return view, nil
	}
	resources.BeforePublish = func(ctx context.Context, caller identity.HTTPIdentity, cat dataset.Service, definition dataset.Dataset) (dataset.Dataset, map[string]any, error) {
		flags := snapshotRequestFlags(ctx, caller, access.Query{Action: "dataset.manage", Dataset: definition.SetID})
		flags["home"] = opened.Dir
		flags["revision"] = definition.Revision
		prepared, err := inv.PublishDatasetRecipe(&inv.Invocation{
			Home: opened.Dir, WS: opened, Context: ctx, Flags: flags, Command: operation,
			Observation: invoke.NewObservationTelemetry(ctx, f.runtime, operation, func() string { return inv.TelemetryProvider(flags) }),
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
	resources.PreparePublication = func(ctx context.Context, _ identity.HTTPIdentity, definition dataset.Dataset) (dataset.Dataset, error) {
		return opened.DatasetPreparation.Execute(ctx, definition)
	}
	resources.EnsureDataset = func(_ context.Context, _ identity.HTTPIdentity, cat dataset.Service, id string) error {
		// Consumption confirms an accepted definition. First adoption is an
		// explicit publication with target management and source permission.
		_, err := cat.Set(id)
		return err
	}
	resources.RecordRead = func(ctx context.Context, caller identity.HTTPIdentity, evidence snapshotservice.FileReadEvidence) error {
		return f.snapshotRecordFileRead(ctx, opened, caller, evidence)
	}
	resources.OpenView = func(ctx context.Context, caller identity.HTTPIdentity, coordinate snapshotservice.FileCoordinate, cat dataset.Service, definition dataset.Dataset, pin dataset.ResolvedDataset, record func(snapshotservice.FileReadEvidence) error) (snapshotservice.FileView, error) {
		return f.snapshotSemanticView(ctx, opened, caller, coordinate, cat, definition, pin, record)
	}
	return resources
}

func (f *httpFacade) authorizeSnapshotRequest(ctx context.Context, opened *Home, operation string, immutable bool, request snapshotservice.Authorization) error {
	if request.NativeRepository {
		err := authorizeNativeSnapshot(opened, request)
		f.snapshotAuthorizationObserver(ctx, operation)(authorizationDecision(err))
		return err
	}
	flags := snapshotRequestFlags(ctx, request.Caller, request.Query)
	flags["home"] = opened.Dir
	if request.Definition != nil && request.Query.Dataset == "" {
		flags[inv.WorkspaceDefinitionFlag] = request.Definition
	}
	if request.Pin != nil {
		raw, err := json.Marshal(request.Pin)
		if err != nil {
			return err
		}
		flags["pin"] = string(raw)
	}
	if request.InventoryDiscovery {
		flags[inv.InventoryDiscoveryFlag] = true
		if err := inv.PrepareInventoryDiscovery(opened, request.Query.Action, flags); err != nil {
			return err
		}
	}
	var err error
	switch {
	case request.Query.Action == access.RepositoryDatasetPublish:
		rules, readErr := inv.ReadAllow(opened.Dir)
		err = readErr
		if err == nil {
			query := request.Query
			query.Principal = request.Caller.Principal
			if _, allowed := access.MatchSourcePublication(rules.Rules, query); !allowed {
				err = kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", query.Principal, query.Action)
			}
		}
	case request.Query.Action == "file.read" && request.Definition != nil && request.Pin != nil:
		err = inv.RequireCompleteWorkspaceRead(opened.Dir, flags, kchome.DatasetPin(*request.Pin), request.Query.Object)
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
		// Only the private Registry/Reader instances may be stamped on a read
		// lease. Home.ReadView deliberately borrows the shared Writers.
		if opened.Reader != nil {
			opened.Reader.SetJournal(requestJournal)
		}
		{
			cat := opened.Datasets
			if cat != nil {
				cat.SetObserver(snapshotDatasetObserver(requestJournal))
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
		"_action": query.Action, "repo": query.Repo,
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
	flags := snapshotRequestFlags(ctx, caller, access.Query{Action: action, Dataset: metadata.Dataset, Repo: metadata.Repository})
	flags["home"] = lease.opened.Dir
	observation := invoke.NewObservationTelemetry(ctx, f.runtime, operation, func() string { return inv.TelemetryProvider(flags) })
	return inv.WithHooks(lease.opened, lease.opened.Dir, action, flags, observation, run)
}

// snapshotGrantStore preserves the application's complete allow.json ledger.
// UpdateRules runs inside the policy file's read-modify-write lock, so a module
// update cannot drop private admission/provisioning receipts or race another
// policy mutation.
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
	return inv.UpdateAllow(s.home, func(file *inv.AllowFile) error {
		rules, err := update(file.Rules)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		file.Rules = rules
		return nil
	})
}

func snapshotDatasetObserver(j journal.Journal) dataset.Observer {
	return func(operation string, refs map[string]any, err error) error {
		return journal.Finish(j, journal.LayerSystem, "registry", operation, refs, err)
	}
}

func authorizeNativeSnapshot(opened *Home, request snapshotservice.Authorization) error {
	if !access.IsRepositoryReadAction(request.Query.Action) && opened.Store != nil {
		source, ok := opened.Store.Get(kernel.RepositoryID(request.Query.Repo))
		if !ok || !nativeRepositoryWritable(source) {
			return kernel.Fail(kernel.ErrForbidden, "repository has no mutable file-store capability or is archived")
		}
	}
	rules, err := inv.ReadAllow(opened.Dir)
	if err != nil {
		return err
	}
	query := request.Query
	query.Principal = request.Caller.Principal
	if _, ok := access.MatchRepository(rules.Rules, query); ok {
		return nil
	}
	if inv.RepositoryAuthenticatedAllowed(opened.Dir, query.Principal, query.Action, query.Repo, opened) {
		return nil
	}
	return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", query.Principal, query.Action)
}

func nativeRepositoryWritable(source snapshot.Store) bool {
	if source == nil {
		return false
	}
	_, ok := snapshot.TreeStoreOf(source)
	return ok && !source.Archived()
}
