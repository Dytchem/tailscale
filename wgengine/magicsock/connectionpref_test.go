// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package magicsock

import (
	"os"
	"strings"
	"testing"
	"time"

	"tailscale.com/envknob"
)

// TestMain clears TS_CONNECTION_PREFERENCE so a developer's shell
// environment cannot change the behavior of unrelated magicsock tests.
func TestMain(m *testing.M) {
	envknob.Setenv("TS_CONNECTION_PREFERENCE", "")
	os.Exit(m.Run())
}

func TestParseConnPref_Default(t *testing.T) {
	p, _ := parseConnPref("")
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
	p, _ := parseConnPref("direct")
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
	p, _ := parseConnPref("direct,derp:900,derp:901,derp:*,peer-relay")
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
	p, _ := parseConnPref("derp:999,derp:*")
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
	p, err := parseConnPref("direct,invalid-token,derp:*")
	// Invalid token: error is returned and preference falls back to default.
	if err == nil {
		t.Error("expected error for invalid token")
	}
	if len(p.entries) != 3 {
		t.Errorf("expected default 3 entries, got %d", len(p.entries))
	}
	if !p.hasDirect || !p.hasAnyDERP || !p.hasPeerRelay {
		t.Error("expected default preference on invalid token")
	}
}

func TestParseConnPref_InvalidDERPID(t *testing.T) {
	p, err := parseConnPref("direct,derp:abc,derp:*")
	if err == nil {
		t.Error("expected error for invalid DERP ID")
	}
	if len(p.entries) != 3 {
		t.Errorf("expected default 3 entries, got %d", len(p.entries))
	}
}

func TestParseConnPref_DuplicateDERPRegions(t *testing.T) {
	p, _ := parseConnPref("derp:900,derp:900,derp:901")
	if len(p.derpOrder) != 2 {
		t.Errorf("expected 2 unique regions, got %v", p.derpOrder)
	}
	if p.derpOrder[0] != 900 || p.derpOrder[1] != 901 {
		t.Errorf("unexpected derpOrder: %v", p.derpOrder)
	}
}

func TestConnPref_MethodPosition(t *testing.T) {
	p, _ := parseConnPref("direct,derp:999,peer-relay")
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
	if p, _ := parseConnPref(""); p.preferDERPOverDirect() {
		t.Error("default should not prefer DERP over direct")
	}
	// Direct before DERP
	if p, _ := parseConnPref("direct,derp:*"); p.preferDERPOverDirect() {
		t.Error("direct before derp should not prefer DERP")
	}
	// DERP before direct
	if p, _ := parseConnPref("derp:*,direct"); !p.preferDERPOverDirect() {
		t.Error("derp before direct should prefer DERP")
	}
	// No direct at all
	if p, _ := parseConnPref("derp:*,peer-relay"); !p.preferDERPOverDirect() {
		t.Error("no direct should prefer DERP over direct")
	}
}

func TestConnPref_PreferDERPOverPeerRelay(t *testing.T) {
	// Default: no reordering of trusted paths (matches upstream behavior).
	if p, _ := parseConnPref(""); p.preferDERPOverPeerRelay() {
		t.Error("default should not prefer DERP over peer-relay (upstream behavior)")
	}
	// Peer-relay before DERP
	if p, _ := parseConnPref("peer-relay,derp:*"); p.preferDERPOverPeerRelay() {
		t.Error("peer-relay before derp should not prefer DERP")
	}
	// DERP before peer-relay
	if p, _ := parseConnPref("derp:*,peer-relay"); !p.preferDERPOverPeerRelay() {
		t.Error("derp before peer-relay should prefer DERP")
	}
	// No DERP at all
	if p, _ := parseConnPref("peer-relay"); p.preferDERPOverPeerRelay() {
		t.Error("no DERP should not prefer DERP over peer-relay")
	}
}

func TestConnPref_DirectAllowed(t *testing.T) {
	if p, _ := parseConnPref("derp:*,peer-relay"); p.directAllowed() {
		t.Error("direct should not be allowed when not in list")
	}
	if p, _ := parseConnPref("direct,derp:*"); !p.directAllowed() {
		t.Error("direct should be allowed when in list")
	}
}

func TestConnPref_AllowPeerRelay(t *testing.T) {
	if p, _ := parseConnPref("direct,derp:*"); p.allowPeerRelay() {
		t.Error("peer-relay should not be allowed when not in list")
	}
	if p, _ := parseConnPref("direct,derp:*,peer-relay"); !p.allowPeerRelay() {
		t.Error("peer-relay should be allowed when in list")
	}
}

func TestConnPref_DERPRegionAllowed(t *testing.T) {
	p, _ := parseConnPref("derp:900,derp:901,derp:*")
	if !p.derpRegionAllowed(1) {
		t.Error("any DERP should allow region 1")
	}
	if !p.derpRegionAllowed(900) {
		t.Error("any DERP should allow region 900")
	}

	p, _ = parseConnPref("derp:900,derp:901,direct")
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
	p, _ := parseConnPref("direct,derp:*,peer-relay")
	if selected := p.selectPreferredDERP(latency, 0, func(int) bool { return true }); selected != 0 {
		t.Errorf("expected 0 for any DERP, got %d", selected)
	}

	// With specific ordering (no derp:*), should pick first preferred with latency data
	p, _ = parseConnPref("direct,derp:901,derp:900,peer-relay")
	if selected := p.selectPreferredDERP(latency, 0, func(int) bool { return true }); selected != 901 {
		t.Errorf("expected 901 as first preferred, got %d", selected)
	}

	// Should keep current home if it's in the preferred list
	if selected := p.selectPreferredDERP(latency, 900, func(int) bool { return true }); selected != 900 {
		t.Errorf("expected 900 as current home, got %d", selected)
	}

	// If no preferred region has latency, force-connect to the first preferred region
	emptyLatency := map[int]time.Duration{}
	if selected := p.selectPreferredDERP(emptyLatency, 0, func(int) bool { return true }); selected != 901 {
		t.Errorf("expected 901 as forced first region, got %d", selected)
	}

	// With specific ordering AND derp:* fallback: specific regions are checked first,
	// then if none match, 0 is returned to let existing logic handle the wildcard.
	p, _ = parseConnPref("direct,derp:901,derp:900,derp:*,peer-relay")
	if selected := p.selectPreferredDERP(latency, 0, func(int) bool { return true }); selected != 901 {
		t.Errorf("expected 901 as first preferred (with wildcard fallback), got %d", selected)
	}
}

func TestConnPref_SelectPreferredDERP_CurrentHomeFallback(t *testing.T) {
	latency := map[int]time.Duration{
		1: 10 * time.Millisecond,
	}

	// Current home is in preferred list but not in latency map; should still try it
	p, _ := parseConnPref("direct,derp:900,peer-relay")
	if selected := p.selectPreferredDERP(latency, 900, func(int) bool { return true }); selected != 900 {
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
			p, _ := parseConnPref(tt.pref)
			if !tt.checkFn(p) {
				t.Errorf("check failed for pref=%q", tt.pref)
				t.Logf("  entries=%d, hasDirect=%v, hasAnyDERP=%v, hasPeerRelay=%v, derpOrder=%v",
					len(p.entries), p.hasDirect, p.hasAnyDERP, p.hasPeerRelay, p.derpOrder)
			}
		})
	}
}

func TestParseConnPref_Whitespace(t *testing.T) {
	p, _ := parseConnPref(" direct , derp:999 , peer-relay ")
	if len(p.entries) != 3 {
		t.Errorf("expected 3 entries with whitespace, got %d", len(p.entries))
	}
}

func TestConnPref_PreferredDERPRegions(t *testing.T) {
	// Any DERP => nil
	p, _ := parseConnPref("direct,derp:*,peer-relay")
	if got := p.preferredDERPRegions(); got != nil {
		t.Errorf("expected nil for any DERP, got %v", got)
	}

	// Specific DERP => ordered list
	p, _ = parseConnPref("direct,derp:901,derp:900")
	if got := p.preferredDERPRegions(); len(got) != 2 || got[0] != 901 || got[1] != 900 {
		t.Errorf("expected [901 900], got %v", got)
	}

	// No DERP => nil
	p, _ = parseConnPref("direct,peer-relay")
	if got := p.preferredDERPRegions(); got != nil {
		t.Errorf("expected nil for no DERP, got %v", got)
	}
}

func TestConnPref_SelectPreferredDERP_CurrentHomePreferred(t *testing.T) {
	latency := map[int]time.Duration{
		900: 100 * time.Millisecond,
		901: 5 * time.Millisecond,
	}
	p, _ := parseConnPref("direct,derp:900,derp:901,peer-relay")
	// Current home is 900, which is preferred over 901 even though 901 has lower latency
	if selected := p.selectPreferredDERP(latency, 900, func(int) bool { return true }); selected != 900 {
		t.Errorf("expected current home 900, got %d", selected)
	}
}

func TestConnPref_SelectPreferredDERP_RegionExists(t *testing.T) {
	latency := map[int]time.Duration{
		900: 10 * time.Millisecond,
	}
	p, _ := parseConnPref("derp:900,derp:901")

	// 900 has latency and exists: selected.
	if selected := p.selectPreferredDERP(latency, 0, func(rid int) bool { return rid == 900 }); selected != 900 {
		t.Errorf("expected 900, got %d", selected)
	}

	// Only 901 exists in the DERP map but has no latency data: still selected.
	if selected := p.selectPreferredDERP(latency, 0, func(rid int) bool { return rid == 901 }); selected != 901 {
		t.Errorf("expected 901 (exists, forced), got %d", selected)
	}

	// Neither preferred region exists in the DERP map: must NOT force-connect.
	if selected := p.selectPreferredDERP(latency, 0, func(int) bool { return false }); selected != 0 {
		t.Errorf("expected 0 when no preferred region exists, got %d", selected)
	}

	// Current home not preferred and does not exist: no selection.
	if selected := p.selectPreferredDERP(latency, 902, func(rid int) bool { return rid == 901 }); selected != 901 {
		t.Errorf("expected 901, got %d", selected)
	}
}

func TestConnPref_PeerRelayOnly(t *testing.T) {
	p, _ := parseConnPref("peer-relay")
	if p.directAllowed() {
		t.Error("direct should not be allowed")
	}
	if p.derpAllowed() {
		t.Error("derp should not be allowed")
	}
	if !p.allowPeerRelay() {
		t.Error("peer-relay should be allowed")
	}
	if p.preferDERPOverDirect() {
		t.Error("should not prefer DERP over direct")
	}
	if p.preferDERPOverPeerRelay() {
		t.Error("should not prefer DERP over peer-relay")
	}
}

func TestConnPref_DerpOnly(t *testing.T) {
	p, _ := parseConnPref("derp:901")
	if p.directAllowed() {
		t.Error("direct should not be allowed")
	}
	if !p.derpAllowed() {
		t.Error("derp should be allowed")
	}
	if p.allowPeerRelay() {
		t.Error("peer-relay should not be allowed")
	}
	if !p.derpRegionAllowed(901) {
		t.Error("region 901 should be allowed")
	}
	if p.derpRegionAllowed(902) {
		t.Error("region 902 should not be allowed")
	}
}

func TestPreferredDerpRegionForSend(t *testing.T) {
	defaultPref, _ := parseConnPref("")
	if got := preferredDerpRegionForSend(defaultPref, 1, 0); got != 1 {
		t.Errorf("default pref: want peer region 1, got %d", got)
	}

	derp901, _ := parseConnPref("derp:901")
	// Peer region in preference list: unchanged.
	if got := preferredDerpRegionForSend(derp901, 901, 901); got != 901 {
		t.Errorf("allowed region: want 901, got %d", got)
	}
	// Peer region NOT in list: replaced by our home.
	if got := preferredDerpRegionForSend(derp901, 902, 901); got != 901 {
		t.Errorf("non-allowed peer region: want our home 901, got %d", got)
	}
	// No home DERP available: no DERP.
	if got := preferredDerpRegionForSend(derp901, 902, 0); got != 0 {
		t.Errorf("no home: want 0, got %d", got)
	}

	// Specific + wildcard: specific regions win over wildcard.
	derpMix, _ := parseConnPref("derp:900,derp:*")
	if got := preferredDerpRegionForSend(derpMix, 900, 900); got != 900 {
		t.Errorf("specific region: want 900, got %d", got)
	}
	// A wildcard-covered peer region is allowed as-is.
	if got := preferredDerpRegionForSend(derpMix, 901, 900); got != 901 {
		t.Errorf("wildcard-covered peer region: want 901, got %d", got)
	}

	// Direct-only: DERP entirely disallowed.
	directOnly, _ := parseConnPref("direct")
	if got := preferredDerpRegionForSend(directOnly, 1, 1); got != 0 {
		t.Errorf("direct-only: want 0, got %d", got)
	}

	// Peer-relay only: DERP entirely disallowed.
	relayOnly, _ := parseConnPref("peer-relay")
	if got := preferredDerpRegionForSend(relayOnly, 1, 1); got != 0 {
		t.Errorf("peer-relay-only: want 0, got %d", got)
	}
}

func TestParseConnPref_RegionIDBounds(t *testing.T) {
	// 0, negative, and out-of-uint16-range region IDs are rejected.
	for _, tok := range []string{"derp:0", "derp:-1", "derp:70000", "derp:65536", "derp:abc"} {
		p, err := parseConnPref("derp:" + strings.TrimPrefix(tok, "derp:"))
		if err == nil {
			t.Errorf("%s: expected error, got pref %+v", tok, p)
		}
	}
	// Max valid region ID accepted.
	if _, err := parseConnPref("derp:65535"); err != nil {
		t.Errorf("derp:65535 should be accepted, got %v", err)
	}
}

func TestDerpRegionBanned(t *testing.T) {
	defaultPref, _ := parseConnPref("")
	if derpRegionBanned(defaultPref, 1) {
		t.Error("default pref should not ban any region")
	}

	derp901, _ := parseConnPref("derp:901")
	if derpRegionBanned(derp901, 901) {
		t.Error("allowed region 901 should not be banned")
	}
	if !derpRegionBanned(derp901, 902) {
		t.Error("region 902 should be banned")
	}

	// Wildcard: any region allowed.
	derpMix, _ := parseConnPref("derp:900,derp:*")
	if derpRegionBanned(derpMix, 900) || derpRegionBanned(derpMix, 901) {
		t.Error("wildcard pref should not ban any region")
	}

	// Direct-only and peer-relay-only: all regions banned.
	for _, pref := range []string{"direct", "peer-relay"} {
		p, _ := parseConnPref(pref)
		if !derpRegionBanned(p, 1) {
			t.Errorf("pref %q should ban all DERP regions", pref)
		}
	}
}
