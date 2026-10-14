# LiveAccess component fixtures

`AdoptRepository` accepts an explicit Repository ID and lakeFS DSN and selects
a LiveAccess instance in its native `kc.plugins` metadata. `AdoptMetadata` uses the same
operation when a fixture already owns the native metadata port. Both preserve
other plugin selections and reject an existing conflicting access instance.

These helpers are test inputs, not runtime defaults or plugin registration.
The host fixture locates its own declaration and calls them only for
Repositories whose scenario uses LiveAccess; this package does not open Home.
Production packages must not import this package.
