// Helpers for the generated wire types. Everything here is hand-written on top
// of apitypes.go (which is generated from shared/api/*.ts). Keep this file
// narrow: error codes, language constants, domain validators.

package apitypes

// Lang is the wire locale tag ("es" | "en"). Alias so handler code can accept
// either a raw string or a typed literal.
type Lang = string

const (
	LangES = "es"
	LangEN = "en"
)

// Department mirrors DepartmentSchema in shared/api/schemas.ts.
type Department = string

// OKResponse is a tiny helper wire type for handlers that return { "ok": true }.
type OKResponse struct {
	Ok bool `json:"ok"`
}


// Error codes mirror ApiErrorCode in shared/api/errors.ts. Handlers use these
// as the Code string when returning ApiError responses.
const (
	ErrCapacityFull     = "capacity_full"
	ErrAuthRequired     = "auth_required"
	ErrAuthInvalid      = "auth_invalid"
	ErrMagicLinkExpired = "magic_link_expired"
	ErrVMLaunchFailed   = "vm_launch_failed"
	ErrVMNotFound       = "vm_not_found"
	ErrSessionReaped    = "session_reaped"
	ErrBadRequest       = "bad_request"
	ErrNotFound         = "not_found"
	ErrRateLimited      = "rate_limited"
	ErrInternal         = "internal"
)

// ValidLang reports whether s is a recognized wire locale.
func ValidLang(s string) bool {
	return s == "es" || s == "en"
}

// ValidDepartment reports whether s is a recognized department slug.
func ValidDepartment(s string) bool {
	switch s {
	case "ventas", "marketing", "tecnologia", "producto",
		"finanzas", "rrhh", "legal", "estrategia":
		return true
	}
	return false
}
