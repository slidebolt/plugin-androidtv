### `plugin-androidtv` repository

#### Project Overview

This repository contains a Slidebolt plugin that discovers Android TV / Google TV devices on the local network via mDNS (`_googlecast._tcp`) and exposes them as plugin devices.

#### Architecture

- `main.go` implements the plugin lifecycle and merges discovered devices with existing state using `runner.ReconcileDevice` (additive refresh semantics).
- `discovery.go` runs `avahi-browse -rt _googlecast._tcp`, parses service records, filters to likely TV devices, and produces deterministic device IDs.
- `discovery_test.go` validates parser behavior and TV filtering logic.

#### Discovery Behavior

- Discovery source: `avahi-browse` service scan for `_googlecast._tcp`
- TV heuristics:
  - Cast TXT record `rr=AndroidNativeApp`, or
  - model/name indicators such as `bravia`, `android`, `google tv`, `smart tv`, `sony`, `tcl`, `hisense`
- Device IDs are normalized to `androidtv-<source>`

#### Notes

- Discovery is best-effort. If `avahi-browse` is unavailable, the plugin keeps existing devices and still returns the core management device.
- Entity command/control behavior remains minimal and can be expanded after discovery is finalized.
