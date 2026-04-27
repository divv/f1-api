# Research: Configurable Tick Rate

## Task
The race telemetry simulation should run at a default 0.5 Hz rate, adjustable with +/- keys in 0.25 Hz increments.

## Relevant Files

### `main.go` (only source file)
The entire application lives in a single ~323-line Go file. Key sections:

- **`tickCmd()` (line 234–238)**: Currently hardcoded to fire every 7 seconds. This is the tick interval to make configurable.
- **`fetchSimData()` (line 240–314)**: Uses a hardcoded `7 * time.Second` window for the API time range filter. This must match the tick interval.
- **`model` struct (line 42–48)**: Holds simulation state. A `tickInterval` field needs to be added here.
- **`Update()` (line 75–114)**: Handles key events (`"q"`, `"ctrl+c"`). The `+` and `-` key handlers need to be added here.
- **`View()` (line 116–155)**: Renders the TUI. Should display the current tick rate so the user can see what it's set to.
- **`initialModel()` (line 67–69)**: Creates the initial model. Needs to set a default tick interval of 2 seconds (0.5 Hz = 1 tick per 2 seconds).

## Architecture

The app is a standard **Bubble Tea** (charmbracelet/bubbletea) TUI application with a single model:

```
Init() → fetchBootData → bootMsg
                              ↓
                   fetchSimData() + tickCmd()
                              ↓
                   tickMsg → fetchSimData() + tickCmd()  (loop)
                              ↓
                   dataMsg → update stats + simClock
```

The tick loop is self-referential: each `tickMsg` triggers both a new `fetchSimData` and a new `tickCmd()`. The interval is determined by `tickCmd()`.

**Current hardcoded values:**
- `tickCmd()`: `time.Second * 7` — tick fires every 7 seconds
- `fetchSimData()`: `endTime := currentClock.Add(7 * time.Second)` — API query window is 7 seconds

Both values must be parameterized together, as they represent the same concept: "how much simulation time passes per tick."

**Note on Hz vs interval:**
- 0.5 Hz = 1 tick every 2 seconds
- 0.25 Hz steps → valid rates: 0.25, 0.50, 0.75, 1.0 Hz → intervals: 4s, 2s, 1.33s, 1s

However, the "seconds" here are **wall-clock seconds** between UI refreshes. The `simClock` advances by the tick interval each tick (simulated time = real time in this design). The API query window equals the tick interval.

Wait — re-reading the code: currently ticks are every 7 **wall-clock** seconds and the sim clock advances 7 seconds per tick. So real-time = sim-time (1:1 ratio). A 0.5 Hz tick rate means 2 wall-clock seconds per tick, advancing 2 simulated seconds per tick. This is significantly faster than the current 7s default.

## Complexity

**Simple change** — the scope is well-contained:

1. Add `tickInterval time.Duration` to the `model` struct
2. Set default `tickInterval = 2 * time.Second` in `initialModel()`
3. Update `tickCmd()` to accept `time.Duration` parameter (or make it a method on model)
4. Update `fetchSimData()` to accept the interval as parameter
5. Add `+`/`-` key handlers in `Update()` to adjust `tickInterval` by 0.25 Hz increments, with a minimum floor (e.g., 0.25 Hz = 4s interval)
6. Pass `m.tickInterval` when calling both `tickCmd()` and `fetchSimData()`
7. Display the current Hz rate in `View()`

## Open Questions

1. **Hz increment math**: 0.25 Hz steps mean interval steps are not uniform:
   - 0.25 Hz = 4.000s
   - 0.50 Hz = 2.000s
   - 0.75 Hz = 1.333s
   - 1.00 Hz = 1.000s
   Should the step be in Hz (non-uniform intervals) or uniform time steps (e.g., ±0.5s)? The task says "0.25hz at a time" so Hz-based stepping is the intent.

2. **Maximum rate**: No upper bound specified. A reasonable cap might be 2 Hz (0.5s interval) or 4 Hz (0.25s) to avoid hammering the API.

3. **Minimum rate**: No lower bound specified. At 0.25 Hz the interval is 4 seconds. Should there be a minimum?

4. **Current 7s tick**: The existing `tickCmd` uses 7 seconds. Moving to 2s default is a significant change in behavior — is this intentional? The task says "default 0.5hz rate."

5. **Display**: Should the rate display in the title bar alongside the sim clock? The task doesn't specify the UI placement.
