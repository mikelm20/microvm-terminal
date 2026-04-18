package api

import (
	"encoding/json"
	"time"

	"github.com/mikelm20/learn-platform/control-plane/internal/apitypes"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
)

// toMeResponse converts a db.Identity plus derived progress counters into the
// wire MeResponse shape.
func toMeResponse(id *db.Identity, streak, xp, grace int) apitypes.MeResponse {
	var email, department, name, claimedAt *string
	if id.Email.Valid {
		v := id.Email.String
		email = &v
	}
	if id.Department.Valid {
		v := id.Department.String
		department = &v
	}
	if id.Name.Valid {
		v := id.Name.String
		name = &v
	}
	if id.ClaimedAt.Valid {
		v := id.ClaimedAt.Time.UTC().Format(time.RFC3339)
		claimedAt = &v
	}
	return apitypes.MeResponse{
		UUID:           id.UUID.String(),
		Email:          email,
		Lang:           apitypes.Lang(id.Lang),
		Department:     department,
		Name:           name,
		StreakDays:     streak,
		TotalXP:        xp,
		GraceTokens:    grace,
		HapticsEnabled: id.HapticsEnabled,
		PushEnabled:    id.PushEnabled,
		CreatedAt:      id.CreatedAt.UTC().Format(time.RFC3339),
		ClaimedAt:      claimedAt,
	}
}

// extractStats pulls streak/xp/grace tokens from a raw progress state JSON blob.
func extractStats(raw []byte) (streak, xp, grace int) {
	grace = 3
	if len(raw) == 0 {
		return 0, 0, grace
	}
	var state apitypes.ProgressState
	if err := json.Unmarshal(raw, &state); err != nil {
		return 0, 0, grace
	}
	streak = state.StreakDays
	grace = state.GraceTokens
	for _, m := range state.Modules {
		xp += m.XPEarned
	}
	return
}
