# Plan: Race Selector Menu

## Analysis

Single-file Go TUI app (`main.go`) using:
- `charmbracelet/bubbletea` — Elm-style state machine with phases
- `charmbracelet/lipgloss` — terminal styling
- OpenF1 public API (`api.openf1.org/v1`) — F1 telemetry data

Current app flow:
```
init → "booting" (fetch session 9145) → "simulating" (7s tick loop)
```

The session key is hardcoded as `const targetSession = "9145"`. All API calls in `fetchBootData` and `fetchSimData` reference this constant.

The `model` struct has a `phase` string field driving all state. This is the natural extension point.

OpenF1 sessions endpoint: `GET /v1/sessions?session_type=Race` returns all races with fields: `session_key`, `session_name`, `country_name`, `year`, `date_start`.

## Plan

### Step 1 — Add `RaceSession` data type and message types

Add:
```go
type RaceSession struct {
    Key         string
    CountryName string
    Year        int
    DateStart   string
}
type racesMsg  []RaceSession
type racesErrMsg struct{ err error }
```

### Step 2 — Add new fields to `model`

```go
type model struct {
    // existing fields...
    races           []RaceSession
    racesCursor     int
    selectedSession string
    selectedName    string  // for dynamic title in simulation view
}
```

### Step 3 — Add `fetchRaces()` command

Queries `https://api.openf1.org/v1/sessions?session_type=Race`, decodes all sessions, sorts by `date_start` descending, takes the last 25. Returns `racesMsg`.

### Step 4 — Change initial phase and `Init()`

```go
func initialModel() model {
    return model{phase: "selecting"}
}

func (m model) Init() tea.Cmd {
    return fetchRaces
}
```

### Step 5 — Parameterize `fetchBootData`

Change from a `tea.Cmd` value to a factory function:
```go
func fetchBootData(sessionKey string) tea.Cmd {
    return func() tea.Msg { ... }
}
```
All internal references to `targetSession` become `sessionKey`. Remove the global `const targetSession`.

### Step 6 — Parameterize `fetchSimData`

Already a factory (`func fetchSimData(stats, clock) tea.Cmd`). Add `sessionKey string` as first argument. Update the one call site in `Update()`.

Also store `sessionKey` on the model so the tick loop can pass it:
```go
// In Update, case bootMsg:
return m, tea.Batch(fetchSimData(m.selectedSession, m.stats, m.simClock), tickCmd())
// In Update, case tickMsg:
return m, tea.Batch(fetchSimData(m.selectedSession, m.stats, m.simClock), tickCmd())
```

### Step 7 — `Update()` additions

**New message handling:**
```go
case racesMsg:
    m.races = []RaceSession(msg)
    m.racesCursor = 0
    return m, nil

case racesErrMsg:
    m.err = msg.err
    return m, nil
```

**Key handling in `"selecting"` phase:**
```go
case tea.KeyMsg:
    switch msg.String() {
    case "q", "ctrl+c":
        return m, tea.Quit
    case "up", "k":
        if m.phase == "selecting" && m.racesCursor > 0 {
            m.racesCursor--
        }
    case "down", "j":
        if m.phase == "selecting" && m.racesCursor < len(m.races)-1 {
            m.racesCursor++
        }
    case "enter", " ":
        if m.phase == "selecting" && len(m.races) > 0 {
            race := m.races[m.racesCursor]
            m.selectedSession = race.Key
            m.selectedName = fmt.Sprintf("%d %s", race.Year, race.CountryName)
            m.phase = "booting"
            m.err = nil
            return m, fetchBootData(m.selectedSession)
        }
    case "r":
        if m.phase == "simulating" {
            m.phase = "selecting"
            m.races = nil   // triggers re-fetch
            return m, fetchRaces
        }
    }
```

**Booting → Simulating transition** — update `bootMsg` handler to use `m.selectedSession`:
```go
case bootMsg:
    ...
    return m, tea.Batch(fetchSimData(m.selectedSession, m.stats, m.simClock), tickCmd())
```

**Retry on boot error** — update to use `m.selectedSession`:
```go
case retryBootMsg:
    m.err = nil
    return m, fetchBootData(m.selectedSession)
```

### Step 8 — `View()` for `"selecting"` phase

Add a branch before the existing phases:
```go
if m.phase == "selecting" {
    if len(m.races) == 0 {
        if m.err != nil {
            return fmt.Sprintf("Failed to load races: %v\n\nPress 'q' to quit.", m.err)
        }
        return "Loading races...\n"
    }
    // Render scrollable list of races with cursor indicator
    // Highlighted item shown with a distinct color/marker
    lines := []string{
        titleStyle.Render("🏎️  F1 RACE SELECTOR"),
        "",
        "Select a race to simulate (↑/↓ or j/k to navigate, Enter to select):",
        "",
    }
    for i, race := range m.races {
        label := fmt.Sprintf("%d  %s GP", race.Year, race.CountryName)
        if i == m.racesCursor {
            lines = append(lines, selectedStyle.Render("▶ " + label))
        } else {
            lines = append(lines, "  " + label)
        }
    }
    lines = append(lines, "", "Press 'q' to quit.")
    return strings.Join(lines, "\n")
}
```

Add a `selectedStyle` lipgloss style (bold, red foreground).

### Step 9 — Dynamic title in simulation view

Change hardcoded `"AUSTRALIAN GP SIMULATOR"` to:
```go
title := titleStyle.Render(fmt.Sprintf("🏎️  %s SIMULATOR [%s UTC] %s", strings.ToUpper(m.selectedName), simTimeStr, liveIcon))
```

Add `"strings"` to imports.

Also add `"r" to open Race Selector` hint in the simulation view footer:
```
Press 'q' to quit | 'r' to change race
```

### Step 10 — Add `selectedStyle` to styles block

```go
selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF1801"))
```

### Order of edits (single file)

1. Add `"strings"` to imports
2. Remove `const targetSession`
3. Add `selectedStyle` var
4. Add `RaceSession`, `racesMsg`, `racesErrMsg` types to data models section
5. Add `races`, `racesCursor`, `selectedSession`, `selectedName` fields to `model`
6. Update `initialModel()` — phase `"selecting"`
7. Update `Init()` — call `fetchRaces`
8. Update `Update()` — key handling, new message cases, boot/tick handlers
9. Update `View()` — add selecting branch, update simulation title and footer
10. Add `fetchRaces()` function
11. Update `fetchBootData` → factory function with `sessionKey string` param
12. Update `fetchSimData` → add `sessionKey string` as first param, update internals

## Risks

- **API pagination**: `GET /v1/sessions?session_type=Race` returns all races ever. The response could be large (100+ records). We fetch all and take the last 25 after sorting — this is fine for a one-time load.
- **Re-entering selector**: When pressing `r` from simulation, we re-fetch the races list. This means a brief "Loading races..." screen. Acceptable UX.
- **Session keys**: We assume `session_key` is a string in the API (it's actually an integer in the JSON). Need to decode it as `int` and convert to string for use in URL params.
- **Retrying boot on error**: The retry path must use `m.selectedSession` not the old constant — covered in Step 7.
- **The 65-minute skip**: Currently hardcoded in `bootMsg` handling. This is a race-specific heuristic — leave it as-is for now since task doesn't address it.
- **`tickMsg` handler**: Currently calls `fetchSimData(m.stats, m.simClock)` — must be updated to pass `m.selectedSession`.
