package steps

import (
	"encoding/json"
	"log/slog"

	"github.com/marcelblijleven/bifrost/internal/pipeline"
)

// setOutput marshals v as JSON and stores it in sc.Output.
// Marshalling failures are logged and silently ignored.
func setOutput(sc *pipeline.StepContext, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Warn("step: failed to marshal output", "err", err)
		return
	}
	sc.Output = string(b)
}
