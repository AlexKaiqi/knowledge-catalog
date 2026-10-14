# internal/httpjson

Shared JSON transport plumbing for application and Snapshot HTTP clients.
Callers own typed endpoints and supply an authenticated `Transport`; this
package owns no session, credential, resource authority or authorization rule.

`Do` encodes once, compresses KC bodies above 512 KiB, leaves native lakeFS
requests uncompressed, bounds responses at 8 MiB and preserves exact JSON
numbers and structured recovery errors. It does not retry requests or cache
authorization. Request IDs are correlation only.
