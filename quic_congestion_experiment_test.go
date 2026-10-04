package caddy

import "testing"

func TestExperimentalQUICConfig(t *testing.T) {
	for _, tc := range []struct {
		algorithm string
		wantBBR   bool
		wantError bool
	}{
		{algorithm: ""},
		{algorithm: "reno"},
		{algorithm: "bbrv1", wantBBR: true},
		{algorithm: "bbr", wantError: true},
		{algorithm: "BBRv1", wantError: true},
		{algorithm: "bbrv1 ", wantError: true},
	} {
		t.Run(tc.algorithm, func(t *testing.T) {
			for _, allow0RTT := range []bool{true, false} {
				cfg, err := experimentalQUICConfig(allow0RTT, tc.algorithm)
				if (err != nil) != tc.wantError {
					t.Fatalf("error = %v, want error = %v", err, tc.wantError)
				}
				if err != nil {
					continue
				}
				if cfg.Allow0RTT != allow0RTT || cfg.Tracer == nil {
					t.Fatal("0-RTT setting or qlog tracer was not preserved")
				}
				if (cfg.Congestion != nil) != tc.wantBBR {
					t.Fatalf("BBR enabled = %v, want %v", cfg.Congestion != nil, tc.wantBBR)
				}
				if tc.wantBBR {
					first, second := cfg.Congestion(), cfg.Congestion()
					if first == nil || second == nil || first == second {
						t.Fatal("each connection must receive an independent controller")
					}
					if first.GetCongestionWindow() <= 0 {
						t.Fatal("BBR must start with a positive congestion window")
					}
				}
			}
		})
	}
}
