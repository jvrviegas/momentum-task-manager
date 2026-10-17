package api

// Error is the body of every failed response, and the error the client
// returns for one. Message is written for the user. Field names the request
// field at fault, by its JSON name, as the bridge's field errors do.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func (e *Error) Error() string { return e.Message }

// Error codes. Clients show Message whatever the code, so later tasks may add
// codes without breaking them.
const (
	// CodeInvalid: the request, or the field Field names, is invalid.
	CodeInvalid = "invalid"
	// CodeUnauthorized: the token is missing, expired, or revoked.
	CodeUnauthorized = "unauthorized"
	// CodeNotFound: the user has no such task.
	CodeNotFound = "not_found"
	// CodeConflict: something changed since the client read it, such as a
	// task after a project migration preview or before an undo, or an
	// idempotency key was reused for a different request.
	CodeConflict = "conflict"
	// CodeInternal: the server failed.
	CodeInternal = "internal"
	// CodeRateLimited: the client sent too many requests; the Retry-After
	// header says how many seconds to wait.
	CodeRateLimited = "rate_limited"
)
