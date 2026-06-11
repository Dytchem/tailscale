// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package magicsock

import (
	"testing"
	"time"
)

func TestParseConnPref_Default(t *testing.T) {
	p := parseConnPref("")
	if len(p.entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(p.entries))
	}
	if !p.hasDirect {
		t.Error("expected hasDirect=true")
	}
	if !p.hasAnyDERP {
		t.Error("expected hasAnyDERP=true")
	}
	if !p.hasPeerRelay {
		t.Error("expected hasPeerRelay=true")
	}
}

func TestParseConnPref_DirectOnly(t *testing.T) {
	p := parseConnPref("direct")
	if len(p.entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(p.entries))
	}
	if !p.hasDirect {
		t.Error("expected hasDirect=true")
	}
	if p.hasPeerRelay {
		t.Error("expected hasPeerRelay=false")
	}
	if p.hasAnyDERP {
		t.Error("expected hasAnyDERP=false")
	}
	if !p.directAllowed() {
		t.Error("directAllowed should be true")
	}
	if p.allowPeerRelay() {
		t.Error("allowPeerRelay should be false")
	}
}

func TestParseConnPref_SpecificDERP(t *testing.T) {
	p := parseConnPref("direct,derp:900,derp:901,derp:*,peer-relay")
	if len(p.entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(p.entries))
	}
	if !p.hasDirect {
		t.Error("expected hasDirect=true")
	}
	if !p.hasPeerRelay {
		t.Error("expected hasPeerRelay=true")
	}
	if !p.hasAnyDERP {
		t.Error("expected hasAnyDERP=true (derp:* present)")
	}
	if len(p.derpOrder) != 2 {
		t.Errorf("expected 2 specific DERP regions, got %v", p.derpOrder)
	}
	if p.derpOrder[0] != 900 || p.derpOrder[1] != 901 {
		t.Errorf("unexpected derpOrder: %v", p.derpOrder)
	}
}

func TestParseConnPref_DERPOnly(t *testing.T) {
	p := parseConnPref("derp:999,derp:*")
	if p.hasDirect {
		t.Error("expected hasDirect=false")
	}
	if !p.hasAnyDERP {
		t.Error("expected hasAnyDERP=true")
	}
	if p.hasPeerRelay {
		t.Error("expected hasPeerRelay=false")
	}
	if len(p.derpOrder) != 1 || p.derpOrder[0] != 999 {
		t.Errorf("unexpected derpOrder: %v", p.derpOrder)
	}
}

func TestParseConnPref_InvalidToken(t *testing.T) {
	p := parseConnPref("direct,invalid-token,derp:*")
	// Should fallback to default on invalid token
	if len(p.entries) != 3 {
		t.Errorf("expected default 3 entries, got %d", len(p.entries))
	}
	if !p.hasDirect || !p.hasAnyDERP || !p.hasPeerRelay {
		t.Error("expected default preference on invalid token")
	}
}

func TestParseConnPref_InvalidDERPID(t *testing.T) {
	p := parseConnPref("direct,derp:abc,derp:*")
	if len(p.entries) != 3 {
		t.Errorf("expected default 3 entries, got %d", len(p.entries))
	}
}

func TestParseConnPref_DuplicateDERPRegions(t *testing.T) {
	p := parseConnPref("derp:900,derp:900,derp:901")
	if len(p.derpOrder) != 2 {
		t.Errorf("expected 2 unique regions, got %v", p.derpOrder)
	}
	if p.derpOrder[0] != 900 || p.derpOrder[1] != 901 {
		t.Errorf("unexpected derpOrder: %v", p.derpOrder)
	}
}

func TestConnPref_MethodPosition(t *testing.T) {
	p := parseConnPref("direct,derp:999,peer-relay")
	if pos := p.methodPosition(connMethodDirect); pos != 0 {
		t.Errorf("direct position: want 0, got %d", pos)
	}
	if pos := p.methodPosition(connMethodDERP); pos != 1 {
		t.Errorf("DERP position: want 1, got %d", pos)
	}
	if pos := p.methodPosition(connMethodPeerRelay); pos != 2 {
		t.Errorf("peer-relay position: want 2, got %d", pos)
	}
}

func TestConnPref_PreferDERPOverDirect(t *testing.T) {
	// Default: direct before DERP
	if p := parseConnPref(""); p.preferDERPOverDirect() {
		t.Error("default should not prefer DERP over direct")
	}
	// Direct before DERP
	if p := parseConnPref("direct,derp:*"); p.preferDERPOverDirect() {
		t.Error("direct before derp should not prefer DERP")
	}
	// DERP before direct
	if p := parseConnPref("derp:*,direct"); !p.preferDERPOverDirect() {
		t.Error("derp before direct should prefer DERP")
	}
	// No direct at all
	if p := parseConnPref("derp:*,peer-relay"); !p.preferDERPOverDirect() {
		t.Error("no direct should prefer DERP over direct")
	}
}

func TestConnPref_PreferDERPOverPeerRelay(t *testing.T) {
	// Default: DERP before peer-relay
	if p := parseConnPref(""); !p.preferDERPOverPeerRelay() {
		t.Error("default should prefer DERP over peer-relay")
	}
	// Peer-relay before DERP
	if p := parseConnPref("peer-relay,derp:*"); p.preferDERPOverPeerRelay() {
		t.Error("peer-relay before derp should not prefer DERP")
	}
	// DERP before peer-relay
	if p := parseConnPref("derp:*,peer-relay"); !p.preferDERPOverPeerRelay() {
		t.Error("derp before peer-relay should prefer DERP")
	}
	// No DERP at all
	if p := parseConnPref("peer-relay"); p.preferDERPOverPeerRelay() {
		t.Error("no DERP should not prefer DERP over peer-relay")
	}
}

func TestConnPref_DirectAllowed(t *testing.T) {
	if p := parseConnPref("derp:*,peer-relay"); p.directAllowed() {
		t.Error("direct should not be allowed when not in list")
	}
	if p := parseConnPref("direct,derp:*"); !p.directAllowed() {
		t.Error("direct should be allowed when in list")
	}
}

func TestConnPref_AllowPeerRelay(t *testing.T) {
	if p := parseConnPref("direct,derp:*"); p.allowPeerRelay() {
		t.Error("peer-relay should not be allowed when not in list")
	}
	if p := parseConnPref("direct,derp:*,peer-relay"); !p.allowPeerRelay() {
		t.Error("peer-relay should be allowed when in list")
	}
}

func TestConnPref_DERPRegionAllowed(t *testing.T) {
	p := parseConnPref("derp:900,derp:901,derp:*")
	if !p.derpRegionAllowed(1) {
		t.Error("any DERP should allow region 1")
	}
	if !p.derpRegionAllowed(900) {
		t.Error("any DERP should allow region 900")
	}

	p = parseConnPref("derp:900,derp:901,direct")
	if p.derpRegionAllowed(1) {
		t.Error("region 1 should not be allowed")
	}
	if !p.derpRegionAllowed(900) {
		t.Error("region 900 should be allowed")
	}
	if !p.derpRegionAllowed(901) {
		t.Error("region 901 should be allowed")
	}
}

func TestConnPref_SelectPreferredDERP(t *testing.T) {
	latency := map[int]time.Duration{
		1:   10 * time.Millisecond,
		900: 50 * time.Millisecond,
		901: 100 * time.Millisecond,
	}

	// With any DERP (no specific regions), selectPreferredDERP returns 0 (let existing logic decide)
	p := parseConnPref("direct,derp:*,peer-relay")
	if selected := p.selectPreferredDERP(latency, 0); selected != 0 {
		t.Errorf("expected 0 for any DERP, got %d", selected)
	}

	// With specific ordering (no derp:*), should pick first preferred with latency data
	p = parseConnPref("direct,derp:901,derp:900,peer-relay")
	if selected := p.selectPreferredDERP(latency, 0); selected != 901 {
		t.Errorf("expected 901 as first preferred, got %d", selected)
	}

	// Should keep current home if it's in the preferred list
	if selected := p.selectPreferredDERP(latency, 900); selected != 900 {
		t.Errorf("expected 900 as current home, got %d", selected)
	}

	// If no preferred region has latency, force-connect to the first preferred region
	emptyLatency := map[int]time.Duration{}
	if selected := p.selectPreferredDERP(emptyLatency, 0); selected != 901 {
		t.Errorf("expected 901 as forced first region, got %d", selected)
	}

	// With specific ordering AND derp:* fallback: specific regions are checked first,
	// then if none match, 0 is returned to let existing logic handle the wildcard.
	p = parseConnPref("direct,derp:901,derp:900,derp:*,peer-relay")
	if selected := p.selectPreferredDERP(latency, 0); selected != 901 {
		t.Errorf("expected 901 as first preferred (with wildcard fallback), got %d", selected)
	}
}

func TestConnPref_SelectPreferredDERP_CurrentHomeFallback(t *testing.T) {
	latency := map[int]time.Duration{
		1: 10 * time.Millisecond,
	}

	// Current home is in preferred list but not in latency map; should still try it
	p := parseConnPref("direct,derp:900,peer-relay")
	if selected := p.selectPreferredDERP(latency, 900); selected != 900 {
		t.Errorf("expected 900 as current home fallback, got %d", selected)
	}
}

func TestConnPref_EndToEndScenarios(t *testing.T) {
	tests := []struct {
		name    string
		pref    string
		checkFn func(p connPref) bool
	}{
		{
			// Problem 1: Bad direct, good self-DERP → prefer DERP over direct
			name: "problem1-self-derp-before-direct",
			pref: "derp:999,derp:*,direct",
			checkFn: func(p connPref) bool {
				return p.preferDERPOverDirect() &&
					p.derpRegionAllowed(999) &&
					p.derpRegionAllowed(1) &&
					p.directAllowed()
			},
		},
		{
			// Problem 2: Self-hosted DERP quality ordering
			name: "problem2-derp-quality-ordering",
			pref: "direct,derp:900,derp:901,derp:902,derp:*",
			checkFn: func(p connPref) bool {
				return len(p.derpOrder) == 3 &&
					p.derpOrder[0] == 900 &&
					p.derpOrder[1] == 901 &&
					p.derpOrder[2] == 902
			},
		},
		{
			// Problem 3: Skip official DERP entirely
			name: "problem3-no-official-derp",
			pref: "direct,derp:999",
			checkFn: func(p connPref) bool {
				return !p.hasAnyDERP &&
					p.derpRegionAllowed(999) &&
					!p.derpRegionAllowed(1) &&
					p.directAllowed()
			},
		},
		{
			// Problem 4: No peer relay
			name: "problem4-no-peer-relay",
			pref: "direct,derp:900,derp:*",
			checkFn: func(p connPref) bool {
				return !p.allowPeerRelay() &&
					p.directAllowed() &&
					p.hasAnyDERP
			},
		},
		{
			// Full custom: DERP 900 > DERP 901 > any DERP > direct > peer-relay
			name: "full-custom-ordering",
			pref: "derp:900,derp:901,derp:*,direct,peer-relay",
			checkFn: func(p connPref) bool {
				return p.preferDERPOverDirect() &&
					p.preferDERPOverPeerRelay() &&
					p.derpOrder[0] == 900 &&
					p.derpOrder[1] == 901 &&
					len(p.entries) == 5
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parseConnPref(tt.pref)
			if !tt.checkFn(p) {
				t.Errorf("check failed for pref=%q", tt.pref)
				t.Logf("  entries=%d, hasDirect=%v, hasAnyDERP=%v, hasPeerRelay=%v, derpOrder=%v",
					len(p.entries), p.hasDirect, p.hasAnyDERP, p.hasPeerRelay, p.derpOrder)
			}
		})
	}
}

func TestParseConnPref_Whitespace(t *testing.T) {
	p := parseConnPref(" direct , derp:999 , peer-relay ")
	if len(p.entries) != 3 {
		t.Errorf("expected 3 entries with whitespace, got %d", len(p.entries))
	}
}

func TestConnPref_PreferredDERPRegions(t *testing.T) {
	// Any DERP => nil
	p := parseConnPref("direct,derp:*,peer-relay")
	if got := p.preferredDERPRegions(); got != nil {
		t.Errorf("expected nil for any DERP, got %v", got)
	}

	// Specific DERP => ordered list
	p = parseConnPref("direct,derp:901,derp:900")
	if got := p.preferredDERPRegions(); len(got) != 2 || got[0] != 901 || got[1] != 900 {
		t.Errorf("expected [901 900], got %v", got)
	}

	// No DERP => nil
	p = parseConnPref("direct,peer-relay")
	if got := p.preferredDERPRegions(); got != nil {
		t.Errorf("expected nil for no DERP, got %v", got)
	}
}

func TestConnPref_SelectPreferredDERP_CurrentHomePreferred(t *testing.T) {
	latency := map[int]time.Duration{
		900: 100 * time.Millisecond,
		901: 5 * time.Millisecond,
	}
	p := parseConnPref("direct,derp:900,derp:901,peer-relay")
	// Current home is 900, which is preferred over 901 even though 901 has lower latency
	if selected := p.selectPreferredDERP(latency, 900); selected != 900 {
		t.Errorf("expected current home 900, got %d", selected)
	}
}
