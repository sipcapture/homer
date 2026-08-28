package cli

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chzyer/readline"
	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

// cliSelectRowLimit caps interactive SELECT/WITH results when no LIMIT is given.
const cliSelectRowLimit = 10000

// CLIFlags holds flags for the "homer cli" subcommand.
type CLIFlags struct {
	ConfigPath string
	Query      string
	NoLimit    bool // skip auto LIMIT 10000 on SELECT/WITH without LIMIT
}

type cliFlagRefs struct {
	ConfigPath *string
	Query      *string
	NoLimit    *bool
}

// RegisterCLIFlags creates a FlagSet for "homer cli" subcommand.
func RegisterCLIFlags() (*flag.FlagSet, *cliFlagRefs) {
	fs := flag.NewFlagSet("cli", flag.ExitOnError)
	refs := &cliFlagRefs{}

	refs.ConfigPath = fs.String("config-path", "", "path to config file or directory")
	refs.Query = fs.String("query", "", "execute single SQL query and exit")
	refs.NoLimit = fs.Bool("no-limit", false, "do not auto-append LIMIT 10000 to SELECT/WITH without LIMIT")

	return fs, refs
}

// ParseCLIFlags extracts CLIFlags from parsed flag refs.
func ParseCLIFlags(refs *cliFlagRefs) CLIFlags {
	return CLIFlags{
		ConfigPath: *refs.ConfigPath,
		Query:      *refs.Query,
		NoLimit:    *refs.NoLimit,
	}
}

// RunCLICmd runs the interactive DuckLake SQL CLI.
func RunCLICmd(f CLIFlags) error {
	cfg, err := config.Load(f.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	duckCfg := duckLakeConfigFromModular(cfg)
	db, err := openDuckLakeReadOnly(duckCfg)
	if err != nil {
		return fmt.Errorf("failed to open DuckLake: %w", err)
	}
	defer db.Close()

	return runCLI(db, duckCfg.LakeName, duckCfg.DataPath, f.Query, f.NoLimit)
}

// ---- config helper ---------------------------------------------------------

func duckLakeConfigFromModular(cfg *config.Config) ducklake.Config {
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
		base.S3URLStyle = source.S3.URLStyle
	}

	if az := source.Azure; az.AccountName != "" || az.AccountKey != "" || az.ConnectionString != "" {
		base.AzureAccountName = az.AccountName
		base.AzureAccountKey = az.AccountKey
		base.AzureConnectionString = az.ConnectionString
		base.AzureEndpoint = az.Endpoint
	}

	return base
}

// ---- DuckDB read-only access -----------------------------------------------

func openDuckLakeReadOnly(cfg ducklake.Config) (*sql.DB, error) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, fmt.Errorf("failed to open DuckDB: %w", err)
	}

	if err := ducklake.ApplyDuckDBS3ClientSettings(db,
		cfg.S3Region, cfg.S3AccessKeyID, cfg.S3SecretAccessKey, cfg.S3Endpoint, cfg.S3UseSSL, cfg.S3URLStyle,
	); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to configure S3: %w", err)
	}
	if ducklake.IsS3Path(cfg.DataPath) {
		if err := ducklake.EnsureWriterS3Secret(db,
			cfg.S3Region, cfg.S3AccessKeyID, cfg.S3SecretAccessKey, cfg.S3Endpoint, cfg.S3UseSSL, cfg.S3URLStyle,
		); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to configure S3 secret: %w", err)
		}
	}
	if ducklake.IsAzurePath(cfg.DataPath) {
		ducklake.EnsureAzureCACertPath()
		if _, err := db.Exec("LOAD azure;"); err != nil {
			// Best-effort: only az:// paths actually need this extension.
			fmt.Printf("Warning: failed to load azure extension: %v\n", err)
		}
		if err := ducklake.EnsureWriterAzureSecret(db,
			cfg.AzureAccountName, cfg.AzureAccountKey, cfg.AzureConnectionString, cfg.AzureEndpoint,
		); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to configure Azure secret: %w", err)
		}
	}

	dataPath := cfg.DataPath
	fmt.Printf("Data path: %s\n", dataPath)

	viewsCreated, err := createParquetViews(db, cfg.LakeName, dataPath)
	if err != nil {
		fmt.Printf("Note: Could not auto-create views: %v\n", err)
		fmt.Printf("You can query parquet files directly:\n")
		fmt.Printf("  SELECT * FROM read_parquet('%s/*/**/*.parquet', hive_partitioning=true) LIMIT 10;\n\n", ducklake.JoinLakeDataPath(dataPath, "main"))
	} else if viewsCreated > 0 {
		fmt.Printf("Created %d table views\n", viewsCreated)
	}

	useSQL := fmt.Sprintf("USE %s", cfg.LakeName)
	if _, err := db.Exec(useSQL); err != nil {
		setSQL := fmt.Sprintf("SET search_path TO '%s'", cfg.LakeName)
		db.Exec(setSQL)
	}

	return db, nil
}

// parquetDataGlobPattern matches hive-partitioned data files only (date=…/).
// DuckLake *-delete.parquet tombstones live in the table root and must not be
// included in read_parquet(..., hive_partitioning=true).
func parquetDataGlobPattern(dataPath, table string) string {
	return ducklake.JoinLakeDataPath(dataPath, "main", table) + "/date=*/**/*.parquet"
}

func createParquetViewSQL(lakeName, table, dataPath string) string {
	pattern := parquetDataGlobPattern(dataPath, table)
	return fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s.%s AS SELECT * FROM read_parquet('%s', hive_partitioning=true, union_by_name=true)",
		lakeName, table, pattern)
}

func createParquetViews(db *sql.DB, lakeName, dataPath string) (int, error) {
	schemaSQL := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", lakeName)
	if _, err := db.Exec(schemaSQL); err != nil {
		return 0, fmt.Errorf("failed to create schema: %w", err)
	}

	mainPath := ducklake.JoinLakeDataPath(dataPath, "main")
	globSQL := fmt.Sprintf("SELECT DISTINCT regexp_extract(file, '.*[/\\\\]main[/\\\\]([^/\\\\]+)[/\\\\].*', 1) as tbl FROM glob('%s/**/*.parquet') WHERE file LIKE '%%/main/%%'", mainPath)
	rows, err := db.Query(globSQL)
	if err != nil {
		return createCommonViews(db, lakeName, dataPath)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tbl sql.NullString
		if err := rows.Scan(&tbl); err != nil {
			continue
		}
		if tbl.Valid && tbl.String != "" && tbl.String != "main" {
			tables = append(tables, tbl.String)
		}
	}

	if len(tables) == 0 {
		return createCommonViews(db, lakeName, dataPath)
	}

	created := 0
	for _, tbl := range tables {
		viewSQL := createParquetViewSQL(lakeName, tbl, dataPath)
		if _, err := db.Exec(viewSQL); err != nil {
			fmt.Printf("Warning: could not create view for %s: %v\n", tbl, err)
		} else {
			created++
		}
	}

	return created, nil
}

func createCommonViews(db *sql.DB, lakeName, dataPath string) (int, error) {
	commonTables := []string{
		"hep_proto_1_call",
		"hep_proto_1_registration",
		"hep_proto_1_default",
		"hep_proto_5_default",
		"hep_proto_34_default",
		"hep_proto_35_default",
		"hep_proto_100_default",
	}

	created := 0
	for _, tbl := range commonTables {
		pattern := parquetDataGlobPattern(dataPath, tbl)
		checkSQL := fmt.Sprintf("SELECT COUNT(*) FROM glob('%s')", pattern)
		var count int
		if err := db.QueryRow(checkSQL).Scan(&count); err != nil || count == 0 {
			continue
		}

		viewSQL := createParquetViewSQL(lakeName, tbl, dataPath)
		if _, err := db.Exec(viewSQL); err == nil {
			created++
		}
	}

	if created == 0 {
		return 0, fmt.Errorf("no parquet files found in %s", dataPath)
	}
	return created, nil
}

// ---- interactive SQL CLI ---------------------------------------------------

func runCLI(db *sql.DB, lakeName, dataPath, singleQuery string, noLimit bool) error {
	fmt.Println("Homer DuckLake SQL CLI (read-only)")
	fmt.Println("===================================")
	fmt.Printf("Schema: %s\n", lakeName)
	if !noLimit {
		fmt.Printf("SELECT/WITH without LIMIT are capped at %d rows (use --no-limit to disable)\n", cliSelectRowLimit)
	}
	fmt.Println("Type 'help' for commands, 'exit' or Ctrl+D to quit")
	fmt.Println("TAB completes SQL keywords, tables, columns, and parquet paths")
	fmt.Println()

	tables := discoverSchemaViews(db, lakeName)

	if singleQuery != "" {
		return executeSQLQuery(db, singleQuery, noLimit)
	}

	completer := newSQLCLICompleter(lakeName, dataPath, func() []string { return tables })

	historyFile := os.ExpandEnv("$HOME/.homer_cli_history")
	rl, err := readline.NewEx(&readline.Config{
		Prompt:            "sql> ",
		HistoryFile:       historyFile,
		HistoryLimit:      1000,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
		AutoComplete:      completer,
	})
	if err != nil {
		return fmt.Errorf("failed to init readline: %w", err)
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			fmt.Println("\nGoodbye!")
			return nil
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		lowerLine := strings.ToLower(line)
		switch lowerLine {
		case "exit", "quit", "\\q":
			fmt.Println("Goodbye!")
			return nil
		case "help", "\\h", "\\?":
			printCLIHelp(lakeName, tables, noLimit)
			continue
		case "tables", "\\dt":
			tables = discoverSchemaViews(db, lakeName)
			printTables(tables)
			fmt.Println("Table list refreshed for TAB completion.")
			continue
		case "clear", "\\c":
			fmt.Print("\033[H\033[2J")
			continue
		case "show tables", "show tables;":
			line = fmt.Sprintf("SHOW TABLES FROM %s", lakeName)
		}

		if err := executeSQLQuery(db, line, noLimit); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}
}

func executeSQLQuery(db *sql.DB, query string, noLimit bool) error {
	query, vertical := parseCLIQueryLine(query)
	if query == "" {
		return nil
	}
	if !noLimit {
		if limited := applyCLISelectLimit(&query); limited {
			fmt.Printf("Note: results capped at %d rows (add LIMIT or use --no-limit)\n\n", cliSelectRowLimit)
		}
	}
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return err
	}

	if len(cols) == 0 {
		fmt.Println("Query executed successfully")
		return nil
	}

	widths := make([]int, len(cols))
	for i, col := range cols {
		widths[i] = len(col)
		if widths[i] < 4 {
			widths[i] = 4
		}
	}

	var allRows [][]string
	values := make([]interface{}, len(cols))
	scanArgs := make([]interface{}, len(cols))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(scanArgs...); err != nil {
			return err
		}
		row := make([]string, len(cols))
		for i, val := range values {
			str := formatSQLValue(val)
			row[i] = str
			if len(str) > widths[i] && len(str) <= 50 {
				widths[i] = len(str)
			} else if len(str) > 50 {
				widths[i] = 50
			}
		}
		allRows = append(allRows, row)
	}

	if vertical {
		printSQLVertical(cols, allRows)
	} else {
		printSQLTableRow(cols, widths)
		printSQLTableSeparator(widths)
		for _, row := range allRows {
			printSQLTableRow(row, widths)
		}
	}

	fmt.Printf("\n(%d rows)\n\n", len(allRows))
	return rows.Err()
}

func printSQLVertical(cols []string, rows [][]string) {
	for i, row := range rows {
		if len(rows) > 1 {
			fmt.Printf("*************************** %d. row ***************************\n", i+1)
		}
		for j, col := range cols {
			val := ""
			if j < len(row) {
				val = row[j]
			}
			fmt.Printf("%-16s: %s\n", col, val)
		}
		if i < len(rows)-1 {
			fmt.Println()
		}
	}
}

// applyCLISelectLimit appends LIMIT cliSelectRowLimit to SELECT/WITH without LIMIT.
// Returns true when a limit was injected.
func applyCLISelectLimit(query *string) bool {
	if query == nil || *query == "" {
		return false
	}
	before := *query
	after := sqlvalidator.EnsureLimit(before, cliSelectRowLimit)
	*query = after
	return after != before
}

func formatSQLValue(val interface{}) string {
	if val == nil {
		return "NULL"
	}
	switch v := val.(type) {
	case []byte:
		s := string(v)
		if len(s) > 100 {
			return s[:97] + "..."
		}
		return s
	case string:
		if len(v) > 100 {
			return v[:97] + "..."
		}
		return v
	case time.Time:
		return v.Format("2006-01-02 15:04:05")
	case int64:
		if v > 1000000000000000000 {
			t := time.Unix(0, v)
			return t.Format("2006-01-02 15:04:05.000")
		}
		return fmt.Sprintf("%d", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func printSQLTableRow(values []string, widths []int) {
	fmt.Print("| ")
	for i, val := range values {
		if len(val) > widths[i] {
			val = val[:widths[i]-3] + "..."
		}
		fmt.Printf("%-*s | ", widths[i], val)
	}
	fmt.Println()
}

func printSQLTableSeparator(widths []int) {
	fmt.Print("+")
	for _, w := range widths {
		fmt.Print(strings.Repeat("-", w+2) + "+")
	}
	fmt.Println()
}

func printCLIHelp(lakeName string, tables []string, noLimit bool) {
	fmt.Println("\nCommands:")
	fmt.Println("  help, \\h, \\?    Show this help")
	fmt.Println("  tables, \\dt     List available tables")
	fmt.Println("  clear, \\c       Clear screen")
	fmt.Println("  exit, quit, \\q  Exit CLI")
	fmt.Println()
	if !noLimit {
		fmt.Printf("  SELECT/WITH without LIMIT are capped at %d rows (restart with --no-limit to disable)\n", cliSelectRowLimit)
	}
	fmt.Println("  TAB: SQL keywords, tables/columns, parquet paths under data_path")
	fmt.Println("  End query with \\G for vertical output (mysql-style)")
	fmt.Println()
	fmt.Println("Example queries:")
	fmt.Printf("  SELECT * FROM %s.main.hep_proto_1_call LIMIT 10;\n", lakeName)
	fmt.Printf("  SELECT COUNT(*) FROM %s.main.hep_proto_1_call;\n", lakeName)
	fmt.Printf("  SELECT caller, callee, method FROM %s.main.hep_proto_1_call WHERE date = current_date LIMIT 10;\n", lakeName)
	fmt.Println()
	fmt.Println("Useful DuckLake functions:")
	fmt.Printf("  SELECT * FROM ducklake_snapshots('%s.main.hep_proto_1_call');\n", lakeName)
	fmt.Printf("  CALL ducklake_merge_adjacent_files('%s');\n", lakeName)
	fmt.Println()
}

func printTables(tables []string) {
	if len(tables) == 0 {
		fmt.Println("No tables found")
		return
	}
	fmt.Println("\nAvailable tables:")
	for _, t := range tables {
		fmt.Printf("  %s\n", t)
	}
	fmt.Println()
}

func discoverSchemaViews(db *sql.DB, schemaName string) []string {
	var tables []string
	query := fmt.Sprintf("SELECT table_name FROM information_schema.tables WHERE table_schema = '%s'", schemaName)
	rows, err := db.Query(query)
	if err != nil {
		return tables
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			tables = append(tables, fmt.Sprintf("%s.%s", schemaName, name))
		}
	}
	return tables
}
