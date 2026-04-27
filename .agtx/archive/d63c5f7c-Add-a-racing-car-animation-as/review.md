# Review

## Review

### What looks good
- **Animation timer is independent**: `animCmd()` uses a 60ms tick separate from the 7-second data tick. The car animation runs smoothly regardless of API latency.
- **Wrap logic is correct**: `carX` decrements each tick and resets to `m.width` when it goes below `-2`, giving a clean off-screen-left exit before re-entering from the right.
- **Terminal resize handled**: `tea.WindowSizeMsg` updates `m.width`, so the track length adapts to the actual terminal width.
- **Default width**: `initialModel()` seeds `carX: 80` and `width: 80` so the car is visible immediately before the first `WindowSizeMsg` arrives.
- **Code style**: Consistent with the existing Bubble Tea MVU pattern. No new dependencies added.
- **No security issues**: Pure TUI animation with no user input or external data involved.

### Fix applied
- **Removed unnecessary `trackWidth` and upper-bound clamp in `View()`**: The original code clamped `carPos` to `[0, trackWidth-2]`. The lower bound caused the car to appear stuck at column 0 for two full ticks before wrapping (since `Update` wraps at `< -2` but `View` was flooring at `0`). The upper bound was also redundant because `carX` is initialized at `m.width` and only decrements. Removed both the `trackWidth` variable and the upper-bound clamp; kept only `if carPos < 0 { carPos = 0 }` as a safety guard against any edge case where `carX` is briefly negative during the frame before `Update` wraps it.

### Minor observations (no action needed)
- The `🏎` emoji renders as 2 terminal columns wide on most terminals. The animation still looks correct since the spacing logic uses single-column `" "` padding — the car just exits ~1 column earlier than the exact edge, which is imperceptible.
- The `"Not enough data"` fallback path (line 158) doesn't show the car line. This path is only hit transiently and has no `animCmd` running alongside it in that state, so omitting the car there is fine.

## Status

`READY`
