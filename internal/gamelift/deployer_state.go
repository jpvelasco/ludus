package gamelift

import (
	"fmt"
	"io"
	"time"

	"github.com/jpvelasco/ludus/internal/state"
)

// RecordFleetState persists the fleet this deployment now runs to the
// profile's state so status, session, and destroy commands find it. Write
// failures are warnings on w, never errors: the fleet exists either way.
//
// An update keeps the fleet's original CreatedAt, and an unchanged fleet that
// is already recorded is left alone so DeployedAt keeps naming the last real
// deployment.
func RecordFleetState(fleet *FleetStatus, w io.Writer) {
	if fleet.Action == ActionUnchanged && recordedFleetID() == fleet.FleetID {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	createdAt := now
	if fleet.Action != ActionCreated {
		if prev := recordedFleetCreatedAt(fleet.FleetID); prev != "" {
			createdAt = prev
		}
	}

	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   fleet.FleetID,
		Status:    fleet.Status,
		CreatedAt: createdAt,
	}); err != nil {
		fmt.Fprintf(w, "Warning: failed to write fleet state: %v\n", err)
	}
	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "gamelift",
		Status:     fleet.Status,
		Detail:     fleetDetail(fleet.FleetID),
		DeployedAt: now,
	}); err != nil {
		fmt.Fprintf(w, "Warning: failed to write deploy state: %v\n", err)
	}
}

// recordedFleetCreatedAt returns when the recorded fleet was first deployed,
// or "" when fleetID is not the recorded fleet.
func recordedFleetCreatedAt(fleetID string) string {
	st, err := state.Load()
	if err != nil || st.Fleet == nil || st.Fleet.FleetID != fleetID {
		return ""
	}
	return st.Fleet.CreatedAt
}

func fleetDetail(fleetID string) string {
	return "fleet " + fleetID
}
