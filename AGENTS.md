# plugin-androidtv Instructions

`plugin-androidtv` follows the reference runnable-module architecture.

- Keep `cmd/plugin-androidtv/main.go` as a thin wrapper only.
- Put runtime lifecycle and command wiring in `app/`.
- Keep private discovery logic under `internal/...`.
- Prefer testing `app/` and `internal/...`; keep `cmd` free of non-wrapper logic.
