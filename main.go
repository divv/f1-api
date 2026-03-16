package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The Magic Key: 9145 is the 2024 Australian Grand Prix Race (Fully Archived & Free)
const targetSession = "9145"

// --- 1. UI Styles ---
var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF1801")).MarginBottom(1)
	cardStyle  = lipgloss.NewStyle().Padding(1, 2).Margin(0, 1).BorderStyle(lipgloss.RoundedBorder()).Width(24)

	p1Style = cardStyle.Copy().BorderForeground(lipgloss.Color("#FFD700")).Foreground(lipgloss.Color("#FFD700"))
	p2Style = cardStyle.Copy().BorderForeground(lipgloss.Color("#C0C0C0")).Foreground(lipgloss.Color("#C0C0C0"))
	p3Style = cardStyle.Copy().BorderForeground(lipgloss.Color("#CD7F32")).Foreground(lipgloss.Color("#CD7F32"))

	statLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// --- 2. Data Models & State ---
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
	stats    []DriverStats
	err      error
	phase    string
	tick     bool
	simClock time.Time
	width    int
	carX     int
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

var httpClient = &http.Client{Timeout: 15 * time.Second}

// --- 3. Bubble Tea App Logic ---
func initialModel() model {
	return model{phase: "booting", width: 80, carX: 80}
}

func animCmd() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(time.Time) tea.Msg {
		return animTickMsg{}
	})
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchBootData, animCmd())
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
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case bootMsg:
		m.stats = msg.Stats
		// Skip the first 65 minutes (pre-race grid & formation lap) to jump straight into the racing action
		m.simClock = msg.StartTime.Add(65 * time.Minute)
		m.phase = "simulating"
		return m, tea.Batch(fetchSimData(m.stats, m.simClock), tickCmd())

	case tickMsg:
		if m.phase == "simulating" {
			m.tick = !m.tick
			return m, tea.Batch(fetchSimData(m.stats, m.simClock), tickCmd())
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
		return m, fetchBootData
	}
	return m, nil
}

func (m model) View() string {
	carPos := m.carX
	if carPos < 0 {
		carPos = 0
	}
	carLine := strings.Repeat(" ", carPos) + "🏎"

	if m.phase == "booting" {
		if m.err != nil {
			return fmt.Sprintf("%s\nAPI Error: %v\n\nRetrying in 5 seconds... (Press 'q' to quit)", carLine, m.err)
		}
		return fmt.Sprintf("%s\nFetching historical Australian GP data and preparing simulation...\n", carLine)
	}

	if len(m.stats) < 3 {
		return "Not enough data to construct a podium.\nPress 'q' to quit."
	}

	liveIcon := " "
	if m.tick {
		liveIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("●")
	}
	
	simTimeStr := m.simClock.Format("15:04:05")
	title := titleStyle.Render(fmt.Sprintf("🏎️  AUSTRALIAN GP SIMULATOR [%s UTC] %s", simTimeStr, liveIcon))

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
	return fmt.Sprintf("%s\n%s\n%s\n\nPress 'q' to quit.", carLine, title, podium)
}

// --- 4. Boot Phase: Get Positions, Drivers, and Start Time ---
func fetchBootData() tea.Msg {
	// 1. Session Time
	respSess, err := httpClient.Get("https://api.openf1.org/v1/sessions?session_key=" + targetSession)
	if err != nil { return errMsg(err) }
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
	if err != nil { return errMsg(err) }

	// 2. Positions
	respPos, err := httpClient.Get("https://api.openf1.org/v1/position?session_key=" + targetSession)
	if err != nil { return errMsg(err) }
	defer respPos.Body.Close()

	var positions []struct {
		DriverNumber int `json:"driver_number"`
		Position     int `json:"position"`
	}
	if err := json.NewDecoder(respPos.Body).Decode(&positions); err != nil { return errMsg(err) }

	currentPos := make(map[int]int)
	found := 0
	for i := len(positions) - 1; i >= 0; i-- {
		p := positions[i]
		if p.Position >= 1 && p.Position <= 3 {
			if _, exists := currentPos[p.Position]; !exists {
				currentPos[p.Position] = p.DriverNumber
				found++
				if found == 3 { break }
			}
		}
	}

	// 3. Driver Info
	respInfo, err := httpClient.Get("https://api.openf1.org/v1/drivers?session_key=" + targetSession)
	if err != nil { return errMsg(err) }
	defer respInfo.Body.Close()

	var driversInfo []struct {
		DriverNumber int    `json:"driver_number"`
		Acronym      string `json:"name_acronym"`
		TeamName     string `json:"team_name"`
	}
	if err := json.NewDecoder(respInfo.Body).Decode(&driversInfo); err != nil { return errMsg(err) }

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

// --- 5. Simulation Phase: 7-Second Ticks ---
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second*7, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fetchSimData(currentStats []DriverStats, currentClock time.Time) tea.Cmd {
	return func() tea.Msg {
		results := make([]DriverStats, len(currentStats))
		copy(results, currentStats)

		endTime := currentClock.Add(7 * time.Second)
		timeFilter := fmt.Sprintf("&date>=%s&date<%s", url.QueryEscape(currentClock.Format(time.RFC3339Nano)), url.QueryEscape(endTime.Format(time.RFC3339Nano)))

		// 1. CAR DATA
		urlCar := "https://api.openf1.org/v1/car_data?session_key=" + targetSession + timeFilter
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
		urlInt := "https://api.openf1.org/v1/intervals?session_key=" + targetSession + timeFilter
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
		urlLap := fmt.Sprintf("https://api.openf1.org/v1/laps?session_key=%s&date_start>=%s&date_start<%s", targetSession, url.QueryEscape(currentClock.Format(time.RFC3339Nano)), url.QueryEscape(endTime.Format(time.RFC3339Nano)))
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

// --- 6. Entry Point ---
func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
