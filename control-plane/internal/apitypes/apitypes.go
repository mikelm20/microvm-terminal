// Package apitypes mirrors shared/api/schemas.ts and events.ts for Go.
// Hand-written for the sprint. Agent-Tooling is expected to replace this
// with tygo output; the field names and JSON tags match the Zod shapes.
package apitypes

import "encoding/json"

// --- enums ---

type Lang string

const (
	LangES Lang = "es"
	LangEN Lang = "en"
)

type Department string

const (
	DeptVentas     Department = "ventas"
	DeptMarketing  Department = "marketing"
	DeptTecnologia Department = "tecnologia"
	DeptProducto   Department = "producto"
	DeptFinanzas   Department = "finanzas"
	DeptRRHH       Department = "rrhh"
	DeptLegal      Department = "legal"
	DeptEstrategia Department = "estrategia"
)

func ValidDepartment(d string) bool {
	switch Department(d) {
	case DeptVentas, DeptMarketing, DeptTecnologia, DeptProducto,
		DeptFinanzas, DeptRRHH, DeptLegal, DeptEstrategia:
		return true
	}
	return false
}

func ValidLang(l string) bool {
	switch Lang(l) {
	case LangES, LangEN:
		return true
	}
	return false
}

// --- identity ---

type MintIdentityRequest struct {
	UUID       *string `json:"uuid,omitempty"`
	DeviceHint *string `json:"device_hint,omitempty"`
	Lang       Lang    `json:"lang"`
}

type MintIdentityResponse struct {
	UUID   string `json:"uuid"`
	Cookie string `json:"cookie"`
}

// --- auth ---

type RequestMagicLinkRequest struct {
	Email         string  `json:"email"`
	AnonymousUUID *string `json:"anonymous_uuid,omitempty"`
	Lang          Lang    `json:"lang"`
}

type RequestMagicLinkResponse struct {
	OK               bool `json:"ok"`
	ExpiresInSeconds int  `json:"expires_in_seconds"`
}

type ClaimAccountRequest struct {
	MagicLinkToken string  `json:"magic_link_token"`
	AnonymousUUID  *string `json:"anonymous_uuid,omitempty"`
}

type ClaimAccountResponse struct {
	UUID         string `json:"uuid"`
	Email        string `json:"email"`
	SessionToken string `json:"session_token"`
}

// --- me ---

type MeResponse struct {
	UUID           string  `json:"uuid"`
	Email          *string `json:"email"`
	Lang           Lang    `json:"lang"`
	Department     *string `json:"department"`
	Name           *string `json:"name"`
	StreakDays     int     `json:"streak_days"`
	TotalXP        int     `json:"total_xp"`
	GraceTokens    int     `json:"grace_tokens"`
	HapticsEnabled bool    `json:"haptics_enabled"`
	PushEnabled    bool    `json:"push_enabled"`
	CreatedAt      string  `json:"created_at"`
	ClaimedAt      *string `json:"claimed_at"`
}

type PatchMeRequest struct {
	Lang           *Lang   `json:"lang,omitempty"`
	Department     *string `json:"department,omitempty"`
	Name           *string `json:"name,omitempty"`
	HapticsEnabled *bool   `json:"haptics_enabled,omitempty"`
	PushEnabled    *bool   `json:"push_enabled,omitempty"`
}

// --- progress ---

type ModuleProgress struct {
	CompletedSteps []string `json:"completed_steps"`
	PausedAt       *string  `json:"paused_at"`
	XPEarned       int      `json:"xp_earned"`
}

type ProgressState struct {
	Modules        map[string]ModuleProgress `json:"modules"`
	StreakDays     int                       `json:"streak_days"`
	GraceTokens    int                       `json:"grace_tokens"`
	LastVisited    string                    `json:"last_visited"`
	SeenCoachmarks []string                  `json:"seen_coachmarks"`
	UpdatedAt      string                    `json:"updated_at"`
}

type ProgressSyncRequest struct {
	UUID  string        `json:"uuid"`
	State ProgressState `json:"state"`
}

type ProgressSyncResponse struct {
	State           ProgressState `json:"state"`
	ServerUpdatedAt string        `json:"server_updated_at"`
}

// --- lessons ---

type LessonSummary struct {
	ID               string  `json:"id"`
	ModuleNumber     int     `json:"module_number"`
	Language         Lang    `json:"language"`
	Title            string  `json:"title"`
	Subtitle         *string `json:"subtitle,omitempty"`
	EstimatedMinutes *int    `json:"estimated_minutes,omitempty"`
}

type ListLessonsResponse struct {
	Lessons []LessonSummary `json:"lessons"`
}

// --- sessions ---

type CreateSessionRequest struct {
	LessonID string `json:"lesson_id"`
	Lang     Lang   `json:"lang"`
	Warm     *bool  `json:"warm,omitempty"`
}

type CreateSessionResponse struct {
	SessionID          string `json:"session_id"`
	VMIP               string `json:"vm_ip"`
	PtyWSURL           string `json:"pty_ws_url"`
	WizardWSURL        string `json:"wizard_ws_url"`
	PreviewURLTemplate string `json:"preview_url_template"`
}

type HeartbeatResponse struct {
	SessionID   string `json:"session_id"`
	VMIP        string `json:"vm_ip"`
	Alive       bool   `json:"alive"`
	ClaudeBusy  bool   `json:"claude_busy"`
	LastEventAt string `json:"last_event_at"`
	AgeSeconds  int    `json:"age_seconds"`
}

type TranscriptResponse struct {
	SessionID string        `json:"session_id"`
	Events    []WsEventRaw  `json:"events"`
}

// WsEventRaw carries an event payload as already-serialized JSON.
// Emitted as-is, preserving the wire shape from events.ts.
type WsEventRaw = json.RawMessage

type SubmitPromptRequest struct {
	Text     string `json:"text"`
	ClientTS string `json:"client_ts"`
}

type SubmitPromptResponse struct {
	OK     bool   `json:"ok"`
	TurnID string `json:"turn_id"`
}

type AttachImageResponse struct {
	InboxPath string `json:"inbox_path"`
}

type OKResponse struct {
	OK bool `json:"ok"`
}

// --- public ---

type PublicProfileResponse struct {
	UUID             string  `json:"uuid"`
	Name             *string `json:"name"`
	Department       *string `json:"department"`
	Lang             Lang    `json:"lang"`
	StreakDays       int     `json:"streak_days"`
	TotalXP          int     `json:"total_xp"`
	ModulesCompleted []struct {
		LessonID    string   `json:"lesson_id"`
		Title       string   `json:"title"`
		CompletedAt string   `json:"completed_at"`
		Takeaways   []string `json:"takeaways"`
	} `json:"modules_completed"`
}

type PublicCertificateResponse struct {
	ID          string   `json:"id"`
	UUID        string   `json:"uuid"`
	Name        *string  `json:"name"`
	Department  *string  `json:"department"`
	Lang        Lang     `json:"lang"`
	CompletedAt string   `json:"completed_at"`
	Signature   string   `json:"signature"`
	Modules     []string `json:"modules"`
}

// --- errors ---

type ApiErrorCode string

const (
	ErrCapacityFull     ApiErrorCode = "capacity_full"
	ErrAuthRequired     ApiErrorCode = "auth_required"
	ErrAuthInvalid      ApiErrorCode = "auth_invalid"
	ErrMagicLinkExpired ApiErrorCode = "magic_link_expired"
	ErrVMLaunchFailed   ApiErrorCode = "vm_launch_failed"
	ErrVMNotFound       ApiErrorCode = "vm_not_found"
	ErrSessionReaped    ApiErrorCode = "session_reaped"
	ErrBadRequest       ApiErrorCode = "bad_request"
	ErrNotFound         ApiErrorCode = "not_found"
	ErrRateLimited      ApiErrorCode = "rate_limited"
	ErrInternal         ApiErrorCode = "internal"
)

type ApiError struct {
	Code              ApiErrorCode `json:"code"`
	Message           string       `json:"message"`
	RetryAfterSeconds *int         `json:"retry_after_seconds,omitempty"`
	QueuePosition     *int         `json:"queue_position,omitempty"`
	RequestID         string       `json:"request_id"`
}
