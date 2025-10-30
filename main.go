package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

const (
	padding         = 2
	maxWidth        = 80
	refreshInterval = time.Second
	labelWidth      = 15
)

// --- Message Types ---
type tickMsg time.Time

// --- Model ---
type model struct {
	cpuProgress  progress.Model
	memProgress  progress.Model
	diskProgress map[string]progress.Model // Currently unused, keep for potential future
	netProgress  map[string]progress.Model // Currently unused, keep for potential future

	// UI Size & Error
	width  int
	errMsg string
}

func NewModel() model {
	m := model{
		// Initialize progress bars
		cpuProgress: progress.New(progress.WithDefaultGradient()),
		memProgress: progress.New(progress.WithDefaultGradient()),
		// Initialize maps if needed later
		diskProgress: make(map[string]progress.Model),
		netProgress:  make(map[string]progress.Model),
	}

	// Set initial widths (will be updated by WindowSizeMsg)
	m.cpuProgress.Width = maxWidth
	m.memProgress.Width = maxWidth
	return m
}

// --- Bubble Tea Methods ---

func (m model) Init() tea.Cmd {
	return tickCmd()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" || msg.String() == "Q" {
			return m, tea.Quit
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		// Adjust barWidth calculation (temp only applies to GPU Util bar visually)
		barWidth := m.width - padding*2 - labelWidth - 1 // Label + Space + Bar + Space

		// Other bars use the standard width
		if barWidth < 10 {
			barWidth = 10
		}
		if barWidth > maxWidth {
			barWidth = maxWidth
		}
		m.cpuProgress.Width = barWidth
		m.memProgress.Width = barWidth

		return m, nil

	case tickMsg:
		var cmds []tea.Cmd
		var cmd tea.Cmd
		m.errMsg = "" // Clear errors at start of tick

		// --- CPU ---
		cpuPercentages, err := cpu.Percent(0, false)
		if err != nil {
			m.errMsg = m.appendError(m.errMsg, fmt.Sprintf("CPU Err: %v", err))
			log.Printf("CPU Err: %v", err)
		} else if len(cpuPercentages) > 0 {
			cmd = m.cpuProgress.SetPercent(cpuPercentages[0] / 100.0)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

		// --- Memory ---
		vmStat, err := mem.VirtualMemory()
		if err != nil {
			m.errMsg = m.appendError(m.errMsg, fmt.Sprintf("Mem Err: %v", err))
			log.Printf("Mem Err: %v", err)
		} else {
			cmd = m.memProgress.SetPercent(vmStat.UsedPercent / 100.0)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

		// Schedule next tick and batch commands
		batchCmds := []tea.Cmd{tickCmd()}
		batchCmds = append(batchCmds, cmds...)
		return m, tea.Batch(batchCmds...)

	case progress.FrameMsg:
		var cmds []tea.Cmd // Collect commands for further animation frames

		// --- Update Animation States ---
		// CPU
		newCPUModel, cmd := m.cpuProgress.Update(msg)
		if updatedModel, ok := newCPUModel.(progress.Model); ok {
			m.cpuProgress = updatedModel
		}
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

		// Memory
		newMemModel, cmd := m.memProgress.Update(msg)
		if updatedModel, ok := newMemModel.(progress.Model); ok {
			m.memProgress = updatedModel
		}
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

		return m, tea.Batch(cmds...)

	default:
		return m, nil
	}
}

// Helper to append errors without making the line too long
func (m *model) appendError(existingErr, newErr string) string {
	if existingErr == "" {
		return newErr
	}
	maxErrLen := m.width - padding*2 - len("Error: ")
	if maxErrLen < 20 {
		maxErrLen = 20
	}
	combined := existingErr + " | " + newErr
	if len(combined) > maxErrLen {
		combined = combined[:maxErrLen-3] + "..."
	}
	return combined
}

// --- View Function ---
func (m model) View() string {
	pad := strings.Repeat(" ", padding)
	view := "\n"

	// CPU
	view += pad + lipgloss.NewStyle().Width(labelWidth).Render("CPU:") + " " + m.cpuProgress.View() + "\n\n"
	// Memory
	view += pad + lipgloss.NewStyle().Width(labelWidth).Render("Memory:") + " " + m.memProgress.View()

	// Error Message
	if m.errMsg != "" {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Width(m.width - padding*2)
		view += pad + errorStyle.Render("Error: "+m.errMsg) + "\n\n"
	}

	return view
}

// --- Timer Command (Unchanged) ---
func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// --- Main Function (Mostly Unchanged) ---
func main() {
	// Configure logging to be discarded (no output)
	log.SetOutput(io.Discard)

	program := tea.NewProgram(NewModel())
	_, runErr := program.Run()
	if runErr != nil {
		// Still print critical errors to stderr so the user sees them
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", runErr)
		os.Exit(1)
	}
}
