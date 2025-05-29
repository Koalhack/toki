package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/v2/key"
	"github.com/charmbracelet/bubbles/v2/progress"
	"github.com/charmbracelet/bubbles/v2/timer"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	mcobra "github.com/muesli/mango-cobra"
	"github.com/muesli/roff"
	"github.com/spf13/cobra"
)

type model struct {
	name            string
	altscreen       bool
	startTimeFormat string
	durations       []time.Duration
	state           int
	passed          time.Duration
	start           time.Time
	timer           timer.Model
	progress        progress.Model
	quitting        bool
	interrupting    bool
}

func (m model) Init() tea.Cmd {
	return m.timer.Init()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case timer.TickMsg:
		var cmds []tea.Cmd
		var cmd tea.Cmd

		m.passed += m.timer.Interval
		pct := m.passed.Milliseconds() * 100 / m.durations[m.state].Milliseconds()
		cmds = append(cmds, m.progress.SetPercent(float64(pct)/100))

		m.timer, cmd = m.timer.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case tea.WindowSizeMsg:
		m.progress.SetWidth(msg.Width - padding*2 - 4)
		winHeight = msg.Height
		if !m.altscreen && m.progress.Width() > maxWidth {
			m.progress.SetWidth(maxWidth)
		}
		return m, nil

	case timer.StartStopMsg:
		var cmd tea.Cmd
		m.timer, cmd = m.timer.Update(msg)
		return m, cmd

	case timer.TimeoutMsg:
		if m.state == len(m.durations)-1 {
			m.quitting = true
			return m, tea.Quit
		}

		m.state++

		m.start = time.Now()
		m.passed = 0

		interval := timerInterval(m.durations[m.state])
		m.timer = timer.New(m.durations[m.state], timer.WithInterval(interval))

		return m, m.timer.Start()

	case progress.FrameMsg:
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if key.Matches(msg, quitKeys) {
			m.quitting = true
			return m, tea.Quit
		}
		if key.Matches(msg, intKeys) {
			m.interrupting = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.quitting || m.interrupting {
		return ""
	}

	var startTimeFormat string
	switch strings.ToLower(m.startTimeFormat) {
	case "24h":
		startTimeFormat = "15:04" // See: https://golang.cafe/blog/golang-time-format-example.html
	default:
		startTimeFormat = time.Kitchen
	}
	result := boldStyle.Render(m.start.Format(startTimeFormat))
	if m.name != "" {
		result += ": " + italicStyle.Render(m.name)
	}
	endTime := m.start.Add(m.durations[m.state])
	result += " - " + boldStyle.Render(endTime.Format(startTimeFormat)) +
		" - " + boldStyle.Render(m.timer.View()) +
		"\n" + m.progress.View()
	if m.altscreen {
		return altscreenStyle.
			MarginTop((winHeight - 2) / 2).
			Render(result)
	}
	return result
}

var (
	name            string
	altscreen       bool
	startTimeFormat string
	winHeight       int
	version         = "dev"
	quitKeys        = key.NewBinding(key.WithKeys("esc", "q"))
	intKeys         = key.NewBinding(key.WithKeys("ctrl+c"))
	altscreenStyle  = lipgloss.NewStyle().MarginLeft(padding)
	boldStyle       = lipgloss.NewStyle().Bold(true)
	italicStyle     = lipgloss.NewStyle().Italic(true)
)

const (
	padding  = 2
	maxWidth = 80
)

var rootCmd = &cobra.Command{
	Use:          "toki",
	Short:        "A timer with many features",
	Version:      version,
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		timerStringArray := splitTimerArgString(args[0])

		var durations []time.Duration
		for index, item := range timerStringArray {
			timerStringArray[index] = addSuffixIfArgIsNumber(item, "s")

			duration, err := time.ParseDuration(timerStringArray[index])
			if err != nil {
				return err
			}
			durations = append(durations, duration)
		}
		var opts []tea.ProgramOption
		if altscreen {
			opts = append(opts, tea.WithAltScreen())
		}
		interval := timerInterval(durations[0])
		m, err := tea.NewProgram(model{
			durations:       durations,
			state:           0,
			timer:           timer.New(durations[0], timer.WithInterval(interval)),
			progress:        progress.New(progress.WithDefaultGradient()),
			name:            name,
			altscreen:       altscreen,
			startTimeFormat: startTimeFormat,
			start:           time.Now(),
		}, opts...).Run()
		if err != nil {
			return err
		}
		if m.(model).interrupting {
			return fmt.Errorf("interrupted")
		}
		if name != "" {
			cmd.Printf("%s ", name)
		}
		cmd.Printf("finished!\n")
		return nil
	},
}

var manCmd = &cobra.Command{
	Use:                   "man",
	Short:                 "Generates man pages",
	SilenceUsage:          true,
	DisableFlagsInUseLine: true,
	Hidden:                true,
	Args:                  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		manPage, err := mcobra.NewManPage(1, rootCmd)
		if err != nil {
			return err
		}

		_, err = fmt.Fprint(os.Stdout, manPage.Build(roff.NewDocument()))
		return err
	},
}

func init() {
	rootCmd.Flags().StringVarP(&name, "name", "n", "", "timer name(s)")
	rootCmd.Flags().BoolVarP(&altscreen, "fullscreen", "f", false, "fullscreen")
	rootCmd.Flags().StringVarP(&startTimeFormat, "format", "", "", "Specify start time format, possible values: 24h, kitchen")

	rootCmd.AddCommand(manCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func timerInterval(d time.Duration) time.Duration {
	if d < time.Minute {
		return 100 * time.Millisecond
	}

	return time.Second
}

func addSuffixIfArgIsNumber(s string, suffix string) string {
	_, err := strconv.ParseFloat(s, 64)
	if err == nil {
		s = s + suffix
		return s
	}
	return s
}

func splitTimerArgString(s string) []string {
	const TIMER_ARG_SEP = "\\s*[\\s,-]\\s*"
	array := regexp.MustCompile(TIMER_ARG_SEP).Split(s, -1)
	return array
}
