// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package magicsock

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"tailscale.com/envknob"
	"tailscale.com/types/logger"
)

// connMethod is a method of connecting to a peer.
type connMethod int

const (
	connMethodInvalid   connMethod = iota
	connMethodDirect               // Direct UDP
	connMethodDERP                 // DERP relay (specific region or any)
	connMethodPeerRelay            // Peer relay (Geneve-encapsulated)
)

func (m connMethod) String() string {
	switch m {
	case connMethodDirect:
		return "direct"
	case connMethodDERP:
		return "derp"
	case connMethodPeerRelay:
		return "peer-relay"
	default:
		return "unknown"
	}
}

// connPrefEntry is a single entry in the connection preference list.
type connPrefEntry struct {
	method    connMethod
	regionIDs []int // for connMethodDERP: preferred region IDs; empty means any
}

// connPref is an ordered list of connection preferences.
// Entries earlier in the list have higher priority.
type connPref struct {
	entries []connPrefEntry

	// explicit is true when the user explicitly configured a preference
	// (non-empty TS_CONNECTION_PREFERENCE). The default and zero-value
	// preferences are not explicit and must behave exactly like upstream
	// Tailscale (in particular, no "prefer" reordering of trusted paths).
	explicit bool

	// Cached lookups for quick checks:
	hasDirect    bool
	hasPeerRelay bool
	hasAnyDERP   bool         // whether "derp:*" is in the list
	derpRegion   map[int]bool // specific DERP region IDs mentioned (only set when hasAnyDERP is false)
	derpOrder    []int        // DERP region IDs in preference order
}

// defaultConnPref returns the default preference (all methods, natural order).
func defaultConnPref() connPref {
	return connPref{
		entries: []connPrefEntry{
			{method: connMethodDirect},
			{method: connMethodDERP},
			{method: connMethodPeerRelay},
		},
		hasDirect:    true,
		hasAnyDERP:   true,
		hasPeerRelay: true,
	}
}

// parseConnPref parses a connection preference string.
// Format: comma-separated list of tokens:
//   - "direct"
//   - "derp:<region_id>"  (e.g. "derp:999")
//   - "derp:*"            (any DERP region)
//   - "peer-relay"
//
// Empty string returns the default preference.
// An invalid token returns an error and the default preference, so callers
// can log the problem instead of silently applying a looser policy than the
// user requested.
func parseConnPref(s string) (connPref, error) {
	if s == "" {
		return defaultConnPref(), nil
	}

	var p connPref
	for _, token := range strings.Split(s, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		entry, err := parseConnPrefToken(token)
		if err != nil {
			return defaultConnPref(), fmt.Errorf("invalid TS_CONNECTION_PREFERENCE token %q: %w", token, err)
		}
		p.explicit = true
		p.entries = append(p.entries, entry)
		switch entry.method {
		case connMethodDirect:
			p.hasDirect = true
		case connMethodPeerRelay:
			p.hasPeerRelay = true
		case connMethodDERP:
			if len(entry.regionIDs) == 0 {
				p.hasAnyDERP = true
			} else {
				if p.derpRegion == nil {
					p.derpRegion = make(map[int]bool)
				}
				for _, rid := range entry.regionIDs {
					if !p.derpRegion[rid] {
						p.derpRegion[rid] = true
						p.derpOrder = append(p.derpOrder, rid)
					}
				}
			}
		}
	}
	if len(p.entries) == 0 {
		return defaultConnPref(), nil
	}
	return p, nil
}

func parseConnPrefToken(token string) (connPrefEntry, error) {
	switch {
	case token == "direct":
		return connPrefEntry{method: connMethodDirect}, nil
	case token == "peer-relay":
		return connPrefEntry{method: connMethodPeerRelay}, nil
	case strings.HasPrefix(token, "derp:"):
		rest := strings.TrimPrefix(token, "derp:")
		if rest == "*" {
			return connPrefEntry{method: connMethodDERP}, nil
		}
		rid, err := strconv.Atoi(rest)
		if err != nil || rid <= 0 || rid > 65535 {
			return connPrefEntry{}, fmt.Errorf("invalid DERP region ID %q", rest)
		}
		return connPrefEntry{method: connMethodDERP, regionIDs: []int{rid}}, nil
	default:
		return connPrefEntry{}, fmt.Errorf("unknown connection preference token %q", token)
	}
}

var debugConnectionPreference = envknob.RegisterString("TS_CONNECTION_PREFERENCE")

// getConnPref returns the parsed connection preference from the environment.
// If the preference string is invalid, the default (all methods allowed)
// preference is returned and the problem is logged.
func getConnPref(logf logger.Logf) connPref {
	s := debugConnectionPreference()
	p, err := parseConnPref(s)
	if err != nil {
		logf("invalid connection preference: %v; using default (all methods allowed)", err)
	}
	return p
}

// isZero reports whether p is the zero value (uninitialized).
// The zero value is treated as the default preference.
func (p connPref) isZero() bool {
	return p.entries == nil
}

// allowPeerRelay reports whether peer relay is allowed by the preference.
func (p connPref) allowPeerRelay() bool {
	if p.isZero() {
		return true
	}
	return p.hasPeerRelay
}

// preferDERPOverDirect reports whether, based on the preference order,
// DERP should be used instead of a direct UDP path. Only meaningful for
// explicitly configured preferences; the default behaves like upstream
// (no reordering of trusted paths).
func (p connPref) preferDERPOverDirect() bool {
	if p.isZero() || !p.explicit {
		return false
	}
	return p.methodPosition(connMethodDERP) < p.methodPosition(connMethodDirect)
}

// preferDERPOverPeerRelay reports whether DERP should be used instead of a
// peer relay path. Only meaningful for explicitly configured preferences;
// the default behaves like upstream (no reordering of trusted paths).
func (p connPref) preferDERPOverPeerRelay() bool {
	if p.isZero() || !p.explicit {
		return false
	}
	return p.methodPosition(connMethodDERP) < p.methodPosition(connMethodPeerRelay)
}

// directAllowed reports whether direct UDP is allowed by the preference.
func (p connPref) directAllowed() bool {
	if p.isZero() {
		return true
	}
	return p.hasDirect
}

// derpAllowed reports whether any DERP method is allowed by the preference.
func (p connPref) derpAllowed() bool {
	if p.isZero() {
		return true
	}
	return p.hasAnyDERP || len(p.derpOrder) > 0
}

// methodPosition returns the index (0-based) of the first entry matching the given method,
// or a large number if the method is not in the preference list.
func (p connPref) methodPosition(method connMethod) int {
	for i, e := range p.entries {
		if e.method == method {
			return i
		}
	}
	return 999
}

// preferredDERPRegions returns the DERP region IDs in order of preference,
// or nil if any DERP is allowed (or default).
func (p connPref) preferredDERPRegions() []int {
	if p.isZero() {
		return nil
	}
	if p.hasAnyDERP {
		return nil
	}
	return p.derpOrder
}

// derpRegionAllowed checks whether the given DERP region ID is allowed
// by the connection preference.
func (p connPref) derpRegionAllowed(regionID int) bool {
	if p.isZero() {
		return true
	}
	if p.hasAnyDERP {
		return true
	}
	return p.derpRegion[regionID]
}

// preferredDerpRegionForSend returns the DERP region to use when sending to a
// peer whose home DERP region is peerRegion, honoring the preference.
//
// A peer's region that is not in the explicit preference list is replaced by
// our own home DERP (myDerp) rather than connecting to a non-preferred region.
// If DERP is entirely disallowed, or no fallback region is available, 0 is
// returned to mean "no DERP". The default (non-explicit) preference passes the
// peer region through unchanged.
func preferredDerpRegionForSend(pref connPref, peerRegion, myDerp int) int {
	if !pref.explicit {
		return peerRegion
	}
	if !pref.derpAllowed() {
		return 0
	}
	if len(pref.derpOrder) > 0 && !pref.derpRegionAllowed(peerRegion) {
		return myDerp
	}
	return peerRegion
}

// selectPreferredDERP applies DERP region ordering from the preference.
// Given the region latency map, the current home DERP, and a predicate that
// reports whether a region ID exists in the DERP map, it returns the best
// DERP region according to the preference, or 0 to let existing logic decide.
//
// The preference can have both specific regions (e.g. "derp:900") and
// a catch-all "derp:*". Specific regions are always preferred over the
// wildcard, regardless of their position in the list.
//
// A region is only ever returned if regionExists reports true for it;
// force-connecting to a region ID absent from the DERP map would select a
// home DERP that drops all traffic.
func (p connPref) selectPreferredDERP(regionLatency map[int]time.Duration, currentHome int, regionExists func(int) bool) int {
	if p.isZero() {
		return 0 // let existing logic handle it
	}

	if p.hasAnyDERP && len(p.derpOrder) == 0 {
		return 0 // let existing logic handle it
	}

	if len(p.derpOrder) > 0 {
		// Current home: keep if it's in our preferred list and reachable.
		for _, rid := range p.derpOrder {
			if rid == currentHome {
				if _, ok := regionLatency[rid]; ok && regionExists(rid) {
					return rid
				}
			}
		}

		// Find the first preferred region with latency data.
		for _, rid := range p.derpOrder {
			if _, ok := regionLatency[rid]; ok && regionExists(rid) {
				return rid
			}
		}

		// No specific preferred region is reachable; try current home as last resort.
		if currentHome != 0 && p.derpRegion[currentHome] && regionExists(currentHome) {
			return currentHome
		}

		// Even without latency data, force-connect to the first preferred
		// region that exists. The STUN probe might have failed (UDP blocked)
		// but TCP DERP may still work.
		for _, rid := range p.derpOrder {
			if regionExists(rid) {
				return rid
			}
		}

		// No preferred region exists in the DERP map; do not select any DERP.
		return 0
	}

	// If we have "any DERP" as fallback, return 0 to let existing netcheck logic decide.
	if p.hasAnyDERP {
		return 0
	}

	return 0
}
