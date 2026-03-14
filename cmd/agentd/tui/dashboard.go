package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/AnantSingh1510/agentd/controlplane/api/proto"
	"github.com/AnantSingh1510/agentd/divergence"
)

var (
	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1)

	styleTitle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	styleMuted = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	styleGreen = lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
	styleAmber = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleRed   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleBlue  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	styleHeader = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			PaddingBottom(0)

	styleMetricVal = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255"))

	styleMetricLabel = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				PaddingLeft(1)

	styleIdle = styleGreen
	styleBusy = styleAmber
	styleFull = styleRed
)


type tickMsg time.Time
type dataMsg struct {
	workers  []*proto.WorkerInfo
	stats    map[string]int
	dissents int
	err      error
}

type feedEntry struct {
	ts   time.Time
	tag  string
	text string
}

type Model struct {
	serverAddr string
	conn       *grpc.ClientConn
	client     proto.AgentDClient
	divStore   *divergence.Store

	workers      []*proto.WorkerInfo
	dissentStats map[string]int
	totalDissents int
	feed         []feedEntry
	lastWorkers  map[string]*proto.WorkerInfo

	spinner  spinner.Model
	started  time.Time
	lastErr  string
	ready    bool
	width    int
}

func NewModel(serverAddr string) *Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styleBlue

	return &Model{
		serverAddr:  serverAddr,
		spinner:     s,
		started:     time.Now(),
		lastWorkers: make(map[string]*proto.WorkerInfo),
		feed:        make([]feedEntry, 0),
	}
}

func (m *Model) connect() error {
	conn, err := grpc.NewClient(m.serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}
	m.conn = conn
	m.client = proto.NewAgentDClient(conn)

	div, err := divergence.New("localhost:6379")
	if err == nil {
		m.divStore = div
	}
	return nil
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		tick(),
	)
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			if m.conn != nil {
				m.conn.Close()
			}
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tickMsg:
		return m, tea.Batch(tick(), fetchData(m.client, m.divStore))

	case dataMsg:
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.ready = false
			return m, nil
		}

		m.lastErr = ""
		m.ready = true
		m.totalDissents = msg.dissents
		m.dissentStats = msg.stats

		newWorkerMap := make(map[string]*proto.WorkerInfo)
		for _, w := range msg.workers {
			newWorkerMap[w.Id] = w
		}
		for id := range newWorkerMap {
			if _, existed := m.lastWorkers[id]; !existed {
				m.addFeed("+ worker", fmt.Sprintf("%s  capacity=%d", id, newWorkerMap[id].Capacity))
			}
		}
		for id := range m.lastWorkers {
			if _, exists := newWorkerMap[id]; !exists {
				m.addFeed("- worker", fmt.Sprintf("%s removed", id))
			}
		}

		m.workers = msg.workers
		m.lastWorkers = newWorkerMap
	}

	return m, nil
}

func (m *Model) addFeed(tag, text string) {
	m.feed = append([]feedEntry{{ts: time.Now(), tag: tag, text: text}}, m.feed...)
	if len(m.feed) > 10 {
		m.feed = m.feed[:10]
	}
}

func fetchData(client proto.AgentDClient, div *divergence.Store) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return dataMsg{err: fmt.Errorf("not connected")}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := client.ListWorkers(ctx, &proto.ListWorkersRequest{})
		if err != nil {
			return dataMsg{err: err}
		}

		stats := make(map[string]int)
		total := 0
		if div != nil {
			ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel2()
			stats, _ = div.Stats(ctx2)
			for _, v := range stats {
				total += v
			}
		}

		return dataMsg{
			workers:  resp.Workers,
			stats:    stats,
			dissents: total,
		}
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString(m.renderHeader())
	b.WriteString("\n")
	b.WriteString(m.renderMetrics())
	b.WriteString("\n")
	b.WriteString(m.renderWorkers())
	b.WriteString("\n")
	b.WriteString(m.renderDivergence())
	b.WriteString("\n")
	b.WriteString(m.renderFeed())
	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return b.String()
}

func (m Model) renderHeader() string {
	uptime := time.Since(m.started).Round(time.Second)
	status := styleGreen.Render("● connected")
	if !m.ready {
		status = styleRed.Render("● " + m.serverAddr + " unreachable")
	}

	left := styleTitle.Render("agentd") + styleMuted.Render("  top")
	right := styleMuted.Render(fmt.Sprintf("uptime %s  %s", uptime, status))

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) renderMetrics() string {
	totalCap := 0
	totalActive := 0
	for _, w := range m.workers {
		totalCap += int(w.Capacity)
		totalActive += int(w.Active)
	}

	metrics := []struct{ val, label string }{
		{fmt.Sprintf("%d", len(m.workers)), "workers"},
		{fmt.Sprintf("%d/%d", totalActive, totalCap), "active/capacity"},
		{fmt.Sprintf("%d", m.totalDissents), "dissents stored"},
		{fmt.Sprintf("%d", len(m.dissentStats)), "models tracked"},
	}

	cols := make([]string, len(metrics))
	for i, metric := range metrics {
		cols[i] = styleMetricVal.Render(metric.val) +
			styleMetricLabel.Render(metric.label)
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(m.width/4).Render(cols[0]),
		lipgloss.NewStyle().Width(m.width/4).Render(cols[1]),
		lipgloss.NewStyle().Width(m.width/4).Render(cols[2]),
		lipgloss.NewStyle().Width(m.width/4).Render(cols[3]),
	)

	return styleBorder.Width(m.width - 4).Render(row)
}

func (m Model) renderWorkers() string {
	header := styleHeader.Render(
		fmt.Sprintf("  %-28s  %-8s  %-10s  %s",
			"worker id", "capacity", "active", "load"),
	)

	rows := []string{header, styleDim.Render(strings.Repeat("─", m.width-6))}

	if len(m.workers) == 0 {
		rows = append(rows, styleMuted.Render("  no workers registered"))
	}

	for _, w := range m.workers {
		pct := 0
		if w.Capacity > 0 {
			pct = int(float64(w.Active) / float64(w.Capacity) * 100)
		}

		bar := renderBar(pct, 24)
		statusStyle := styleIdle
		statusText := "idle"
		if pct > 0 && pct < 100 {
			statusStyle = styleBusy
			statusText = "busy"
		} else if pct >= 100 {
			statusStyle = styleFull
			statusText = "full"
		}

		row := fmt.Sprintf("  %-28s  %-8d  %-10s  %s %s",
			w.Id,
			w.Capacity,
			statusStyle.Render(fmt.Sprintf("%d/%d", w.Active, w.Capacity)),
			bar,
			statusStyle.Render(statusText),
		)
		rows = append(rows, row)
	}

	content := strings.Join(rows, "\n")
	label := styleMuted.Render("workers")
	return label + "\n" + styleBorder.Width(m.width-4).Render(content)
}

func (m Model) renderDivergence() string {
	if len(m.dissentStats) == 0 {
		label := styleMuted.Render("divergence")
		content := styleMuted.Render("  no dissents recorded yet — run agentd negotiate to seed the dataset")
		return label + "\n" + styleBorder.Width(m.width-4).Render(content)
	}

	rows := []string{}
	for model, count := range m.dissentStats {
		bar := renderBar(count*10, 20)
		rows = append(rows,
			fmt.Sprintf("  %-20s  %s  %s",
				styleBlue.Render(model),
				bar,
				styleAmber.Render(fmt.Sprintf("%d dissents", count)),
			),
		)
	}

	label := styleMuted.Render("divergence by model")
	return label + "\n" + styleBorder.Width(m.width-4).Render(strings.Join(rows, "\n"))
}

func (m Model) renderFeed() string {
	rows := []string{}
	for _, e := range m.feed {
		ts := styleMuted.Render(e.ts.Format("15:04:05"))
		tag := styleBlue.Render(fmt.Sprintf("%-10s", e.tag))
		rows = append(rows, fmt.Sprintf("  %s  %s  %s", ts, tag, e.text))
	}
	if len(rows) == 0 {
		rows = append(rows, styleMuted.Render("  waiting for events..."))
	}

	label := styleMuted.Render("event feed")
	return label + "\n" + styleBorder.Width(m.width-4).Render(strings.Join(rows, "\n"))
}

func (m Model) renderFooter() string {
	return styleMuted.Render("  q quit  •  refreshes every second")
}

func (m *Model) Connect() error {
	return m.connect()
}

func renderBar(pct, width int) string {
	if pct > 100 {
		pct = 100
	}
	filled := pct * width / 100
	if filled > width {
		filled = width
	}

	color := "76"
	if pct > 50 {
		color = "214"
	}
	if pct > 80 {
		color = "196"
	}

	bar := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).
		Render(strings.Repeat("█", filled))
	empty := styleDim.Render(strings.Repeat("░", width-filled))
	return "[" + bar + empty + "]"
}