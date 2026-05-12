// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	_ "github.com/sipcapture/homer-core/src/glibcstub"
	"github.com/mcuadros/go-defaults"
	"github.com/sipcapture/homer-core/src/cli"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator"
	"github.com/sipcapture/homer-core/src/homerconfig"
	"github.com/sipcapture/homer-core/src/lineprotoreceiver"
	homermcp "github.com/sipcapture/homer-core/src/mcp"
	"github.com/sipcapture/homer-core/src/node"
	"github.com/sipcapture/homer-core/src/otlpreceiver"
	otlpsink "github.com/sipcapture/homer-core/src/otlpreceiver/sink"
	input "github.com/sipcapture/homer-core/src/server"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	"github.com/sipcapture/homer-core/src/stream/hepstream"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/metrics"
	"github.com/sipcapture/homer-core/src/writer"
)

const (
	defaultShutdownTimeout = 30 * time.Second
)

// Module represents a startable/stoppable module
type Module interface {
	Start() error
	Stop() error
}

// Reloadable represents a module that can be reloaded
type Reloadable interface {
	Reload() error
}

type server interface {
	Run()
	End()
}

type metricsServerWrapper struct {
	*metrics.MetricsServer
}

func (m *metricsServerWrapper) Run() {
	m.MetricsServer.Start()
}

func (m *metricsServerWrapper) End() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m.MetricsServer.Stop(ctx)
}

// ---- Server-mode flags -----------------------------------------------------

// ServerFlags holds flags for default server mode (no subcommand).
type ServerFlags struct {
	ConfigPath         *string
	LogName            *string
	LogPathHomerServer *string
	SyslogDisable      *bool
	LogLevel           *string
	PidFile            *string
}

func initServerFlags() ServerFlags {
	var sf ServerFlags
	sf.ConfigPath = flag.String("config-path", "", "path to config file or directory")
	sf.LogName = flag.String("homer-core-log-name", "", "the name prefix of the log file")
	sf.LogPathHomerServer = flag.String("homer-core-log-path", "", "the path of the log file")
	sf.SyslogDisable = flag.Bool("syslog-disable", false, "disable syslog, use only stdout for logging")
	sf.LogLevel = flag.String("log-level", "", "set log level: debug, info, warn, error, trace")
	sf.PidFile = flag.String("pid-file", "/var/run/homer-core.pid", "path to PID file")
	return sf
}

// ---- Usage -----------------------------------------------------------------

func printUsage() {
	fmt.Fprintf(os.Stderr, `Homer Core %s

Usage:
  homer                         Run the server (default)
  homer search [flags]          Search Homer data via coordinator API
  homer cli [flags]             Interactive DuckLake SQL shell
  homer system [flags]          System operations (compaction, extensions, reload)
  homer agent [action] [flags]  Query heplify agent API (stats, health, watch)
  homer wizard [flags]          Interactive config generator wizard
  homer mcp [flags]             Start MCP stdio server
  homer migrate [action] [flags]
                                Migrate data from Homer 7 (homer-app PostgreSQL).
                                Actions: settings (default), hep
  homer version                 Show version
  homer help                    Show this help

Server flags (no subcommand):
  --config-path <path>          Path to config file or directory
  --log-level <level>           Set log level: debug, info, warn, error, trace
  --syslog-disable              Disable syslog, use only stdout
  --pid-file <path>             Path to PID file (default: /var/run/homer-core.pid)
  -v, --version                 Show version and exit

search -- Search Homer data via coordinator API:
  --host <host:port>            Coordinator address (required)
  --user <name>                 Username for authentication
  --pass <secret>               Password for authentication
  --token <value>               Session JWT or Settings→Auth Token secret (instead of user/pass); or env HOMER_TOKEN
  --from <spec>                 Time range start: 1h, 30m, 1d, ISO 8601 (default: 1h)
  --to <spec>                   Time range end (default: now)
  --proto <name|int>          Protocol: sip, rtcp, rtp, dns, log, otlp_traces, otlp_metrics, otlp_logs, lp, or decimal (default: sip)
  --event-type <type>           Event type: call, registration, default
  --method <method>             SIP method: INVITE, REGISTER, OPTIONS...
  --call-id <id>                Call/session ID filter
  --from-user <user>            From user filter
  --to-user <user>              To user filter
  --src-ip <ip>                 Source IP filter
  --dst-ip <ip>                 Destination IP filter
  --node <name>                 Node name filter
  --limit <n>                   Result limit (default: 50)
  --sql <query>                 Raw SQL query (bypasses structured search, full DuckDB SQL)
  --select <cols>               Custom SELECT columns/aggregations (e.g. "method, count(*) as cnt")
  --group-by <cols>             GROUP BY clause (e.g. "method")
  --order-by <expr>             Custom ORDER BY (e.g. "cnt DESC", default: timestamp DESC)
  --format <fmt>                Output: table, vertical, csv, json, chart, callflow (default: table)
  --fields <list>               Comma-separated fields to show (default: all)
  --grep <pattern>              Post-filter: include rows matching pattern (e.g. INVITE or event=INVITE)
  --exclude <pattern>           Post-filter: exclude rows matching pattern (e.g. "100,183")
  --interactive                 Launch interactive TUI mode
  --debug                       Print request/response to stderr

cli -- Interactive DuckLake SQL shell:
  --config-path <path>          Path to config file or directory
  --query <sql>                 Execute single SQL query and exit

system -- System operations:
  --config-path <path>          Path to config file or directory
  --compaction-force            Run full compaction (merge, expire, cleanup)
  --compaction-merge            Merge adjacent small files
  --compaction-expire-snapshots Expire DuckLake snapshots
  --compaction-expire-older-than <dur>  Age threshold (default: 1h)
  --compaction-retention-days <n>       Delete data older than N days
  --compaction-merge-list       List smallest files before/after merge
  --compaction-merge-list-limit <n>     Limit for merge list (default: 50)
  --install-extensions          Install DuckDB extensions (ducklake, sqlite)
  --reload                      Send SIGHUP to running process
  --pid-file <path>             PID file path (default: /var/run/homer-core.pid)
  --duckdb-version              Show DuckDB version
  --generate-example-config     Generate example JSON config

agent -- Query heplify agent API:
  stats                         Show packet capture statistics (default)
  health                        Show agent health status
  watch                         Continuously refresh stats (Ctrl+C to stop)
  --host <host:port>            heplify API address (default: 127.0.0.1:8008)
  --user <name>                 Username for Basic Auth
  --pass <secret>               Password for Basic Auth
  --interval <dur>              Refresh interval for watch mode (default: 3s)
  --format <fmt>                Output format: table, json (default: table)

wizard -- Interactive config generator:
  --output <path>               Output file path (default: homer.json)
  --profile <name>              Non-interactive preset: all-in-one, writer, edge, coordinator, node

mcp -- Start MCP stdio server:
  --config-path <path>          Path to config file or directory
`, VERSION_APPLICATION)
}

// ---- ModuleManager ---------------------------------------------------------

type ModuleManager struct {
	modules []Module
	servers []server
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewModuleManager() *ModuleManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &ModuleManager{
		modules: make([]Module, 0),
		servers: make([]server, 0),
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (mm *ModuleManager) AddModule(m Module) {
	mm.modules = append(mm.modules, m)
}

func (mm *ModuleManager) AddLegacyServer(s server) {
	mm.servers = append(mm.servers, s)
}

func (mm *ModuleManager) Start() error {
	logger.Info("Starting modules...")
	for _, m := range mm.modules {
		if err := m.Start(); err != nil {
			return fmt.Errorf("failed to start module: %w", err)
		}
	}
	for i, srv := range mm.servers {
		mm.wg.Add(1)
		go func(idx int, s server) {
			defer mm.wg.Done()
			logger.Info("Starting legacy server", "index", idx+1)
			s.Run()
			logger.Info("Legacy server stopped", "index", idx+1)
		}(i, srv)
	}
	logger.Info("All modules started")
	return nil
}

func (mm *ModuleManager) Stop(timeout time.Duration) error {
	logger.Info("Stopping homer-core", "timeout", timeout)
	mm.cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Stop in reverse registration order so ingest-side modules (OTLP,
		// Line Protocol, MCP) shut down before Writer closes DuckLake —
		// e.g. OTLP async queue can drain while the DB is still open.
		for i := len(mm.modules) - 1; i >= 0; i-- {
			idx := len(mm.modules) - i
			logger.Info("Stopping module", "index", idx, "total", len(mm.modules))
			if err := mm.modules[i].Stop(); err != nil {
				logger.Error(fmt.Sprintf("Error stopping module %d/%d: %v", idx, len(mm.modules), err))
			}
			logger.Info("Module stopped", "index", idx, "total", len(mm.modules))
		}
		for i, srv := range mm.servers {
			logger.Info("Stopping legacy server", "index", i+1)
			srv.End()
		}
		mm.wg.Wait()
	}()

	select {
	case <-done:
		logger.Info("All modules stopped gracefully")
		return nil
	case <-time.After(timeout):
		logger.Error("Shutdown timeout exceeded, forcing exit", "timeout", timeout)
		return fmt.Errorf("shutdown timeout exceeded")
	}
}

func (mm *ModuleManager) Reload() error {
	logger.Info("Reloading modules...")
	for _, m := range mm.modules {
		if reloadable, ok := m.(Reloadable); ok {
			if err := reloadable.Reload(); err != nil {
				logger.Error(fmt.Sprintf("Failed to reload module: %v", err))
			}
		}
	}
	logger.Info("Module reload completed")
	return nil
}

// ---- Config helpers --------------------------------------------------------

func tryLoadModularConfig(configPath string) (*config.Config, bool) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, false
	}
	return cfg, true
}

func ducklakeConfigFromModular(cfg *config.Config) ducklake.Config {
	base := ducklake.DefaultConfig()

	source := cfg.Storage.DuckLake
	if !cfg.Storage.Enable && cfg.Node.Enable {
		source = cfg.Node.DuckLake
	}

	if source.CatalogType != "" {
		base.CatalogType = ducklake.CatalogType(source.CatalogType)
	}
	if source.CatalogPath != "" {
		base.CatalogPath = source.CatalogPath
	}
	if source.DataPath != "" {
		base.DataPath = source.DataPath
	}
	if source.LakeName != "" {
		base.LakeName = source.LakeName
	}
	if source.BatchSize > 0 {
		base.BatchSize = source.BatchSize
	}
	if source.FlushInterval > 0 {
		base.FlushInterval = time.Duration(source.FlushInterval) * time.Second
	}
	if source.FlushQueue != nil {
		base.FlushQueue = source.FlushQueue
	}
	if source.DataInliningRowLimit != -1 {
		base.DataInliningRowLimit = source.DataInliningRowLimit
	}

	if source.S3.AccessKeyID != "" {
		base.S3Region = source.S3.Region
		base.S3AccessKeyID = source.S3.AccessKeyID
		base.S3SecretAccessKey = source.S3.SecretAccessKey
		base.S3Endpoint = source.S3.Endpoint
		base.S3UseSSL = source.S3.UseSSL
	}

	return base
}

// writePidFile writes the current process PID to file
func writePidFile(pidFile string) error {
	return os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644)
}

func removePidFile(pidFile string) {
	os.Remove(pidFile)
}

// ---- Main ------------------------------------------------------------------

func main() {
	// Handle common flags before subcommand dispatch
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "-version", "--version", "-v":
			PrintVersion()
			return
		case "-h", "-help", "--help", "help":
			printUsage()
			return
		}
	}

	// If no args or first arg starts with "-", run server mode
	if len(os.Args) < 2 || strings.HasPrefix(os.Args[1], "-") {
		runServer()
		return
	}

	// Subcommand dispatch
	switch os.Args[1] {
	case "search":
		fs, refs := cli.RegisterSearchFlags()
		fs.Parse(os.Args[2:])
		sf := cli.ParseSearchFlags(refs)
		if err := cli.RunSearch(sf); err != nil {
			fmt.Fprintf(os.Stderr, "Search error: %v\n", err)
			os.Exit(1)
		}

	case "cli":
		fs, refs := cli.RegisterCLIFlags()
		fs.Parse(os.Args[2:])
		cf := cli.ParseCLIFlags(refs)
		if err := cli.RunCLICmd(cf); err != nil {
			fmt.Fprintf(os.Stderr, "CLI error: %v\n", err)
			os.Exit(1)
		}

	case "system":
		fs, refs := cli.RegisterSystemFlags()
		fs.Parse(os.Args[2:])
		sf := cli.ParseSystemFlags(refs)
		if err := cli.RunSystemCmd(sf); err != nil {
			fmt.Fprintf(os.Stderr, "System error: %v\n", err)
			os.Exit(1)
		}

	case "agent":
		action := "stats"
		args := os.Args[2:]
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			action = args[0]
			args = args[1:]
		}
		fs, refs := cli.RegisterAgentFlags()
		fs.Parse(args)
		af := cli.ParseAgentFlags(refs)
		af.Action = action
		if err := cli.RunAgentCmd(af); err != nil {
			fmt.Fprintf(os.Stderr, "Agent error: %v\n", err)
			os.Exit(1)
		}

	case "wizard":
		fs, refs := cli.RegisterWizardFlags()
		fs.Parse(os.Args[2:])
		wf := cli.ParseWizardFlags(refs)
		if err := cli.RunWizardCmd(wf); err != nil {
			fmt.Fprintf(os.Stderr, "Wizard error: %v\n", err)
			os.Exit(1)
		}

	case "mcp":
		if err := runMCPSubcommand(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "MCP error: %v\n", err)
			os.Exit(1)
		}

	case "migrate":
		action := "settings"
		args := os.Args[2:]
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			action = args[0]
			args = args[1:]
		}
		fs, refs := cli.RegisterMigrateFlags()
		fs.Parse(args)
		mf := cli.ParseMigrateFlags(refs)
		mf.Action = action
		if err := cli.RunMigrateCmd(mf); err != nil {
			fmt.Fprintf(os.Stderr, "Migrate error: %v\n", err)
			os.Exit(1)
		}

	case "version":
		PrintVersion()

	case "help", "--help", "-help", "-h":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// runServer is the default mode -- starts the homer-core server.
func runServer() {
	sf := initServerFlags()
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	mm := NewModuleManager()

	configPath := ""
	if sf.ConfigPath != nil && *sf.ConfigPath != "" {
		configPath = *sf.ConfigPath
	}

	cfg, isModular := tryLoadModularConfig(configPath)

	var writerModule *writer.Writer
	var nodeModule *node.Node

	if isModular {
		useStdout := false
		for _, out := range cfg.Log.Output {
			if out == "stdout" {
				useStdout = true
				break
			}
		}
		logger.InitLoggerSimple(cfg.Log.Level, useStdout, cfg.Log.JSON)
		logger.Info("homer-core starting in modular mode...")
		logger.Info("version", "version", VERSION_APPLICATION)
		homerconfig.SystemSettingsGlobal.VersionApp = VERSION_APPLICATION
		logger.Info("duckdb version", "version", cli.GetDuckDBVersion())
		logger.Info("enabled modules", "modules", fmt.Sprintf("%v", cfg.GetEnabledModules()))

		// Live HEP stream broker (circular buffer + fan-out).
		// Created once per process and shared between writer (publisher),
		// node (subscriber endpoint) and coordinator (local fan-in).
		// Nil when ingest.hep_stream.enable is false, which turns every
		// downstream Publish/Subscribe into a no-op.
		var liveBroker *hepstream.Broker
		if cfg.Ingest.Enable && cfg.Ingest.HepStream.Enable {
			liveBroker = hepstream.NewBroker(hepstream.Config{
				Enable:         true,
				BufferSize:     cfg.Ingest.HepStream.BufferSize,
				MaxSubscribers: cfg.Ingest.HepStream.MaxSubscribers,
				PerSubQueueLen: cfg.Ingest.HepStream.PerSubQueueLen,
				RatePerSubPPS:  cfg.Ingest.HepStream.RatePerSubPPS,
			})
			logger.Info("Live HEP stream broker enabled",
				"buffer_size", cfg.Ingest.HepStream.BufferSize,
				"max_subscribers", cfg.Ingest.HepStream.MaxSubscribers)
		}

		// Start Writer module if ingest + storage are enabled
		if cfg.Ingest.Enable && cfg.Storage.Enable {
			w, err := writer.New(&cfg.Ingest, &cfg.Storage, &cfg.Prometheus, &cfg.RemoteLogging)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create writer module: %v\n", err)
				os.Exit(1)
			}
			if liveBroker != nil {
				w.SetBroker(liveBroker)
			}
			mm.AddModule(w)
			writerModule = w
			logger.Info("Writer module configured (ingest + storage)")
		}

		// Start Node module if enabled
		if cfg.Node.Enable {
			n, err := node.New(&cfg.Node)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create node module: %v\n", err)
				os.Exit(1)
			}
			// Share writer's DuckDB connection with node for real-time query visibility.
			// Without this, the node's separate DuckDB can't see data flushed by the writer.
			if writerModule != nil {
				if db := writerModule.GetDB(); db != nil {
					n.SetSharedDB(db)
				}
			}
			if liveBroker != nil {
				n.SetBroker(liveBroker)
			}
			mm.AddModule(n)
			nodeModule = n
			logger.Info("Node module configured")
		}

		// Start Coordinator module if enabled
		if cfg.Coordinator.Enable {
			// Propagate lake name from storage/node config so coordinator SQL
			// uses the correct catalog name (user may override via coordinator.lake_name).
			if cfg.Coordinator.LakeName == "" || cfg.Coordinator.LakeName == "homer_lake" {
				if cfg.Storage.Enable && cfg.Storage.DuckLake.LakeName != "" {
					cfg.Coordinator.LakeName = cfg.Storage.DuckLake.LakeName
				} else if cfg.Node.Enable && cfg.Node.DuckLake.LakeName != "" {
					cfg.Coordinator.LakeName = cfg.Node.DuckLake.LakeName
				}
			}
			cfg.Coordinator.MCP = cfg.MCP
			if HasEmbeddedUI() {
				coordinator.WebAssets = &WebAssets
			}
			c, err := coordinator.New(&cfg.Coordinator)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create coordinator module: %v\n", err)
				os.Exit(1)
			}
			// When writer+coordinator run in the same process, hand the
			// local broker to the coordinator so the stream service can
			// subscribe in-process instead of opening a WS back to
			// ourselves. In distributed deployments this stays nil and
			// StreamService fans out to remote nodes.
			if liveBroker != nil {
				c.SetLocalBroker(liveBroker)
			}
			mm.AddModule(c)
			logger.Info("Coordinator module configured")
		}

		// Start OTLP receiver if enabled. The receiver writes to
		// dedicated DuckLake tables (otlp_traces / otlp_metrics /
		// otlp_logs) on the writer's primary shard, so it is only
		// started when the writer module is also up — otherwise we
		// have no DuckDB to write to and no shared storage handle.
		if cfg.Ingest.OTLP.Enable {
			if writerModule == nil {
				logger.Warn("OTLP receiver enabled but writer module is disabled — OTLP requires storage; skipping receiver startup")
			} else {
				duckMgr := writerModule.GetDuckLakeManager()
				if duckMgr == nil {
					logger.Warn("OTLP receiver enabled but DuckLake manager is unavailable; skipping receiver startup")
				} else {
					storage := duckMgr.OTLPStorage()
					duckSink, err := otlpsink.NewDuckLake(storage, cfg.Ingest.OTLP.Sinks)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Failed to create OTLP DuckLake sink: %v\n", err)
						os.Exit(1)
					}
					var otlpSink otlpsink.Sink = duckSink
					var asyncDrain func(context.Context) error
					if cfg.Ingest.OTLP.AsyncEnable {
						depth := cfg.Ingest.OTLP.AsyncQueueDepth
						if depth < 1 {
							depth = 512
						}
						aq := otlpsink.NewAsyncQueue(duckSink, depth, cfg.Ingest.OTLP.AsyncEnqueueTimeoutMs)
						otlpSink = aq
						asyncDrain = aq.Drain
						logger.Info("OTLP async queue enabled",
							"queue_depth", depth,
							"enqueue_timeout_ms", cfg.Ingest.OTLP.AsyncEnqueueTimeoutMs)
					}
					otlpMod, err := otlpreceiver.New(&cfg.Ingest.OTLP, otlpSink, duckSink.EnsureSchema, asyncDrain)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Failed to create OTLP receiver: %v\n", err)
						os.Exit(1)
					}
					if otlpMod != nil {
						mm.AddModule(otlpMod)
						logger.Info("OTLP receiver configured (gRPC + HTTP)",
							"grpc_listen", cfg.Ingest.OTLP.GRPC.Listen,
							"http_listen", cfg.Ingest.OTLP.HTTP.Listen)
					}
				}
			}
		}

		// Start InfluxDB Line Protocol receiver if enabled. Each
		// measurement materialises into its own DuckLake table on the
		// writer's primary shard, so the receiver shares the writer's
		// DuckDB connection and is only started when the writer module
		// is up.
		if cfg.Ingest.LineProto.Enable {
			if writerModule == nil {
				logger.Warn("Line Protocol receiver enabled but writer module is disabled — LP requires storage; skipping receiver startup")
			} else {
				duckMgr := writerModule.GetDuckLakeManager()
				if duckMgr == nil {
					logger.Warn("Line Protocol receiver enabled but DuckLake manager is unavailable; skipping receiver startup")
				} else {
					lpMod, err := lineprotoreceiver.New(&cfg.Ingest.LineProto, duckMgr.GetDB(), duckMgr.GetLakeName())
					if err != nil {
						fmt.Fprintf(os.Stderr, "Failed to create Line Protocol receiver: %v\n", err)
						os.Exit(1)
					}
					if lpMod != nil {
						mm.AddModule(lpMod)
						logger.Info("Line Protocol receiver configured",
							"listen", cfg.Ingest.LineProto.Listen,
							"default_db", cfg.Ingest.LineProto.DefaultDB,
							"table_prefix", cfg.Ingest.LineProto.TablePrefix)
					}
				}
			}
		}

		// Start MCP module if enabled
		if cfg.MCP.Enable {
			mcpModule, err := homermcp.New(&cfg.MCP)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create MCP module: %v\n", err)
				os.Exit(1)
			}
			mm.AddModule(mcpModule)
			logger.Info("MCP module configured")
		}

		// Prometheus metrics if enabled
		if cfg.Prometheus.Enable {
			metricsCfg := &metrics.MetricsServerConfig{
				Enable: cfg.Prometheus.Enable,
				Host:   cfg.Prometheus.Host,
				Port:   cfg.Prometheus.Port,
				Path:   cfg.Prometheus.Path,
			}
			metricsSrv := metrics.NewMetricsServerWithConfig(metricsCfg)
			mm.AddLegacyServer(&metricsServerWrapper{MetricsServer: metricsSrv})
		}

	} else {
		// Legacy mode
		logger.Info("Loading legacy configuration...")

		var configPaths []string
		if sf.ConfigPath != nil && *sf.ConfigPath != "" {
			path := *sf.ConfigPath
			if info, err := os.Stat(path); err == nil {
				if info.IsDir() {
					configPaths = append(configPaths, path+"/homer_server_config.json")
					configPaths = append(configPaths, path+"/homer_server_custom.json")
				} else {
					configPaths = append(configPaths, path)
				}
			}
		}

		var logName, logPath string
		if sf.LogName != nil {
			logName = *sf.LogName
		}
		if sf.LogPathHomerServer != nil {
			logPath = *sf.LogPathHomerServer
		}

		homerconfig.MainConfig = homerconfig.New(configPaths, logName, logPath)
		homerconfig.MainConfig.ReadConfig()

		if sf.SyslogDisable != nil && *sf.SyslogDisable {
			homerconfig.MainConfig.Setting.LOG_SETTINGS.SysLog = false
			homerconfig.MainConfig.Setting.LOG_SETTINGS.Stdout = true
		}

		if sf.LogLevel != nil && *sf.LogLevel != "" {
			validLevels := map[string]bool{
				"debug": true, "info": true, "warn": true,
				"warning": true, "error": true, "trace": true,
			}
			if validLevels[strings.ToLower(*sf.LogLevel)] {
				homerconfig.MainConfig.Setting.LOG_SETTINGS.Level = strings.ToLower(*sf.LogLevel)
			}
		}

		systemSettings := new(homerconfig.SystemSettingsSchema)
		defaults.SetDefaults(systemSettings)
		homerconfig.SystemSettingsGlobal = *systemSettings
		homerconfig.SystemSettingsGlobal.VersionApp = VERSION_APPLICATION

		if homerconfig.MainConfig.Setting.SYSTEM_SETTINGS.CPUMaxProcs > 0 {
			runtime.GOMAXPROCS(homerconfig.MainConfig.Setting.SYSTEM_SETTINGS.CPUMaxProcs)
		}

		logger.InitLogger()
		logger.Info("homer-core starting in legacy mode...")
		logger.Info("version", "version", VERSION_APPLICATION)
		logger.Info("duckdb version", "version", cli.GetDuckDBVersion())

		// Prometheus metrics if enabled
		if homerconfig.MainConfig.Setting.PROMETHEUS_SETTINGS.Enable {
			metricsSrv := metrics.NewMetricsServer()
			mm.AddLegacyServer(&metricsServerWrapper{MetricsServer: metricsSrv})
		}

		// Legacy HEP input server.
		// Legacy mode never spins up the modular coordinator/node stack,
		// so the broker here would have no subscribers. We still create
		// it when legacy config opts in (via SYSTEM_SETTINGS.HepStream
		// if ever added) -- for now the feature is modular-only.
		hep := input.NewHEPInput()
		mm.AddLegacyServer(hep)
	}

	logger.Info("go runtime info", "go_version", runtime.Version(), "cpu_cores", runtime.NumCPU(), "GOMAXPROCS", runtime.GOMAXPROCS(0))

	pidFile := *sf.PidFile
	if err := writePidFile(pidFile); err != nil {
		logger.Warn(fmt.Sprintf("Failed to write PID file %s: %v (hot reload via -reload may not work)", pidFile, err))
	} else {
		logger.Info("PID file written", "path", pidFile)
		defer removePidFile(pidFile)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	if err := mm.Start(); err != nil {
		logger.Error(fmt.Sprintf("Failed to start: %v", err))
		os.Exit(1)
	}

	// TieredStorageManager DuckDB is created in Writer.Start; wire it for
	// merged HTTP /query search (writer homer_lake + hot/cold tier catalogs).
	if writerModule != nil && nodeModule != nil {
		if tdb := writerModule.GetTieredQueryDB(); tdb != nil {
			nodeModule.SetTieredQueryDB(tdb)
		}
	}

	for {
		sig := <-sigCh
		logger.Info("Received signal", "signal", sig)

		if sig == syscall.SIGHUP {
			logger.Info("SIGHUP received, reloading configuration and scripts...")
			if err := mm.Reload(); err != nil {
				logger.Error(fmt.Sprintf("Reload failed: %v", err))
			}
			continue
		}

		if err := mm.Stop(defaultShutdownTimeout); err != nil {
			logger.Error(fmt.Sprintf("Error during shutdown: %v", err))
			os.Exit(1)
		}
		break
	}

	logger.Info("homer-core stopped successfully")
}

func runMCPSubcommand(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	configPath := fs.String("config-path", "", "path to config file or directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	useStdout := false
	for _, out := range cfg.Log.Output {
		if out == "stdout" {
			useStdout = true
			break
		}
	}
	logger.InitLoggerSimple(cfg.Log.Level, useStdout, cfg.Log.JSON)

	m, err := homermcp.New(&cfg.MCP)
	if err != nil {
		return fmt.Errorf("create mcp module: %w", err)
	}
	return m.Run()
}
