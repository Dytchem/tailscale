// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package magicsock

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"tailscale.com/envknob"
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
func parseConnPref(s string) connPref {
	if s == "" {
		return defaultConnPref()
	}

	var p connPref
	for _, token := range strings.Split(s, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		entry, err := parseConnPrefToken(token)
		if err != nil {
			return defaultConnPref()
		}
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
		return defaultConnPref()
	}
	return p
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
		if err != nil || rid <= 0 {
			return connPrefEntry{}, fmt.Errorf("invalid DERP region ID %q", rest)
		}
		return connPrefEntry{method: connMethodDERP, regionIDs: []int{rid}}, nil
	default:
		return connPrefEntry{}, fmt.Errorf("unknown connection preference token %q", token)
	}
}

var debugConnectionPreference = envknob.RegisterString("TS_CONNECTION_PREFERENCE")

// getConnPref returns the parsed connection preference from the environment.
// It is safe for concurrent use.
func getConnPref() connPref {
	s := debugConnectionPreference()
	return parseConnPref(s)
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
// DERP should be preferred over a direct UDP path.
func (p connPref) preferDERPOverDirect() bool {
	if p.isZero() {
		return false
	}
	return p.methodPosition(connMethodDERP) < p.methodPosition(connMethodDirect)
}

// preferDERPOverPeerRelay reports whether DERP should be preferred over peer relay.
func (p connPref) preferDERPOverPeerRelay() bool {
	if p.isZero() {
		return true
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

// selectPreferredDERP applies DERP region ordering from the preference.
// Given the region latency map and current home DERP, it returns the best
// DERP region according to the preference, or 0 to let existing logic decide.
//
// The preference can have both specific regions (e.g. "derp:900") and
// a catch-all "derp:*". This function handles the case where specific
// regions should be preferred over the wildcard.
func (p connPref) selectPreferredDERP(regionLatency map[int]time.Duration, currentHome int) int {
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
				if _, ok := regionLatency[rid]; ok {
					return rid
				}
			}
		}

		// Find the first preferred region with latency data.
		for _, rid := range p.derpOrder {
			if _, ok := regionLatency[rid]; ok {
				return rid
			}
		}

		// No specific preferred region is reachable; try current home as last resort.
		if currentHome != 0 && p.derpRegion[currentHome] {
			return currentHome
		}

		// Even without latency data, force-connect to the first preferred region.
		// The STUN probe might have failed (UDP blocked) but TCP DERP may still work.
		return p.derpOrder[0]
	}

	// If we have "any DERP" as fallback, return 0 to let existing netcheck logic decide.
	if p.hasAnyDERP {
		return 0
	}

	return 0
}
