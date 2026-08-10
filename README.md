# Tailscale — Dytchem Fork

**Fork with `--connection-preference` support for precise control
over DERP/direct/relay path selection.**

Upstream: https://tailscale.com — Private WireGuard® networks made easy

## Overview

This repository contains the majority of Tailscale's open source code,
plus Dytchem's connection-preference extension.
It includes the `tailscaled` daemon and
the `tailscale` CLI tool. The `tailscaled` daemon runs on Linux, Windows,
[macOS](https://tailscale.com/kb/1065/macos-variants/), and to varying degrees
on FreeBSD and OpenBSD. The Tailscale iOS and Android apps use this repo's
code, but this repo doesn't contain the mobile GUI code.

Other [Tailscale repos](https://github.com/orgs/tailscale/repositories) of note:

* the Android app is at https://github.com/tailscale/tailscale-android
* the Synology package is at https://github.com/tailscale/tailscale-synology
* the QNAP package is at https://github.com/tailscale/tailscale-qpkg
* the Chocolatey packaging is at https://github.com/tailscale/tailscale-chocolatey

For background on which parts of Tailscale are open source and why,
see [https://tailscale.com/opensource/](https://tailscale.com/opensource/).

## Dytchem Fork — Connection Preference

**GitHub**: https://github.com/Dytchem/tailscale

This fork adds `--connection-preference` / `TS_CONNECTION_PREFERENCE` to
control the priority ordering of connection methods (direct UDP, DERP relay,
peer relay). Unlike upstream Tailscale's automatic path selection, this allows
exact specification of which connection methods to use and in what order.

```
tailscaled --connection-preference=derp:902,derp:*,direct
```

### Syntax

- `direct` — Direct UDP only
- `derp:<region_id>` — Specific DERP region (e.g. `derp:902`)
- `derp:*` — Any DERP region
- `peer-relay` — Peer relay (Geneve-encapsulated)

Comma-separated ordered list. When a single method is specified, no fallback
to other methods occurs. Multiple methods create a priority chain.

Notes on DERP selection:

- Specific regions (`derp:<id>`) are always preferred over the wildcard
  (`derp:*`), regardless of their position in the list. The wildcard is a
  fallback for any region not explicitly listed.
- A region is only ever selected if it exists in the DERP map the control
  server advertised; a nonexistent region ID will never be force-connected to
  (which would drop all traffic). A missing preferred region logs an error
  and disables DERP entirely (or falls back to the wildcard, if present).
- An invalid token in the list logs an error and falls back to the default
  (all methods allowed) preference — the failure mode is "everything
  allowed", not "nothing allowed". A typo is loud (startup log) but
  permissive; double-check the value if you rely on it for isolation.

### Behavior details

- Any method not listed in the preference is disabled. For example
  `direct,derp:900` disables peer relay entirely (both discovery and use),
  and `derp:901` disables direct connections.
- When sending to a peer whose home DERP region is not in your preference
  list, your own home DERP region is used instead (or, early at startup
  before the first netcheck, no DERP until a home is selected).
- PINGs that arrive over a disallowed direct path get their PONG redirected
  through your home DERP; if DERP is also disallowed (e.g. `peer-relay`
  only), the PONG is dropped.
- Peer relay control-plane traffic (relay allocation requests) may travel
  over DERP regions that the preference forbids for user traffic: a relay
  server is only reachable on its home region, and an otherwise-allowed
  `peer-relay` method must be able to reach it.
- Incoming PINGs over peer relay (Geneve) are always answered over the same
  relay path; the preference governs which paths *this* node uses.
- WireGuard-only peers (no disco/DERP support) are exempt from the
  preference and are always reached directly.

### Example: Force DERP 902 only

```
tailscaled --connection-preference=derp:902
```

The node will exclusively use DERP region 902, refusing direct connections
and other DERP regions. Verified on Linux (userspace-networking) and FreeBSD
with three exit nodes in production.

## Using

We serve packages for a variety of distros and platforms at
[https://pkgs.tailscale.com](https://pkgs.tailscale.com/).

## Other clients

The [macOS, iOS, and Windows clients](https://tailscale.com/download)
use the code in this repository but additionally include small GUI
wrappers. The GUI wrappers on non-open source platforms are themselves
not open source.

## Building

We always require the latest Go release, currently Go 1.26. (While we build
releases with our [Go fork](https://github.com/tailscale/go/), its use is not
required.)

```
go install tailscale.com/cmd/tailscale{,d}
```

If you're packaging Tailscale for distribution, use `build_dist.sh`
instead, to burn commit IDs and version info into the binaries:

```
./build_dist.sh tailscale.com/cmd/tailscale
./build_dist.sh tailscale.com/cmd/tailscaled
```

If your distro has conventions that preclude the use of
`build_dist.sh`, please do the equivalent of what it does in your
distro's way, so that bug reports contain useful version information.

## Bugs

Please file any issues about this code or the hosted service on
[the issue tracker](https://github.com/tailscale/tailscale/issues).

## Contributing

PRs welcome! But please file bugs. Commit messages should [reference
bugs](https://docs.github.com/en/github/writing-on-github/autolinked-references-and-urls).

We require [Developer Certificate of
Origin](https://en.wikipedia.org/wiki/Developer_Certificate_of_Origin)
`Signed-off-by` lines in commits.

See [commit-messages.md](docs/commit-messages.md) (or skim `git log`) for our commit message style.

## About Us

[Tailscale](https://tailscale.com/) is primarily developed by the
people at https://github.com/orgs/tailscale/people. For other contributors,
see:

* https://github.com/tailscale/tailscale/graphs/contributors
* https://github.com/tailscale/tailscale-android/graphs/contributors

## Legal

WireGuard is a registered trademark of Jason A. Donenfeld.
