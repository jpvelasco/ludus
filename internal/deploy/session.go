package deploy

import (
	"fmt"
	"time"

	"github.com/devrecon/ludus/internal/state"
)

// SaveSessionState persists session info to state, logging a warning on failure.
// This is shared by all adapters that implement SessionManager.
func SaveSessionState(info *SessionInfo) {
	if err := state.UpdateSession(&state.SessionState{
		SessionID: info.SessionID,
		IPAddress: info.IPAddress,
		Port:      info.Port,
		Status:    "ACTIVE",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write session state: %v\n", err)
	}
}

// ClearFleetState clears fleet state, logging a warning on failure.
// This is shared by adapters that manage fleet lifecycle.
func ClearFleetState() {
	if err := state.ClearFleet(); err != nil {
		fmt.Printf("Warning: failed to clear fleet state: %v\n", err)
	}
}
