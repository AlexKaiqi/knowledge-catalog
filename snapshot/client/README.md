# snapshot/client

Independent typed HTTP client for Snapshot Store registration, Dataset
publication and delivery, native repository metadata, and explicit access
rules. It has no KC Client, Home, Invocation, Object, Discovery or Server
dependency. Authentication, credentials and refresh belong to the injected
`Transport`; the SDK does not create another identity or session authority.

```go
import snapshotclient "kc/snapshot/client"

sdk, err := snapshotclient.New(snapshotclient.Config{
    BaseURL: serverURL,
    Transport: authenticatedTransport,
})
// authenticatedTransport implements Do(context.Context, string, *http.Request).
// Use sdk.SnapshotService(), sdk.WorkspaceFilesService(), sdk.AdminService().
```

`Config` requires an HTTP(S) service address without credentials, query or
fragment, and an authenticated transport. Each typed call invokes that transport
again; fixed pins, successful reads and inventory never cache authorization.
`RequestOptions.RequestID` is diagnostic correlation, not identity proof.
The shared internal HTTP JSON transport preserves exact JSON numbers, response
limits, structured errors and compression above 512 KiB. Native `/api/v1/`
requests remain uncompressed and use native `message` / `code` errors.

`Client` exposes three services. `Service` addresses inventory and audit,
registered Repository membership, Dataset definitions and releases, and native
metadata. `FilesService` provides Dataset mounts and bounded directory pages,
plus file ranges from a Dataset or fixed Repository commit. `AccessService`
provides explicit grant inventory, add, atomic update, revoke and the current
caller's admission overview. Provisioning,
connection credentials, ownership recovery and sharing remain host application
operations, not independent Snapshot SDK methods.

The standalone Snapshot handler also accepts `Show`, `Audit`, `AttachRepository`
and `DetachRepository`. Inventory uses the same accepted Dataset registration
authority; connected sources are admitted explicitly, without granting access.
`Audit` defaults to 50 entries and accepts at most 200; zero selects the default.
It has no continuation pages and rejects a nonempty continuation, including one
sent through the retained SDK argument. Provider history failures remain errors.

Some deployed registration endpoints currently use the KC host's external
management policy adapters. Moving their typed client does not claim every
endpoint is mounted by a minimal Snapshot handler. Routes and authorization
remain the deployed service's contract.

`DatasetRequest.Metadata` is an optional string map. Publication accepts the
complete map with its revision; omission or `{}` publishes no metadata. Dataset
inventory, selected historical detail and portable recipes preserve accepted
attributes. This capability does not read or interpret explanatory files.

`ShowDataset` accepts `DatasetShowOptions{Revision, Limit, Continuation}`. Nil
revision selects the current release; a positive explicit revision selects an
accepted historical release, independently of its history page. `Dataset`
uses default options. `ResolveDataset` accepts an optional positive
`DatasetResolveRequest.Revision`, mutually exclusive with `Pin`.
`CheckDataset` retains its pin contract and rejects a revision selector.
Current retirement, registration, availability and authorization still apply.

`RepositoryMetadata`, `SetRepositoryMetadata` and `DeleteRepositoryMetadata`
require the actual native lakeFS name at
`/api/v1/repositories/{repository}/metadata`, never a guessed logical ID.
Reads return a string map; sets submit `{metadata: {...}}` and preserve omitted
keys; deletes submit `{keys: [...]}`. Both current product and upstream rights
apply. The original repository remains the only metadata authority.

`WorkspaceFileCoordinate` addresses one published Dataset with an optional
fixed pin. Mount-relative coordinates (`MountPath` plus `Directory` or `File`)
and delivered-tree `Path` coordinates do not mix. Empty delivered `Path`
addresses the root. The fixed commits determine directory entries and bytes;
every read still uses current access policy.

`WorkspaceFileReadRequest` also accepts `Repository`, `Commit` and `Path` for
one raw source file. `Repository` is the logical ID already present in resource
declarations, not a native lakeFS name; `Commit` must be an immutable commit,
not a branch, ref or `HEAD`. This mode cannot carry Dataset, pin, view or
mount-relative coordinates. Call the same `FilesService.Read` method with a
`RepositoryFileReadResponse` output. It returns source coordinates, byte range,
total size and binary content without a Dataset pin. No inventory query or
Dataset publication is required; each request needs the current raw Repository
`file.read` permission. Dataset consumption does not grant that permission.
Directory and mount methods remain Dataset-only.

`UpdateGrant` sends `PUT /admin/v1/grants/{id}` with a full editable replacement
and its previously observed `GrantExpected`. It does not fetch a fresher rule
or emulate atomic update with remove/add. The server owns old/new scope checks,
identity, provenance and durability. `GrantsMatching` is bounded inventory,
not an effective permission evaluator.
