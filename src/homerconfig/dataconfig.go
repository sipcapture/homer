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
//

package homerconfig

import (
	"net/http"
	"sync"
	"time"

	"gopkg.in/go-playground/validator.v9"
)

var MainConfig *HomerServerConfig
var SystemSettingsGlobal SystemSettingsSchema

type SystemSettingsSchema struct {
	HostURL             string `default:""`
	HostName            string `default:"hostname"`
	SrartTime           time.Time
	VersionApp          string `default:"0.0.0.0"`
	NameApp             string `default:"homer-core"`
	SubscribeHttpClient *http.Client
	StenoHttpClient     *http.Client
	RemoteHttpClient    *http.Client
}

// InternalSettings holds internal runtime parameters that are not part of configuration
type InternalSettings struct {
	SrartTime                time.Time
	FingerPrintType          uint
	DataBaseStrategy         uint
	CurrentDataBaseIndex     uint
	DataDatabaseGroupNodeMap map[string][]string
	Validate                 *validator.Validate
	EnvPrefix                string
	PluginsPath              string
	DbConnectionMutex        sync.Mutex
	CloudCaptureCategory     []string
}

type HomerServerSettings struct {
	//Http Settings (optimized for fasthttp)
	HTTP_SETTINGS struct {
		ApiPrefix          string `json:"api_prefix" mapstructure:"api_prefix" default:""`
		ReadTimeout        int    `json:"read_timeout" mapstructure:"read_timeout" default:"30"`                         // seconds
		WriteTimeout       int    `json:"write_timeout" mapstructure:"write_timeout" default:"30"`                       // seconds
		IdleTimeout        int    `json:"idle_timeout" mapstructure:"idle_timeout" default:"120"`                        // seconds
		MaxRequestBodySize int    `json:"max_request_body_size" mapstructure:"max_request_body_size" default:"67108864"` // bytes (64MB)
		Concurrency        int    `json:"concurrency" mapstructure:"concurrency" default:"10000"`                        // max concurrent connections
		DisableKeepalive   bool   `json:"disable_keepalive" mapstructure:"disable_keepalive" default:"false"`
		TCPKeepalive       bool   `json:"tcp_keepalive" mapstructure:"tcp_keepalive" default:"true"`
		TCPKeepalivePeriod int    `json:"tcp_keepalive_period" mapstructure:"tcp_keepalive_period" default:"30"` // seconds
		Endpoints          struct {
			HEP         string `json:"hep" mapstructure:"hep" default:"/api/hep"`                            // Auto-detect format endpoint
			HEPBinary   string `json:"hep_binary" mapstructure:"hep_binary" default:"/api/hep/binary"`       // Binary format endpoint
			HEPProtobuf string `json:"hep_protobuf" mapstructure:"hep_protobuf" default:"/api/hep/protobuf"` // Protobuf format endpoint
			Health      string `json:"health" mapstructure:"health" default:"/health"`                       // Health check endpoint
		} `json:"endpoints" mapstructure:"endpoints"`
		WebSocket struct {
			Enable bool   `json:"enable" mapstructure:"enable" default:"false"`
			Path   string `json:"path" mapstructure:"path" default:"/api/hep/ws"` // WebSocket endpoint path
		} `json:"websocket" mapstructure:"websocket"`
	} `json:"http_settings" mapstructure:"http_settings"`

	// HTTPS_SETTINGS is deprecated, use SERVER_SETTINGS.HTTPS_SERVER instead
	// Kept for backward compatibility
	HTTPS_SETTINGS struct {
		Enable              bool   `json:"enable" mapstructure:"enable" default:"false"`
		Host                string `json:"host" mapstructure:"host" default:"0.0.0.0"`
		Port                int    `json:"port" mapstructure:"port" default:"443"`
		Cert                string `json:"cert" mapstructure:"cert" default:""`
		CaCert              string `json:"cacert" mapstructure:"cacert" default:""`
		Key                 string `json:"key" mapstructure:"key" default:""`
		HttpRedirect        bool   `json:"http_redirect" mapstructure:"http_redirect" default:"false"`
		MinTLSVersionString string `json:"min_tls_version" mapstructure:"min_tls_version" default:"TLS1.2"`
		MaxTLSVersionString string `json:"max_tls_version" mapstructure:"max_tls_version" default:"TLS1.3"`
		MinTLSVersion       uint16 `default:"0"`
		MaxTLSVersion       uint16 `default:"0"`
		PreferServerCipher  bool   `json:"prefer_server_cipher" mapstructure:"prefer_server_cipher" default:"true"`
		MutualTLS           bool   `json:"mutual_tls" mapstructure:"mutual_tls" default:"false"`
		InsecureSkipVerify  bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" default:"false"`
		ReadTimeout         int    `json:"read_timeout" mapstructure:"read_timeout" default:"30"`
		WriteTimeout        int    `json:"write_timeout" mapstructure:"write_timeout" default:"30"`
		IdleTimeout         int    `json:"idle_timeout" mapstructure:"idle_timeout" default:"120"`
		MaxRequestBodySize  int    `json:"max_request_body_size" mapstructure:"max_request_body_size" default:"67108864"`
		Concurrency         int    `json:"concurrency" mapstructure:"concurrency" default:"10000"`
		DisableKeepalive    bool   `json:"disable_keepalive" mapstructure:"disable_keepalive" default:"false"`
		TCPKeepalive        bool   `json:"tcp_keepalive" mapstructure:"tcp_keepalive" default:"true"`
		TCPKeepalivePeriod  int    `json:"tcp_keepalive_period" mapstructure:"tcp_keepalive_period" default:"30"`
		WebSocket           struct {
			Enable bool `json:"enable" mapstructure:"enable" default:"false"`
		} `json:"websocket" mapstructure:"websocket"`
	} `json:"https_settings,omitempty" mapstructure:"https_settings,omitempty"`

	//Log settings
	LOG_SETTINGS struct {
		Enable          bool   `json:"enable" mapstructure:"enable" default:"true"`
		MaxAgeDays      uint32 `json:"max_age_days" mapstructure:"max_age_days" default:"7"`
		RotationHours   uint32 `json:"rotation_hours" mapstructure:"rotation_hours" default:"24"`
		Path            string `json:"path" mapstructure:"path" default:"/usr/local/homer-core/log"`
		Level           string `json:"level" mapstructure:"level" default:"error"`
		Name            string `json:"name" mapstructure:"name" default:"homer-core.log"`
		Stdout          bool   `json:"stdout" mapstructure:"stdout" default:"false"`
		Json            bool   `json:"json" mapstructure:"json" default:"true"`
		SysLogLevel     string `json:"syslog_level" mapstructure:"syslog_level" default:"LOG_INFO"`
		SysLog          bool   `json:"syslog" mapstructure:"syslog" default:"false"`
		SyslogUri       string `json:"syslog_uri" mapstructure:"syslog_uri" default:""`
		InternalTracing bool   `json:"internal_tracing" mapstructure:"internal_tracing" default:"false"`
	} `json:"log_settings" mapstructure:"log_settings"`

	//hep settings
	HEP_SETTINGS struct {
		HepV2Enable    bool `json:"hepv2_enable" mapstructure:"hepv2_enable" default:"true"`
		HepV3Enable    bool `json:"hepv3_enable" mapstructure:"hepv3_enable" default:"true"`
		ProtobufEnable bool `json:"protobuf_enable" mapstructure:"protobuf_enable" default:"true"`
		Deduplicate    bool `json:"deduplicate" mapstructure:"deduplicate" default:"false"`
	} `json:"hep_settings" mapstructure:"hep_settings"`

	// Prometheus metrics settings
	PROMETHEUS_SETTINGS struct {
		Enable bool   `json:"enable" mapstructure:"enable" default:"false"`
		Host   string `json:"host" mapstructure:"host" default:"0.0.0.0"`
		Port   int    `json:"port" mapstructure:"port" default:"9090"`
		Path   string `json:"path" mapstructure:"path" default:"/metrics"`
	} `json:"prometheus_settings" mapstructure:"prometheus_settings"`

	// Server settings (HTTP, HTTPS, UDP, TCP, TLS)
	SERVER_SETTINGS struct {
		WorkerCount int `json:"worker_count" mapstructure:"worker_count" default:"0"`
		QueueSize   int `json:"queue_size" mapstructure:"queue_size" default:"200000"`

		// HTTP server settings
		HTTP_SERVER struct {
			Enable bool   `json:"enable" mapstructure:"enable" default:"true"`
			Host   string `json:"host" mapstructure:"host" default:"0.0.0.0"`
			Port   int    `json:"port" mapstructure:"port" default:"80"`
		} `json:"http_server" mapstructure:"http_server"`

		// HTTPS server settings
		HTTPS_SERVER struct {
			Enable             bool   `json:"enable" mapstructure:"enable" default:"false"`
			Host               string `json:"host" mapstructure:"host" default:"0.0.0.0"`
			Port               int    `json:"port" mapstructure:"port" default:"443"`
			Cert               string `json:"cert" mapstructure:"cert" default:""`
			Key                string `json:"key" mapstructure:"key" default:""`
			CaCert             string `json:"cacert" mapstructure:"cacert" default:""`
			HttpRedirect       bool   `json:"http_redirect" mapstructure:"http_redirect" default:"false"`
			MinTLSVersion      string `json:"min_tls_version" mapstructure:"min_tls_version" default:"TLS1.2"`
			MaxTLSVersion      string `json:"max_tls_version" mapstructure:"max_tls_version" default:"TLS1.3"`
			PreferServerCipher bool   `json:"prefer_server_cipher" mapstructure:"prefer_server_cipher" default:"true"`
			MutualTLS          bool   `json:"mutual_tls" mapstructure:"mutual_tls" default:"false"`
			InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" default:"false"`
			MinTLSVersionNum   uint16 `default:"0"`
			MaxTLSVersionNum   uint16 `default:"0"`
		} `json:"https_server" mapstructure:"https_server"`

		// UDP server settings
		UDP_SERVER struct {
			Enable           bool   `json:"enable" mapstructure:"enable" default:"false"`
			Host             string `json:"host" mapstructure:"host" default:"0.0.0.0"`
			Port             int    `json:"port" mapstructure:"port" default:"9060"`
			Multicore        bool   `json:"multicore" mapstructure:"multicore" default:"true"`
			SocketRecvBuffer int    `json:"socket_recv_buffer" mapstructure:"socket_recv_buffer" default:"8388608"`
			SocketSendBuffer int    `json:"socket_send_buffer" mapstructure:"socket_send_buffer" default:"1048576"`
			ReadBufferCap    int    `json:"read_buffer_cap" mapstructure:"read_buffer_cap" default:"131072"`
		} `json:"udp_server" mapstructure:"udp_server"`

		// TCP server settings
		TCP_SERVER struct {
			Enable    bool   `json:"enable" mapstructure:"enable" default:"false"`
			Host      string `json:"host" mapstructure:"host" default:"0.0.0.0"`
			Port      int    `json:"port" mapstructure:"port" default:"9060"`
			Multicore bool   `json:"multicore" mapstructure:"multicore" default:"true"`
		} `json:"tcp_server" mapstructure:"tcp_server"`

		// TLS server settings
		TLS_SERVER struct {
			Enable             bool   `json:"enable" mapstructure:"enable" default:"false"`
			Host               string `json:"host" mapstructure:"host" default:"0.0.0.0"`
			Port               int    `json:"port" mapstructure:"port" default:"9061"`
			Cert               string `json:"cert" mapstructure:"cert" default:""`
			Key                string `json:"key" mapstructure:"key" default:""`
			CaCert             string `json:"cacert" mapstructure:"cacert" default:""`
			MinTLSVersion      string `json:"min_tls_version" mapstructure:"min_tls_version" default:"TLS1.2"`
			MaxTLSVersion      string `json:"max_tls_version" mapstructure:"max_tls_version" default:"TLS1.3"`
			MutualTLS          bool   `json:"mutual_tls" mapstructure:"mutual_tls" default:"false"`
			InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" default:"false"`
		} `json:"tls_server" mapstructure:"tls_server"`

		// Arrow Flight server settings (for DuckDB Airport Extension)
		FLIGHT_SERVER struct {
			Enable         bool   `json:"enable" mapstructure:"enable" default:"false"`
			Host           string `json:"host" mapstructure:"host" default:"0.0.0.0"`
			Port           int    `json:"port" mapstructure:"port" default:"50051"`
			AuthToken      string `json:"auth_token" mapstructure:"auth_token" default:""`
			BufferSize     int    `json:"buffer_size" mapstructure:"buffer_size" default:"100000"`
			MaxMessageSize int    `json:"max_message_size" mapstructure:"max_message_size" default:"16777216"`
		} `json:"flight_server" mapstructure:"flight_server"`
	} `json:"server_settings" mapstructure:"server_settings"`

	// hep settings
	SIP_SETTINGS struct {
		CensorMethod   []string `json:"censored_methods" mapstructure:"censored_methods" default:"[]"`
		DiscardMethods []string `json:"discard_methods" mapstructure:"discard_methods" default:"[]"`
		AlegIDs        []string `json:"aleg_ids" mapstructure:"aleg_ids" default:"[]"`
		CustomHeaders  []string `json:"custom_headers" mapstructure:"custom_headers" default:"[]"`
		//ForceALegID
		ForceALegID bool `json:"force_aleg_id" mapstructure:"force_aleg_id" default:"false"`
	} `json:"sip_settings" mapstructure:"sip_settings"`

	SYSTEM_SETTINGS struct {
		HostName     string `json:"hostname" mapstructure:"hostname" default:"hostname"`
		Url          string `json:"url" mapstructure:"url" default:"http://127.0.0.1"`
		UUID         string `json:"uuid" mapstructure:"uuid" default:""`
		CPUMaxProcs  int    `json:"cpu_max_procs" mapstructure:"cpu_max_procs" default:"0"`
		ConfigExport struct {
			Path   string `json:"path" mapstructure:"path" default:"/opt/homer-core/snapshot/"`
			Enable bool   `json:"enable" mapstructure:"enable" default:"false"`
		} `json:"config_export" mapstructure:"config_export"`
		LogDownload struct {
			Enable bool `json:"enable" mapstructure:"enable" default:"false"`
		} `json:"log_download" mapstructure:"log_download"`
		SqlExport struct {
			Enable bool `json:"enable" mapstructure:"enable" default:"false"`
		} `json:"sql_export" mapstructure:"sql_export"`
		Script struct {
			Engine string `json:"engine" mapstructure:"engine" default:"lua"`
			Folder string `json:"folder" mapstructure:"folder" default:"/usr/local/homer-core/scripts"`
			Enable bool   `json:"enable" mapstructure:"enable" default:"false"`
		} `json:"script" mapstructure:"script"`
		ResetMergeMappings struct {
			Enable bool `json:"enable" mapstructure:"enable" default:"false"`
		} `json:"reset_merge_mappings" mapstructure:"reset_merge_mappings"`
		JobQueueSize         uint32   `json:"job_queue_size" mapstructure:"job_queue_size" default:"5000"`
		IntervalDataCheck    int      `json:"interval_data_check" mapstructure:"interval_data_check" default:"60"`
		IntervalCleanUpCheck int      `json:"interval_cleanup_check" mapstructure:"interval_cleanup_check" default:"3600"`
		IntervalConfigCheck  int      `json:"interval_config_check" mapstructure:"interval_config_check" default:"60"`
		ConfigTables         []string `json:"config_tables" mapstructure:"config_tables" default:"[global_settings,lookup_ip,scripts_data,user_preferences,users]"`
		ReplaceFromJsonFile  bool     `json:"replace_from_json_file" mapstructure:"replace_from_json_file" default:"false"`
	} `json:"system_settings" mapstructure:"system_settings"`

	// DuckLake storage settings
	// DuckLake provides lakehouse features: time travel, snapshots, ACID transactions
	DUCKLAKE_SETTINGS struct {
		Enable        bool   `json:"enable" mapstructure:"enable" default:"false"`
		CatalogType   string `json:"catalog_type" mapstructure:"catalog_type" default:"sqlite"` // DuckLake catalog — sqlite
		CatalogPath   string `json:"catalog_path" mapstructure:"catalog_path" default:"homer_catalog.sqlite"`
		DataPath      string `json:"data_path" mapstructure:"data_path" default:"/var/lib/homer/parquet"`
		LakeName      string `json:"lake_name" mapstructure:"lake_name" default:"homer_lake"`
		TableName     string `json:"table_name" mapstructure:"table_name" default:"hep_messages"`
		BatchSize     int    `json:"batch_size" mapstructure:"batch_size" default:"10000"`
		FlushInterval int    `json:"flush_interval_sec" mapstructure:"flush_interval_sec" default:"30"`
		SearchBuffer  bool   `json:"search_buffer" mapstructure:"search_buffer" default:"false"`
		ShardCount    int    `json:"shard_count" mapstructure:"shard_count" default:"1"`
		FlushQueue    *bool  `json:"flush_queue" mapstructure:"flush_queue"` // nil = auto (true for SQLite)
		S3            struct {
			Region          string `json:"region" mapstructure:"region" default:""`
			AccessKeyID     string `json:"access_key_id" mapstructure:"access_key_id" default:""`
			SecretAccessKey string `json:"secret_access_key" mapstructure:"secret_access_key" default:""`
			Endpoint        string `json:"endpoint" mapstructure:"endpoint" default:""`
			UseSSL          bool   `json:"use_ssl" mapstructure:"use_ssl" default:"true"`
		} `json:"s3" mapstructure:"s3"`
	} `json:"ducklake_settings" mapstructure:"ducklake_settings"`
}
