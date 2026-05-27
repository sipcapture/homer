package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/passwordhash"
)

// ---- Flags -----------------------------------------------------------------

// WizardFlags holds flags for the "homer wizard" subcommand.
type WizardFlags struct {
	Output  string // output file path
	Profile string // non-interactive preset name
}

type wizardFlagRefs struct {
	Output  *string
	Profile *string
}

// RegisterWizardFlags creates a FlagSet for "homer wizard" subcommand.
func RegisterWizardFlags() (*flag.FlagSet, *wizardFlagRefs) {
	fs := flag.NewFlagSet("wizard", flag.ExitOnError)
	refs := &wizardFlagRefs{}
	refs.Output = fs.String("output", "homer.json", "output config file path")
	refs.Profile = fs.String("profile", "", "non-interactive preset: all-in-one, writer, edge, coordinator, node")
	return fs, refs
}

// ParseWizardFlags extracts WizardFlags from parsed flag refs.
func ParseWizardFlags(refs *wizardFlagRefs) WizardFlags {
	return WizardFlags{
		Output:  *refs.Output,
		Profile: *refs.Profile,
	}
}

// RunWizardCmd is the entry point for "homer wizard".
func RunWizardCmd(f WizardFlags) error {
	// Non-interactive preset mode
	if f.Profile != "" {
		return runPresetConfig(f.Profile, f.Output)
	}

	// Interactive TUI wizard
	m := newWizardModel(f.Output)
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return fmt.Errorf("wizard: %w", err)
	}

	// Check if wizard was completed (not cancelled)
	if wm, ok := result.(wizardModel); ok && wm.done {
		return nil
	}
	return nil
}

// ---- Non-interactive presets -----------------------------------------------

func runPresetConfig(profile, outputPath string) error {
	var cfg config.Config
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "all-in-one", "all":
		cfg = presetAllInOne()
	case "writer":
		cfg = presetWriter()
	case "edge":
		cfg = presetEdge()
	case "coordinator":
		cfg = presetCoordinator()
	case "node":
		cfg = presetNode()
	default:
		return fmt.Errorf("unknown profile %q (available: all-in-one, writer, edge, coordinator, node)", profile)
	}

	return saveConfig(cfg, outputPath)
}

func presetAllInOne() config.Config {
	cfg := baseConfig()
	cfg.Ingest.Enable = true
	cfg.Storage.Enable = true
	cfg.Node.Enable = true
	cfg.Coordinator.Enable = true
	cfg.Coordinator.Nodes = []config.NodeEndpoint{{
		Name: "local", Host: "127.0.0.1", Port: 50051,
	}}
	return cfg
}

func presetWriter() config.Config {
	cfg := baseConfig()
	cfg.Ingest.Enable = true
	cfg.Storage.Enable = true
	cfg.Node.Enable = false
	cfg.Coordinator.Enable = false
	return cfg
}

func presetEdge() config.Config {
	cfg := baseConfig()
	cfg.Ingest.Enable = true
	cfg.Storage.Enable = true
	cfg.Node.Enable = true
	cfg.Coordinator.Enable = false
	return cfg
}

func presetCoordinator() config.Config {
	cfg := baseConfig()
	cfg.Ingest.Enable = false
	cfg.Storage.Enable = false
	cfg.Node.Enable = false
	cfg.Coordinator.Enable = true
	cfg.Coordinator.Nodes = []config.NodeEndpoint{{
		Name: "node1", Host: "10.0.0.1", Port: 50051,
	}}
	return cfg
}

func presetNode() config.Config {
	cfg := baseConfig()
	cfg.Ingest.Enable = false
	cfg.Storage.Enable = false
	cfg.Node.Enable = true
	cfg.Coordinator.Enable = false
	return cfg
}

func baseConfig() config.Config {
	return config.Config{
		Ingest: config.IngestConfig{
			WorkerCount: 8,
			QueueSize:   80000,
			UDP: config.UDPServerConfig{
				Enable: true, Host: "0.0.0.0", Port: 9060, Multicore: true,
			},
			TCP: config.TCPServerConfig{
				Enable: false, Host: "0.0.0.0", Port: 9060,
			},
			HTTP: config.HTTPServerConfig{
				Enable: true, Host: "0.0.0.0", Port: 9080,
				ReadTimeout: 30, WriteTimeout: 30,
			},
			HEP: config.HEPConfig{
				HepV2Enable: true, HepV3Enable: true, ProtobufEnable: true,
			},
		},
		Storage: config.StorageConfig{
			DuckLake: config.DuckLakeConfig{
				CatalogType:   "sqlite",
				CatalogPath:   "/data/homer/homer_catalog.sqlite",
				DataPath:      "/data/homer/parquet",
				LakeName:      "homer_lake",
				BatchSize:     10000,
				FlushInterval: 30,
				Compaction: config.CompactionConfig{
					Enable:           true,
					CheckIntervalSec: 1800,
					RetentionDays:    30,
				},
			},
		},
		Node: config.NodeConfig{
			FlightServer: config.FlightServerConfig{
				Enable: true, Host: "0.0.0.0", Port: 50051, MaxMessageSize: 16777216,
			},
			DuckLake: config.DuckLakeConfig{
				CatalogType: "sqlite",
				CatalogPath: "/data/homer/homer_catalog.sqlite",
				DataPath:    "/data/homer/parquet",
				LakeName:    "homer_lake",
			},
		},
		Coordinator: config.CoordinatorConfig{
			HTTPServer: config.CoordinatorHTTPServerConfig{
				Enable: true, Host: "0.0.0.0", Port: 8080,
				ReadTimeout: 30, WriteTimeout: 30,
			},
			SettingsDBPath: "/var/lib/homer/homer_settings.duckdb",
			JWT: config.JWTConfig{
				Secret:      randomHex(32),
				ExpireHours: 24,
			},
			Auth: config.AuthConfig{
				Type:                   "internal",
				AuthFromInternalString: true,
				AdminUser:              "admin",
				AdminPasswordHash:      config.DefaultInternalAuthPasswordHash,
			},
		},
		Log: config.LogConfig{
			Level:  "info",
			Output: []string{"stdout"},
			Stdout: config.StdoutLogConfig{Colored: true},
		},
		Prometheus: config.PrometheusConfig{
			Enable: true, Host: "0.0.0.0", Port: 9090, Path: "/metrics",
		},
	}
}

// ---- TUI wizard model ------------------------------------------------------

// wizard step indices
const (
	wizStepProfile = iota
	wizStepIngest
	wizStepStorage
	wizStepNode
	wizStepCoordinator
	wizStepSystem
	wizStepReview
	wizNumSteps
)

// wizard field indices (flat list across all steps)
const (
	// Profile step
	wfProfile = iota

	// Ingest step
	wfIngestEnable
	wfUDPPort
	wfTCPEnable
	wfHTTPPort
	wfWorkerCount
	wfQueueSize

	// Storage step
	wfStorageEnable
	wfCatalogType
	wfCatalogPath
	wfDataPath
	wfRetentionDays
	wfBatchSize

	// Node step
	wfNodeEnable
	wfFlightPort
	wfAuthToken

	// Coordinator step
	wfCoordEnable
	wfCoordPort
	wfNodeHost
	wfNodePort
	wfJWTSecret
	wfAdminUser
	wfAdminPass
	wfSettingsDB

	// System step
	wfLogLevel
	wfPrometheusEnable
	wfPrometheusPort

	// Review step
	wfOutputPath

	wizNumFields
)

type wizardModel struct {
	inputs   []textinput.Model
	step     int
	focused  int // focused field within current step
	done     bool
	saved    bool
	savedMsg string
	errMsg   string
	output   string // output file path
}

// stepFields maps each step to its field indices.
var stepFields = [wizNumSteps][]int{
	wizStepProfile:     {wfProfile},
	wizStepIngest:      {wfIngestEnable, wfUDPPort, wfTCPEnable, wfHTTPPort, wfWorkerCount, wfQueueSize},
	wizStepStorage:     {wfStorageEnable, wfCatalogPath, wfDataPath, wfRetentionDays, wfBatchSize},
	wizStepNode:        {wfNodeEnable, wfFlightPort, wfAuthToken},
	wizStepCoordinator: {wfCoordEnable, wfCoordPort, wfNodeHost, wfNodePort, wfJWTSecret, wfAdminUser, wfAdminPass, wfSettingsDB},
	wizStepSystem:      {wfLogLevel, wfPrometheusEnable, wfPrometheusPort},
	wizStepReview:      {wfOutputPath},
}

var stepNames = [wizNumSteps]string{
	"Deployment Profile",
	"Ingest Module",
	"Storage Module",
	"Node Module",
	"Coordinator Module",
	"System Settings",
	"Review & Save",
}

var wizTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).MarginBottom(1)
var wizStepStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
var wizHelpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1)
var wizSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
var wizErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)

func newWizardModel(outputPath string) wizardModel {
	inputs := make([]textinput.Model, wizNumFields)
	for i := range inputs {
		t := textinput.New()
		t.Cursor.Style = cursorStyle
		t.CharLimit = 256
		inputs[i] = t
	}

	// Profile
	inputs[wfProfile].Prompt = "Profile:         "
	inputs[wfProfile].Placeholder = "all-in-one (all-in-one, writer, edge, coordinator, node, custom)"
	inputs[wfProfile].SetValue("all-in-one")

	// Ingest
	inputs[wfIngestEnable].Prompt = "Enable Ingest:   "
	inputs[wfIngestEnable].Placeholder = "yes/no"
	inputs[wfIngestEnable].SetValue("yes")

	inputs[wfUDPPort].Prompt = "UDP Port:        "
	inputs[wfUDPPort].Placeholder = "9060"
	inputs[wfUDPPort].SetValue("9060")

	inputs[wfTCPEnable].Prompt = "Enable TCP:      "
	inputs[wfTCPEnable].Placeholder = "yes/no"
	inputs[wfTCPEnable].SetValue("no")

	inputs[wfHTTPPort].Prompt = "HTTP Port:       "
	inputs[wfHTTPPort].Placeholder = "9080"
	inputs[wfHTTPPort].SetValue("9080")

	inputs[wfWorkerCount].Prompt = "Worker Count:    "
	inputs[wfWorkerCount].Placeholder = "8 (0 = auto)"
	inputs[wfWorkerCount].SetValue("8")

	inputs[wfQueueSize].Prompt = "Queue Size:      "
	inputs[wfQueueSize].Placeholder = "80000"
	inputs[wfQueueSize].SetValue("80000")

	// Storage
	inputs[wfStorageEnable].Prompt = "Enable Storage:  "
	inputs[wfStorageEnable].Placeholder = "yes/no"
	inputs[wfStorageEnable].SetValue("yes")

	// DuckLake catalog — sqlite; wfCatalogType slot kept for stable field indices.

	inputs[wfCatalogPath].Prompt = "Catalog Path:    "
	inputs[wfCatalogPath].Placeholder = "/data/homer/homer_catalog.sqlite"
	inputs[wfCatalogPath].SetValue("/data/homer/homer_catalog.sqlite")

	inputs[wfDataPath].Prompt = "Data Path:       "
	inputs[wfDataPath].Placeholder = "/data/homer/parquet"
	inputs[wfDataPath].SetValue("/data/homer/parquet")

	inputs[wfRetentionDays].Prompt = "Retention Days:  "
	inputs[wfRetentionDays].Placeholder = "30 (0 = unlimited)"
	inputs[wfRetentionDays].SetValue("30")

	inputs[wfBatchSize].Prompt = "Batch Size:      "
	inputs[wfBatchSize].Placeholder = "10000"
	inputs[wfBatchSize].SetValue("10000")

	// Node
	inputs[wfNodeEnable].Prompt = "Enable Node:     "
	inputs[wfNodeEnable].Placeholder = "yes/no"
	inputs[wfNodeEnable].SetValue("yes")

	inputs[wfFlightPort].Prompt = "FlightSQL Port:  "
	inputs[wfFlightPort].Placeholder = "50051"
	inputs[wfFlightPort].SetValue("50051")

	inputs[wfAuthToken].Prompt = "Auth Token:      "
	inputs[wfAuthToken].Placeholder = "(empty = no auth)"
	inputs[wfAuthToken].SetValue("")

	// Coordinator
	inputs[wfCoordEnable].Prompt = "Enable Coord:    "
	inputs[wfCoordEnable].Placeholder = "yes/no"
	inputs[wfCoordEnable].SetValue("yes")

	inputs[wfCoordPort].Prompt = "API Port:        "
	inputs[wfCoordPort].Placeholder = "8080"
	inputs[wfCoordPort].SetValue("8080")

	inputs[wfNodeHost].Prompt = "Node Host:       "
	inputs[wfNodeHost].Placeholder = "127.0.0.1 (FlightSQL node to query)"
	inputs[wfNodeHost].SetValue("127.0.0.1")

	inputs[wfNodePort].Prompt = "Node Port:       "
	inputs[wfNodePort].Placeholder = "50051"
	inputs[wfNodePort].SetValue("50051")

	inputs[wfJWTSecret].Prompt = "JWT Secret:      "
	inputs[wfJWTSecret].Placeholder = "(auto-generated if empty)"
	inputs[wfJWTSecret].SetValue("")

	inputs[wfAdminUser].Prompt = "Admin User:      "
	inputs[wfAdminUser].Placeholder = "admin"
	inputs[wfAdminUser].SetValue("admin")

	inputs[wfAdminPass].Prompt = "Admin Password:  "
	inputs[wfAdminPass].Placeholder = "sipcapture"
	inputs[wfAdminPass].SetValue("sipcapture")
	inputs[wfAdminPass].EchoMode = textinput.EchoPassword

	inputs[wfSettingsDB].Prompt = "Settings DB:     "
	inputs[wfSettingsDB].Placeholder = "/var/lib/homer/homer_settings.duckdb"
	inputs[wfSettingsDB].SetValue("/var/lib/homer/homer_settings.duckdb")

	// System
	inputs[wfLogLevel].Prompt = "Log Level:       "
	inputs[wfLogLevel].Placeholder = "info (debug, info, warn, error)"
	inputs[wfLogLevel].SetValue("info")

	inputs[wfPrometheusEnable].Prompt = "Prometheus:      "
	inputs[wfPrometheusEnable].Placeholder = "yes/no"
	inputs[wfPrometheusEnable].SetValue("yes")

	inputs[wfPrometheusPort].Prompt = "Prometheus Port: "
	inputs[wfPrometheusPort].Placeholder = "9090"
	inputs[wfPrometheusPort].SetValue("9090")

	// Review
	inputs[wfOutputPath].Prompt = "Output File:     "
	inputs[wfOutputPath].Placeholder = "homer.json"
	inputs[wfOutputPath].SetValue(outputPath)

	// Apply profile defaults
	applyProfileDefaults(inputs[:], "all-in-one")

	// Focus first field
	fields := stepFields[0]
	inputs[fields[0]].Focus()
	inputs[fields[0]].PromptStyle = focusedStyle
	inputs[fields[0]].TextStyle = focusedStyle

	return wizardModel{
		inputs: inputs,
		step:   wizStepProfile,
		output: outputPath,
	}
}

func applyProfileDefaults(inputs []textinput.Model, profile string) {
	switch strings.ToLower(profile) {
	case "all-in-one", "all":
		inputs[wfIngestEnable].SetValue("yes")
		inputs[wfStorageEnable].SetValue("yes")
		inputs[wfNodeEnable].SetValue("yes")
		inputs[wfCoordEnable].SetValue("yes")
	case "writer":
		inputs[wfIngestEnable].SetValue("yes")
		inputs[wfStorageEnable].SetValue("yes")
		inputs[wfNodeEnable].SetValue("no")
		inputs[wfCoordEnable].SetValue("no")
	case "edge":
		inputs[wfIngestEnable].SetValue("yes")
		inputs[wfStorageEnable].SetValue("yes")
		inputs[wfNodeEnable].SetValue("yes")
		inputs[wfCoordEnable].SetValue("no")
	case "coordinator":
		inputs[wfIngestEnable].SetValue("no")
		inputs[wfStorageEnable].SetValue("no")
		inputs[wfNodeEnable].SetValue("no")
		inputs[wfCoordEnable].SetValue("yes")
	case "node":
		inputs[wfIngestEnable].SetValue("no")
		inputs[wfStorageEnable].SetValue("no")
		inputs[wfNodeEnable].SetValue("yes")
		inputs[wfCoordEnable].SetValue("no")
	}
}

// ---- tea.Model implementation ----------------------------------------------

func (m wizardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			if m.step > 0 && !m.saved {
				m.step--
				m.focused = 0
				m.errMsg = ""
				return m, m.wizFocusField()
			}
			return m, tea.Quit

		case "tab", "down":
			if m.saved {
				return m, nil
			}
			fields := stepFields[m.step]
			if m.focused < len(fields)-1 {
				m.focused++
				return m, m.wizFocusField()
			}

		case "shift+tab", "up":
			if m.saved {
				return m, nil
			}
			if m.focused > 0 {
				m.focused--
				return m, m.wizFocusField()
			}

		case "enter":
			if m.saved {
				return m, tea.Quit
			}

			// On profile step, apply defaults when moving to next step
			if m.step == wizStepProfile {
				applyProfileDefaults(m.inputs, m.inputs[wfProfile].Value())
			}

			// On review step, generate and save
			if m.step == wizStepReview {
				cfg := m.buildConfigFromInputs()
				outPath := m.inputs[wfOutputPath].Value()
				if outPath == "" {
					outPath = "homer.json"
				}
				if err := saveConfig(cfg, outPath); err != nil {
					m.errMsg = fmt.Sprintf("Error: %v", err)
					return m, nil
				}
				m.saved = true
				m.done = true
				m.savedMsg = fmt.Sprintf("Config saved to %s", outPath)
				return m, nil
			}

			// Move to next step, skipping disabled modules
			m.step++
			m.focused = 0
			m.errMsg = ""

			// Skip disabled modules
			for m.step < wizStepSystem {
				skip := false
				switch m.step {
				case wizStepIngest:
					skip = !parseBool(m.inputs[wfIngestEnable].Value())
				case wizStepStorage:
					skip = !parseBool(m.inputs[wfStorageEnable].Value())
				case wizStepNode:
					skip = !parseBool(m.inputs[wfNodeEnable].Value())
				case wizStepCoordinator:
					skip = !parseBool(m.inputs[wfCoordEnable].Value())
				}
				if !skip {
					break
				}
				m.step++
			}

			return m, m.wizFocusField()
		}
	}

	// Update the focused input
	if !m.saved {
		fields := stepFields[m.step]
		if m.focused < len(fields) {
			idx := fields[m.focused]
			var cmd tea.Cmd
			m.inputs[idx], cmd = m.inputs[idx].Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m *wizardModel) wizFocusField() tea.Cmd {
	fields := stepFields[m.step]
	cmds := make([]tea.Cmd, 0)
	for fi, idx := range fields {
		if fi == m.focused {
			cmds = append(cmds, m.inputs[idx].Focus())
			m.inputs[idx].PromptStyle = focusedStyle
			m.inputs[idx].TextStyle = focusedStyle
		} else {
			m.inputs[idx].Blur()
			m.inputs[idx].PromptStyle = noStyle
			m.inputs[idx].TextStyle = noStyle
		}
	}
	return tea.Batch(cmds...)
}

func (m wizardModel) View() string {
	var b strings.Builder

	b.WriteString(wizTitleStyle.Render("Homer Configuration Wizard"))
	b.WriteString("\n")

	// Progress bar
	progress := fmt.Sprintf("Step %d/%d", m.step+1, wizNumSteps)
	bar := strings.Repeat("=", (m.step+1)*3) + strings.Repeat("-", (wizNumSteps-m.step-1)*3)
	b.WriteString(fmt.Sprintf("  [%s] %s\n", bar, progress))

	b.WriteString("\n")
	b.WriteString(wizStepStyle.Render(stepNames[m.step]))
	b.WriteString("\n\n")

	if m.saved {
		b.WriteString(wizSuccessStyle.Render(m.savedMsg))
		b.WriteString("\n\n")

		// Show preview of generated config
		cfg := m.buildConfigFromInputs()
		data, _ := json.MarshalIndent(cfg, "", "  ")
		preview := string(data)
		lines := strings.Split(preview, "\n")
		if len(lines) > 30 {
			preview = strings.Join(lines[:30], "\n") + "\n  ... (truncated)"
		}
		b.WriteString(preview)
		b.WriteString("\n\n")
		b.WriteString(wizHelpStyle.Render("Press Enter or Esc to exit"))
		return b.String()
	}

	// Show review: full config preview
	if m.step == wizStepReview {
		cfg := m.buildConfigFromInputs()
		data, _ := json.MarshalIndent(cfg, "", "  ")
		preview := string(data)
		lines := strings.Split(preview, "\n")
		if len(lines) > 25 {
			preview = strings.Join(lines[:25], "\n") + "\n  ... (truncated)"
		}
		b.WriteString(preview)
		b.WriteString("\n\n")
	}

	// Render fields for current step
	fields := stepFields[m.step]
	for _, idx := range fields {
		b.WriteString(m.inputs[idx].View())
		b.WriteString("\n")
	}

	if m.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(wizErrorStyle.Render(m.errMsg))
	}

	b.WriteString("\n")
	nav := "Tab/Shift+Tab: navigate  |  Enter: next step  |  Esc: back"
	if m.step == wizStepReview {
		nav = "Enter: save config  |  Esc: back"
	}
	b.WriteString(wizHelpStyle.Render(nav))

	return b.String()
}

// ---- Config builder --------------------------------------------------------

func (m wizardModel) buildConfigFromInputs() config.Config {
	ingestEnable := parseBool(m.inputs[wfIngestEnable].Value())
	storageEnable := parseBool(m.inputs[wfStorageEnable].Value())
	nodeEnable := parseBool(m.inputs[wfNodeEnable].Value())
	coordEnable := parseBool(m.inputs[wfCoordEnable].Value())
	promEnable := parseBool(m.inputs[wfPrometheusEnable].Value())

	udpPort := parseInt(m.inputs[wfUDPPort].Value(), 9060)
	httpPort := parseInt(m.inputs[wfHTTPPort].Value(), 9080)
	workerCount := parseInt(m.inputs[wfWorkerCount].Value(), 8)
	queueSize := parseInt(m.inputs[wfQueueSize].Value(), 80000)
	batchSize := parseInt(m.inputs[wfBatchSize].Value(), 10000)
	retentionDays := parseInt(m.inputs[wfRetentionDays].Value(), 30)
	flightPort := parseInt(m.inputs[wfFlightPort].Value(), 50051)
	coordPort := parseInt(m.inputs[wfCoordPort].Value(), 8080)
	nodePort := parseInt(m.inputs[wfNodePort].Value(), 50051)
	promPort := parseInt(m.inputs[wfPrometheusPort].Value(), 9090)

	catalogType := "sqlite"

	catalogPath := m.inputs[wfCatalogPath].Value()
	if catalogPath == "" {
		catalogPath = "/data/homer/homer_catalog.sqlite"
	}
	dataPath := m.inputs[wfDataPath].Value()
	if dataPath == "" {
		dataPath = "/data/homer/parquet"
	}

	jwtSecret := m.inputs[wfJWTSecret].Value()
	if jwtSecret == "" {
		jwtSecret = randomHex(32)
	}

	adminUser := m.inputs[wfAdminUser].Value()
	if adminUser == "" {
		adminUser = "admin"
	}
	adminPass := m.inputs[wfAdminPass].Value()
	if adminPass == "" {
		adminPass = "sipcapture"
	}
	adminHash := config.DefaultInternalAuthPasswordHash
	if adminPass != "sipcapture" {
		adminHash = passwordhash.MustHash(adminPass)
	}

	settingsDB := m.inputs[wfSettingsDB].Value()
	if settingsDB == "" {
		settingsDB = "/var/lib/homer/homer_settings.duckdb"
	}

	nodeHost := m.inputs[wfNodeHost].Value()
	if nodeHost == "" {
		nodeHost = "127.0.0.1"
	}

	logLevel := m.inputs[wfLogLevel].Value()
	if logLevel == "" {
		logLevel = "info"
	}

	cfg := config.Config{
		Ingest: config.IngestConfig{
			Enable:      ingestEnable,
			WorkerCount: workerCount,
			QueueSize:   queueSize,
			UDP: config.UDPServerConfig{
				Enable: true, Host: "0.0.0.0", Port: udpPort, Multicore: true,
			},
			TCP: config.TCPServerConfig{
				Enable: parseBool(m.inputs[wfTCPEnable].Value()),
				Host:   "0.0.0.0",
				Port:   udpPort, // same port as UDP by convention
			},
			HTTP: config.HTTPServerConfig{
				Enable: true, Host: "0.0.0.0", Port: httpPort,
				ReadTimeout: 30, WriteTimeout: 30,
			},
			HEP: config.HEPConfig{
				HepV2Enable: true, HepV3Enable: true, ProtobufEnable: true,
			},
		},
		Storage: config.StorageConfig{
			Enable: storageEnable,
			DuckLake: config.DuckLakeConfig{
				CatalogType:   catalogType,
				CatalogPath:   catalogPath,
				DataPath:      dataPath,
				LakeName:      "homer_lake",
				BatchSize:     batchSize,
				FlushInterval: 30,
				Compaction: config.CompactionConfig{
					Enable:           true,
					CheckIntervalSec: 1800,
					RetentionDays:    retentionDays,
				},
			},
		},
		Node: config.NodeConfig{
			Enable: nodeEnable,
			FlightServer: config.FlightServerConfig{
				Enable:         true,
				Host:           "0.0.0.0",
				Port:           flightPort,
				AuthToken:      m.inputs[wfAuthToken].Value(),
				MaxMessageSize: 16777216,
			},
			DuckLake: config.DuckLakeConfig{
				CatalogType: catalogType,
				CatalogPath: catalogPath,
				DataPath:    dataPath,
				LakeName:    "homer_lake",
			},
		},
		Coordinator: config.CoordinatorConfig{
			Enable: coordEnable,
			HTTPServer: config.CoordinatorHTTPServerConfig{
				Enable: true, Host: "0.0.0.0", Port: coordPort,
				ReadTimeout: 30, WriteTimeout: 30,
			},
			Nodes: []config.NodeEndpoint{{
				Name:  "local",
				Host:  nodeHost,
				Port:  nodePort,
				Token: m.inputs[wfAuthToken].Value(),
			}},
			SettingsDBPath: settingsDB,
			JWT: config.JWTConfig{
				Secret:      jwtSecret,
				ExpireHours: 24,
			},
			Auth: config.AuthConfig{
				Type:              "internal",
				AdminUser:         adminUser,
				AdminPasswordHash: adminHash,
			},
		},
		Log: config.LogConfig{
			Level:  logLevel,
			Output: []string{"stdout"},
			Stdout: config.StdoutLogConfig{Colored: true},
		},
		Prometheus: config.PrometheusConfig{
			Enable: promEnable,
			Host:   "0.0.0.0",
			Port:   promPort,
			Path:   "/metrics",
		},
	}

	return cfg
}

// ---- Helpers ---------------------------------------------------------------

func saveConfig(cfg config.Config, path string) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "Config saved to %s (%d bytes)\n", path, len(data))
	return nil
}

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "yes" || s == "true" || s == "1" || s == "y"
}

func parseInt(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fallback: deterministic but better than empty
		return "change-this-to-a-secure-random-string-in-production"
	}
	return hex.EncodeToString(b)
}
