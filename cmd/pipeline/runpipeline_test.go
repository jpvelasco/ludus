package pipeline

import (
	"testing"
)

func TestRunPipelineViaHelpers(t *testing.T) {
	// runPipeline requires cmd.Context() to be initialized by Cobra framework.
	// Testing via lower-level executeStages instead.
	t.Skip("runPipeline tested via E2E; executeStages covered by TestExecuteStages_* tests")
}
