package caddy

import (
	"fmt"

	"github.com/quic-go/quic-go"
	h3qlog "github.com/quic-go/quic-go/http3/qlog"
)

// experimentalQUICConfig keeps NewReno as the default. The environment switch
// applies to all newly created QUIC listeners and requires a process restart;
// reloading a Caddyfile can reuse an existing listener and its configuration.
func experimentalQUICConfig(allow0RTT bool, algorithm string) (*quic.Config, error) {
	cfg := &quic.Config{
		Allow0RTT: allow0RTT,
		Tracer:    h3qlog.DefaultConnectionTracer,
	}
	switch algorithm {
	case "", "reno":
	case "bbrv1":
		cfg.Congestion = quic.NewBBRCongestionController()
	default:
		return nil, fmt.Errorf("invalid CADDY_QUIC_CONGESTION %q: expected reno or bbrv1", algorithm)
	}
	return cfg, nil
}
