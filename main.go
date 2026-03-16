package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- 1. UI Styles ---
var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF1801")).MarginBottom(1)
	cardStyle  = lipgloss.NewStyle().Padding(1, 2).Margin(0, 1).BorderStyle(lipgloss.RoundedBorder()).Width(24)

	p1Style = cardStyle.Copy().BorderForeground(lipgloss.Color("#FFD700")).Foreground(lipgloss.Color("#FFD700"))
	p2Style = cardStyle.Copy().BorderForeground(lipgloss.Color("#C0C0C0")).Foreground(lipgloss.Color("#C0C0C0"))
	p3Style = cardStyle.Copy().BorderForeground(lipgloss.Color("#CD7F32")).Foreground(lipgloss.Color("#CD7F32"))

	statLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF1801"))
)

// --- 2. Data Models & State ---
type RaceSession struct {
	Key         string
	CountryName string
	Year        int
	DateStart   string
}

type DriverStats struct {
	Number  int
	Acronym string
	Team    string
	Speed   int
	Gear    int
	RPM     int
	Delta   float64
	LapTime float64
}

type model struct {
	// Race selector state
	races       []RaceSession
	racesCursor int

	// Selected race
	selectedSession string
	selectedName    string

	// Simulation state
	stats    []DriverStats
	err      error
	phase    string
	tick     bool
	simClock time.Time
	width    int
	carX     int
	tickHz   float64 // simulation tick rate in Hz
}

// Messages
type animTickMsg struct{}
type tickMsg time.Time
type bootData struct {
	Stats     []DriverStats
	StartTime time.Time
}
type bootMsg bootData
type dataMsg struct {
	Stats    []DriverStats
	NewClock time.Time
}
type errMsg error
type retryBootMsg struct{}
type racesMsg []RaceSession
type racesErrMsg struct{ err error }

var httpClient = &http.Client{Timeout: 15 * time.Second}

// --- 3. Bubble Tea App Logic ---
func initialModel() model {
	return model{phase: "selecting", width: 80, carX: 80, tickHz: 0.5}
}

func (m model) interval() time.Duration {
	return time.Duration(float64(time.Second) / m.tickHz)
}

func animCmd() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(time.Time) tea.Msg {
		return animTickMsg{}
	})
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchRaces, animCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case animTickMsg:
		m.carX--
		if m.carX < -2 {
			m.carX = m.width
		}
		return m, animCmd()

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
			} else if m.phase == "selecting" && m.err != nil {
				m.err = nil
				return m, fetchRaces
			}
		case "r":
			if m.phase == "simulating" || (m.phase == "selecting" && m.err != nil) {
				m.phase = "selecting"
				m.races = nil
				m.err = nil
				return m, fetchRaces
			}
		case "+", "=":
			if m.tickHz < 2.0 {
				m.tickHz = math.Round((m.tickHz+0.25)*100) / 100
			}
		case "-":
			if m.tickHz > 0.25 {
				m.tickHz = math.Round((m.tickHz-0.25)*100) / 100
			}
		}

	case racesMsg:
		m.races = []RaceSession(msg)
		m.racesCursor = 0
		return m, nil

	case racesErrMsg:
		m.err = msg.err
		return m, nil

	case bootMsg:
		m.stats = msg.Stats
		// Skip the first 65 minutes (pre-race grid & formation lap) to jump straight into the racing action
		m.simClock = msg.StartTime.Add(65 * time.Minute)
		m.phase = "simulating"
		return m, tea.Batch(fetchSimData(m.selectedSession, m.stats, m.simClock, m.interval()), tickCmd(m.interval()))

	case tickMsg:
		if m.phase == "simulating" {
			m.tick = !m.tick
			return m, tea.Batch(fetchSimData(m.selectedSession, m.stats, m.simClock, m.interval()), tickCmd(m.interval()))
		}

	case dataMsg:
		m.stats = msg.Stats
		m.simClock = msg.NewClock
		return m, nil

	case errMsg:
		m.err = msg
		if m.phase == "booting" {
			return m, tea.Tick(5*time.Second, func(time.Time) tea.Msg {
				return retryBootMsg{}
			})
		}
		return m, nil

	case retryBootMsg:
		m.err = nil
		return m, fetchBootData(m.selectedSession)
	}
	return m, nil
}

func (m model) View() string {
	if m.phase == "selecting" {
		if m.err != nil {
			return fmt.Sprintf("Failed to load races: %v\n\nPress Enter or 'r' to retry · 'q' to quit.", m.err)
		}
		if len(m.races) == 0 {
			return "Loading races...\n"
		}
		lines := []string{
			titleStyle.Render("🏎️  F1 RACE SELECTOR"),
			"",
			"Select a race to simulate  (↑/↓ or j/k · Enter to select · q to quit)",
			"",
		}
		for i, race := range m.races {
			label := fmt.Sprintf("%d  %s GP", race.Year, race.CountryName)
			if i == m.racesCursor {
				lines = append(lines, selectedStyle.Render("▶ "+label))
			} else {
				lines = append(lines, "  "+label)
			}
		}
		return strings.Join(lines, "\n")
	}

	carPos := m.carX
	if carPos < 0 {
		carPos = 0
	}
	carLine := strings.Repeat(" ", carPos) + "🏎"

	if m.phase == "booting" {
		if m.err != nil {
			return fmt.Sprintf("%s\nAPI Error: %v\n\nRetrying in 5 seconds... (Press 'q' to quit)", carLine, m.err)
		}
		return fmt.Sprintf("%s\nFetching %s data and preparing simulation...\n", carLine, m.selectedName)
	}

	if len(m.stats) < 3 {
		return "Not enough data to construct a podium.\nPress 'q' to quit."
	}

	liveIcon := " "
	if m.tick {
		liveIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("●")
	}

	simTimeStr := m.simClock.Format("15:04:05")
	title := titleStyle.Render(fmt.Sprintf("🏎️  %s SIMULATOR [%s UTC] [%.2f Hz] %s", strings.ToUpper(m.selectedName), simTimeStr, m.tickHz, liveIcon))

	cards := make([]string, 3)
	styles := []lipgloss.Style{p1Style, p2Style, p3Style}

	for i, driver := range m.stats {
		header := fmt.Sprintf("P%d %s\n#%d | %s\n", i+1, driver.Acronym, driver.Number, driver.Team)

		stats := fmt.Sprintf("%s %d km/h\n%s %d\n%s %d\n%s +%.3fs\n%s %.3fs",
			statLabelStyle.Render("Speed:"), driver.Speed,
			statLabelStyle.Render("Gear: "), driver.Gear,
			statLabelStyle.Render("RPM:  "), driver.RPM,
			statLabelStyle.Render("Delta:"), driver.Delta,
			statLabelStyle.Render("Lap:  "), driver.LapTime,
		)

		cards[i] = styles[i].Render(header + "\n" + stats)
	}

	podium := lipgloss.JoinHorizontal(lipgloss.Bottom, cards[1], cards[0], cards[2])
	return fmt.Sprintf("%s\n%s\n%s\n\nPress 'q' to quit | 'r' to change race | +/- to adjust speed", carLine, title, podium)
}

// --- 4. Race Selector: Fetch Last 25 Races ---
func fetchRaces() tea.Msg {
	resp, err := httpClient.Get("https://api.openf1.org/v1/sessions?session_type=Race")
	if err != nil {
		return racesErrMsg{err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return racesErrMsg{fmt.Errorf("OpenF1 API returned %s for /sessions", resp.Status)}
	}

	var raw []struct {
		SessionKey  int    `json:"session_key"`
		CountryName string `json:"country_name"`
		Year        int    `json:"year"`
		DateStart   string `json:"date_start"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return racesErrMsg{err}
	}

	sort.Slice(raw, func(i, j int) bool {
		return raw[i].DateStart > raw[j].DateStart
	})

	limit := 25
	if len(raw) < limit {
		limit = len(raw)
	}

	races := make([]RaceSession, limit)
	for i := 0; i < limit; i++ {
		races[i] = RaceSession{
			Key:         fmt.Sprintf("%d", raw[i].SessionKey),
			CountryName: raw[i].CountryName,
			Year:        raw[i].Year,
			DateStart:   raw[i].DateStart,
		}
	}

	return racesMsg(races)
}

// --- 5. Boot Phase: Get Positions, Drivers, and Start Time ---
func fetchBootData(sessionKey string) tea.Cmd {
	return func() tea.Msg {
		// 1. Session Time
		respSess, err := httpClient.Get("https://api.openf1.org/v1/sessions?session_key=" + sessionKey)
		if err != nil {
			return errMsg(err)
		}
		defer respSess.Body.Close()

		if respSess.StatusCode != http.StatusOK {
			return errMsg(fmt.Errorf("OpenF1 API returned %s for /sessions", respSess.Status))
		}

		var sessions []struct {
			DateStart string `json:"date_start"`
		}
		if err := json.NewDecoder(respSess.Body).Decode(&sessions); err != nil || len(sessions) == 0 {
			return errMsg(fmt.Errorf("could not parse session data"))
		}

		startTime, err := time.Parse(time.RFC3339Nano, sessions[0].DateStart)
		if err != nil {
			return errMsg(err)
		}

		// 2. Positions
		respPos, err := httpClient.Get("https://api.openf1.org/v1/position?session_key=" + sessionKey)
		if err != nil {
			return errMsg(err)
		}
		defer respPos.Body.Close()

		var positions []struct {
			DriverNumber int `json:"driver_number"`
			Position     int `json:"position"`
		}
		if err := json.NewDecoder(respPos.Body).Decode(&positions); err != nil {
			return errMsg(err)
		}

		currentPos := make(map[int]int)
		found := 0
		for i := len(positions) - 1; i >= 0; i-- {
			p := positions[i]
			if p.Position >= 1 && p.Position <= 3 {
				if _, exists := currentPos[p.Position]; !exists {
					currentPos[p.Position] = p.DriverNumber
					found++
					if found == 3 {
						break
					}
				}
			}
		}

		// 3. Driver Info
		respInfo, err := httpClient.Get("https://api.openf1.org/v1/drivers?session_key=" + sessionKey)
		if err != nil {
			return errMsg(err)
		}
		defer respInfo.Body.Close()

		var driversInfo []struct {
			DriverNumber int    `json:"driver_number"`
			Acronym      string `json:"name_acronym"`
			TeamName     string `json:"team_name"`
		}
		if err := json.NewDecoder(respInfo.Body).Decode(&driversInfo); err != nil {
			return errMsg(err)
		}

		infoMap := make(map[int]struct{ Acronym, Team string })
		for _, d := range driversInfo {
			infoMap[d.DriverNumber] = struct{ Acronym, Team string }{d.Acronym, d.TeamName}
		}

		var initialStats []DriverStats
		for i := 1; i <= 3; i++ {
			dNum := currentPos[i]
			info := infoMap[dNum]
			initialStats = append(initialStats, DriverStats{
				Number:  dNum,
				Acronym: info.Acronym,
				Team:    info.Team,
			})
		}

		return bootMsg{Stats: initialStats, StartTime: startTime}
	}
}

// --- 6. Simulation Phase ---
func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fetchSimData(sessionKey string, currentStats []DriverStats, currentClock time.Time, interval time.Duration) tea.Cmd {
	return func() tea.Msg {
		results := make([]DriverStats, len(currentStats))
		copy(results, currentStats)

		endTime := currentClock.Add(interval)
		timeFilter := fmt.Sprintf("&date>=%s&date<%s", url.QueryEscape(currentClock.Format(time.RFC3339Nano)), url.QueryEscape(endTime.Format(time.RFC3339Nano)))

		// 1. CAR DATA
		urlCar := "https://api.openf1.org/v1/car_data?session_key=" + sessionKey + timeFilter
		if resp, err := httpClient.Get(urlCar); err == nil && resp.StatusCode == 200 {
			var data []struct {
				DriverNumber int `json:"driver_number"`
				Speed        int `json:"speed"`
				Gear         int `json:"n_gear"`
				RPM          int `json:"rpm"`
			}
			if json.NewDecoder(resp.Body).Decode(&data) == nil {
				for i := range results {
					for j := len(data) - 1; j >= 0; j-- {
						if data[j].DriverNumber == results[i].Number {
							results[i].Speed = data[j].Speed
							results[i].Gear = data[j].Gear
							results[i].RPM = data[j].RPM
							break
						}
					}
				}
			}
			resp.Body.Close()
		}

		// 2. INTERVALS
		urlInt := "https://api.openf1.org/v1/intervals?session_key=" + sessionKey + timeFilter
		if resp, err := httpClient.Get(urlInt); err == nil && resp.StatusCode == 200 {
			var iData []struct {
				DriverNumber int     `json:"driver_number"`
				Interval     float64 `json:"interval"`
			}
			if json.NewDecoder(resp.Body).Decode(&iData) == nil {
				for i := range results {
					for j := len(iData) - 1; j >= 0; j-- {
						if iData[j].DriverNumber == results[i].Number {
							results[i].Delta = iData[j].Interval
							break
						}
					}
				}
			}
			resp.Body.Close()
		}

		// 3. LAPS
		urlLap := fmt.Sprintf("https://api.openf1.org/v1/laps?session_key=%s&date_start>=%s&date_start<%s", sessionKey, url.QueryEscape(currentClock.Format(time.RFC3339Nano)), url.QueryEscape(endTime.Format(time.RFC3339Nano)))
		if resp, err := httpClient.Get(urlLap); err == nil && resp.StatusCode == 200 {
			var lData []struct {
				DriverNumber int     `json:"driver_number"`
				LapDuration  float64 `json:"lap_duration"`
			}
			if json.NewDecoder(resp.Body).Decode(&lData) == nil {
				for i := range results {
					for j := len(lData) - 1; j >= 0; j-- {
						if lData[j].DriverNumber == results[i].Number {
							results[i].LapTime = lData[j].LapDuration
							break
						}
					}
				}
			}
			resp.Body.Close()
		}

		return dataMsg{Stats: results, NewClock: endTime}
	}
}

// --- 7. Entry Point ---
func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
