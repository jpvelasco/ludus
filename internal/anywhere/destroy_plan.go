package anywhere

import (
	"strconv"

	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

// PlanDestroy reads recorded candidates and the existing runtime-directory
// selector. It does not signal a process or construct an AWS client.
func PlanDestroy(st *state.State) deploy.DestroyTargetPlan {
	plan := deploy.DestroyTargetPlan{Target: "anywhere", Items: []deploy.DestroyItem{}}
	as := st.Anywhere
	if as == nil {
		plan.Items = append(plan.Items, deploy.DestroyItem{
			Kind: "deployment", Action: deploy.ActionDelete,
			Decision: deploy.DecisionSkip, Source: deploy.SourceState,
			Reason: "No Anywhere deployment state; execution does nothing.",
		})
		return plan
	}
	pid := ""
	if as.PID > 0 {
		pid = strconv.Itoa(as.PID)
	}
	process := deploy.RecordedDestroyItem("process", pid, deploy.ActionStop)
	if pid != "" {
		process.Reason = "Recorded PID only; process identity and liveness are not checked offline."
	}
	compute := deploy.RecordedDestroyItem("compute", as.ComputeName, deploy.ActionDeregister)
	if as.FleetID == "" {
		compute.Decision, compute.Reason = deploy.DecisionSkip, "No fleet recorded; deregistration requires both fleet and compute."
	} else {
		compute.Conditions = []string{"Deregister from recorded fleet " + as.FleetID + "."}
	}
	plan.Items = []deploy.DestroyItem{
		process, compute,
		deploy.RecordedDestroyItem("fleet", as.FleetID, deploy.ActionDelete),
		deploy.RecordedDestroyItem("location", as.LocationName, deploy.ActionDelete),
		planRuntimeDirectory(as.WrapperConfigDir),
	}
	for i := range plan.Items {
		plan.Items[i].Conditions = append(plan.Items[i].Conditions, "Best-effort sequence; a prior failure does not necessarily suppress this attempt.")
	}
	return plan
}

func planRuntimeDirectory(recorded string) deploy.DestroyItem {
	directory := resolveDestroyConfigDir(recorded)
	item := deploy.RecordedDestroyItem("directory", directory, deploy.ActionDelete)
	if directory != "" {
		item.Reason = "Remove this wrapper runtime directory and its contents."
		if recorded == "" {
			item.Source = deploy.SourceDefault
			item.Reason += " Legacy config file exists; no per-deployment directory was recorded."
		}
	}
	return item
}
