module github.com/sipcapture/homer-core/src

go 1.27.0

require (
	github.com/Jeffail/gabs/v2 v2.7.0
	github.com/VictoriaMetrics/fastcache v1.13.2
	github.com/apache/arrow-go/v18 v18.5.1
	github.com/aws/aws-sdk-go-v2 v1.43.7
	github.com/aws/aws-sdk-go-v2/config v1.32.38
	github.com/aws/aws-sdk-go-v2/credentials v1.19.37
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.22.44
	github.com/aws/aws-sdk-go-v2/service/s3 v1.107.3
	github.com/buger/jsonparser v1.1.2
	github.com/cespare/xxhash/v2 v2.3.0
	github.com/charmbracelet/bubbles v0.21.1
	github.com/charmbracelet/bubbletea v1.3.10
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/chzyer/readline v1.5.1
	github.com/duckdb/duckdb-go/v2 v2.10505.0
	github.com/emiago/sipgo v1.4.0
	github.com/fasthttp/websocket v1.5.12
	github.com/gaissmai/cidrtree v0.5.0
	github.com/gnet-io/tls v0.0.0-20240508094059-f3c43f05f76d
	github.com/go-ldap/ldap/v3 v3.4.11
	github.com/go-viper/mapstructure/v2 v2.5.0
	github.com/gogo/protobuf v1.3.2
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/gopacket v1.1.19
	github.com/google/uuid v1.6.0
	github.com/guptarohit/asciigraph v0.7.3
	github.com/hugr-lab/airport-go v0.2.1
	github.com/ijt/go-anytime/v2 v2.1.1
	github.com/jackc/pgx/v5 v5.9.2
	github.com/labstack/echo/v4 v4.15.4
	github.com/lestrrat-go/file-rotatelogs v2.4.0+incompatible
	github.com/lmittmann/tint v1.1.3
	github.com/mark3labs/mcp-go v0.44.0
	github.com/mcuadros/go-defaults v1.2.0
	github.com/notnil/chess v1.10.0
	github.com/panjf2000/gnet/v2 v2.9.5
	github.com/prometheus/client_golang v1.23.2
	github.com/sipcapture/golua v0.0.0-20200610090950-538d24098d76
	github.com/spf13/viper v1.21.0
	github.com/valyala/fasthttp v1.68.0
	go.opentelemetry.io/proto/otlp v1.10.0
	golang.org/x/crypto v0.54.0
	golang.org/x/oauth2 v0.36.0
	golang.org/x/term v0.45.0
	google.golang.org/grpc v1.82.1
	google.golang.org/protobuf v1.36.11
	gopkg.in/go-playground/validator.v9 v9.31.0
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.52.0
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.23.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.14.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.12.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.7.0 // indirect
	github.com/Azure/go-ntlmssp v0.1.1 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.7.2 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/apache/thrift v0.23.0 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.18 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.38 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.38 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.38 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.39 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.38 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.39 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.7 // indirect
	github.com/aws/smithy-go v1.27.8 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/charmbracelet/colorprofile v0.4.1 // indirect
	github.com/charmbracelet/x/ansi v0.11.5 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.15 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.9.0 // indirect
	github.com/clipperhouse/stringish v0.1.1 // indirect
	github.com/clipperhouse/uax29/v2 v2.5.0 // indirect
	github.com/duckdb/duckdb-go-bindings v0.10505.0 // indirect
	github.com/duckdb/duckdb-go-bindings/lib/darwin-amd64 v0.10505.0 // indirect
	github.com/duckdb/duckdb-go-bindings/lib/darwin-arm64 v0.10505.0 // indirect
	github.com/duckdb/duckdb-go-bindings/lib/linux-amd64 v0.10505.0 // indirect
	github.com/duckdb/duckdb-go-bindings/lib/linux-arm64 v0.10505.0 // indirect
	github.com/duckdb/duckdb-go-bindings/lib/windows-amd64 v0.10505.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/gaissmai/extnetip v0.4.0 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.8-0.20250403174932-29230038a667 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.3.2 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.28.0 // indirect
	github.com/icholy/digest v1.1.0 // indirect
	github.com/invopop/jsonschema v0.13.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/klauspost/asmfmt v1.3.2 // indirect
	github.com/klauspost/compress v1.18.4 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/labstack/gommon v0.5.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/lestrrat-go/strftime v1.1.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/minio/asm2plan9s v0.0.0-20200509001527-cdd76441f9d8 // indirect
	github.com/minio/c2goasm v0.0.0-20190812172519-36a3d3bbc4f3 // indirect
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/panjf2000/ants/v2 v2.11.3 // indirect
	github.com/paulmach/orb v0.12.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.25 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/savsgio/gotils v0.0.0-20240704082632-aef3928b8a38 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/exp v0.0.0-20260209203927-2842357ff358 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/telemetry v0.0.0-20260625142307-59b4966ccb57 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260414002931-afd174a4e478 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260414002931-afd174a4e478 // indirect
	gopkg.in/go-playground/assert.v1 v1.2.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	modernc.org/libc v1.72.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
