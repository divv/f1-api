# Execution Summary

## Changes

**`main.go`** — all changes in a single file:

- Added `"strings"` to imports
- Added `width int` and `carX int` fields to `model` struct to track terminal width and car X position
- Added `animTickMsg` message type for the 60ms animation timer
- Added `animCmd()` function that fires every 60ms
- Updated `initialModel()` to set `width: 80, carX: 80` as defaults
- Updated `Init()` to `tea.Batch(fetchBootData, animCmd())` so animation starts immediately
- Added `tea.WindowSizeMsg` handler in `Update` to track real terminal width
- Added `animTickMsg` handler in `Update`: decrements `carX` each tick; wraps back to `m.width` when it goes below -2 (fully off-screen left)
- Updated `View()` to compute `carLine` (spaces + `🏎` emoji) and prepend it to all view states (booting, simulating)

## Testing

- `go build ./...` — compiles cleanly, no errors or warnings
- Logic review: car starts at `carX = width` (right edge), moves left by 1 column per 60ms tick (~16 columns/second on an 80-col terminal, full pass ≈ 5 seconds), wraps seamlessly back to the right edge after it exits the left side
- `tea.WindowSizeMsg` keeps the track width accurate on resize
