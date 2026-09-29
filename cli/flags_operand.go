package cli

import ()

// Flag decoding shared by more than one verb. A helper belongs here when two
// verbs must agree on what a flag means; a one-verb reader stays with its verb.
