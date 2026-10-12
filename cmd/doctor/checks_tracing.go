package doctor

import (
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/tracing"
)

// checkOTLPTracing reports the effective OTLP trace export state so users can
// see when a shell's OTEL_* environment variables enable export without any
// Ludus config. Informational only — an active export is never a failure.
func checkOTLPTracing(cfg *config.Config) diagnostic {
	var tcfg tracing.Config
	if cfg != nil {
		o := cfg.Observability.OTLP
		tcfg = tracing.Config{Enabled: o.Enabled, Endpoint: o.Endpoint}
	}

	state := tracing.Diagnose(tcfg)
	d := diagnostic{name: "OTLP Tracing", status: "ok"}
	if !state.Active {
		d.message = "off"
		return d
	}

	switch state.Source {
	case "config":
		d.message = "on (observability.otlp.enabled=true)"
	default:
		d.message = "on (via " + state.Source + ")"
	}
	d.details = []string{"endpoint: " + state.Endpoint}
	return d
}
