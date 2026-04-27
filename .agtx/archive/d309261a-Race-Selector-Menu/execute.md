# Execute Summary: Race Selector Menu

## Changes

### `main.go` — sole file modified

**Imports added:** `"sort"`, `"strings"`

**Removed:** `const targetSession = "9145"` (hardcoded session)

**New styles added:**
- `selectedStyle` — bold red, used to highlight the cursor row in the race selector list

**New types added:**
- `RaceSession` — holds `Key`, `CountryName`, `Year`, `DateStart` for a race
- `racesMsg []RaceSession` — message carrying the fetched race list
- `racesErrMsg{err}` — message for fetch failures

**`model` struct — new fields:**
- `races []RaceSession` — list shown in the selector
- `racesCursor int` — which row is highlighted
- `selectedSession string` — session key of the chosen race
- `selectedName string` — human-readable race name for the simulation title

**`initialModel()`** — now returns `phase: "selecting"` instead of `"booting"`

**`Init()`** — now calls `fetchRaces` instead of the old `fetchBootData`

**`Update()` changes:**
- Key `q`/`ctrl+c` → quit (always)
- Keys `up`/`k`, `down`/`j` → move cursor in selecting phase
- Key `enter`/` ` → confirm selection, transition to `"booting"`, call `fetchBootData(sessionKey)`
- Key `r` → from simulating phase, return to selecting, re-fetch races
- New case `racesMsg` → store races, reset cursor
- New case `racesErrMsg` → store error for display
- `bootMsg` handler → calls `fetchSimData(m.selectedSession, ...)` (was global const)
- `tickMsg` handler → calls `fetchSimData(m.selectedSession, ...)` (was global const)
- `retryBootMsg` handler → calls `fetchBootData(m.selectedSession)` (was global const)

**`View()` changes:**
- New `"selecting"` branch: loading indicator, error display, scrollable list with `▶` cursor
- `"booting"` branch: now shows `m.selectedName` in the status text
- Simulation title now uses `strings.ToUpper(m.selectedName)` instead of `"AUSTRALIAN GP"`
- Footer now reads `Press 'q' to quit | 'r' to change race`

**`fetchRaces()` — new function:**
- Calls `GET /v1/sessions?session_type=Race`
- Decodes JSON with `session_key` as `int` (actual API type)
- Sorts by `DateStart` descending, takes first 25
- Returns `racesMsg`

**`fetchBootData`** — changed from `tea.Cmd` value to factory `func(sessionKey string) tea.Cmd`; all internal URL strings use `sessionKey` param

**`fetchSimData`** — added `sessionKey string` as first parameter; all internal URL strings use `sessionKey` param

## Testing

- `go build ./...` — clean compile, zero errors or warnings
- Logic review: state transitions `selecting → booting → simulating → (r) → selecting` all wired correctly
- Session key integer-to-string conversion handled explicitly in `fetchRaces` via `fmt.Sprintf("%d", ...)`
- Hotkey `r` is only active in `"simulating"` phase (guard in Update)
- Enter is only active in `"selecting"` phase with non-empty `races` (guard in Update)
