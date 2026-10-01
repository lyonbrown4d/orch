module github.com/lyonbrown4d/orch

// The `go` line is the minimum language version for this module; `go mod tidy` raises it to match
// transitive requirements (currently 1.27.x). Toolchain selection must be >= this `go` line — you
// cannot pin toolchain below `go` to “fix” a dependency; resolve outdated deps via version bumps or
// `replace`. Use GOTOOLCHAIN=local only to forbid auto-download of another toolchain (offline CI).
go 1.27.1

require (
	github.com/adrg/xdg v0.5.3
	github.com/arcgolabs/authx v0.3.3
	github.com/arcgolabs/authx/http/fiber v0.4.1
	github.com/arcgolabs/authx/jwt v0.3.1
	github.com/arcgolabs/clientx v0.1.4
	github.com/arcgolabs/collectionx/graph v1.0.0
	github.com/arcgolabs/collectionx/list v1.0.0
	github.com/arcgolabs/collectionx/mapping v1.0.0
	github.com/arcgolabs/collectionx/set v1.0.0
	github.com/arcgolabs/configx v0.6.4
	github.com/arcgolabs/configx/format/hcl v0.6.2
	github.com/arcgolabs/dix v0.11.2
	github.com/arcgolabs/dnsx/dnsserver v0.1.3
	github.com/arcgolabs/httpx v0.1.9
	github.com/arcgolabs/httpx/adapter/fiber v0.1.9
	github.com/arcgolabs/logx v0.1.4
	github.com/arcgolabs/mapper v0.2.2
	github.com/arcgolabs/observabilityx v0.4.2
	github.com/arcgolabs/plano v0.9.0
	github.com/arcgolabs/plano/lsp v0.9.0
	github.com/arcgolabs/vale v0.1.7
	github.com/cenkalti/backoff/v5 v5.0.3
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/compose-spec/compose-go/v2 v2.16.1
	github.com/containerd/errdefs v1.0.0
	github.com/coreos/go-systemd/v22 v22.7.0
	github.com/danielgtaylor/huma/v2 v2.39.1
	github.com/docker/docker v28.5.2+incompatible
	github.com/docker/go-connections v0.8.1
	github.com/firecracker-microvm/firecracker-go-sdk v1.0.0
	github.com/go-co-op/gocron/v2 v2.22.0
	github.com/gofiber/contrib/v3/prometheus v0.0.5
	github.com/gofiber/fiber/v3 v3.5.0
	github.com/hashicorp/memberlist v0.7.0
	github.com/jeremyhahn/dragonboat/v4 v4.0.1
	github.com/lni/vfs v0.2.1-0.20220616104132-8852fd867376
	github.com/miekg/dns v1.1.73
	github.com/prometheus/client_golang v1.24.1
	github.com/pterm/pterm v0.12.83
	github.com/samber/lo v1.53.0
	github.com/samber/mo v1.17.0
	github.com/samber/oops v1.23.2
	github.com/shirou/gopsutil/v4 v4.26.9
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	go.opentelemetry.io/otel v1.46.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.46.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.46.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.46.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.46.0
	go.opentelemetry.io/otel/sdk v1.46.0
	go.opentelemetry.io/otel/sdk/metric v1.46.0
	golang.org/x/crypto v0.57.0
	golang.org/x/sys v0.48.0
	golang.zx2c4.com/wireguard v0.0.0-20260522210424-ecfc5a8d5446
	google.golang.org/grpc v1.84.0
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/cri-api v0.37.1
)

require (
	atomicgo.dev/cursor v0.2.0 // indirect
	atomicgo.dev/keyboard v0.2.10 // indirect
	atomicgo.dev/schedule v0.1.0 // indirect
	github.com/DataDog/zstd v1.5.7 // indirect
	github.com/DmitriyVTitov/size v1.5.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/VictoriaMetrics/metrics v1.44.1 // indirect
	github.com/arcgolabs/collectionx/bitset v1.0.0 // indirect
	github.com/arcgolabs/collectionx/interval v1.0.0 // indirect
	github.com/arcgolabs/collectionx/prefix v1.0.0 // indirect
	github.com/arcgolabs/collectionx/stream v1.0.0 // indirect
	github.com/arcgolabs/dnsx/dnsclient v0.1.3 // indirect
	github.com/arcgolabs/httpx/adapter/std v0.1.9 // indirect
	github.com/arcgolabs/pkg/option v0.0.3 // indirect
	github.com/arcgolabs/storx v0.3.0 // indirect
	github.com/arcgolabs/storx/bboltx v0.5.0 // indirect
	github.com/arcgolabs/storx/codec v0.1.0 // indirect
	github.com/arcgolabs/storx/keycodec v0.2.0 // indirect
	github.com/arcgolabs/storx/observer v0.1.0 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmatcuk/doublestar/v4 v4.10.2 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/x/ansi v0.11.8 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.15 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cockroachdb/errors v1.14.0 // indirect
	github.com/cockroachdb/fifo v0.0.0-20240606204812-0bbfbd93a7ce // indirect
	github.com/cockroachdb/logtags v0.0.0-20241215232642-bb51bb14a506 // indirect
	github.com/cockroachdb/pebble v1.1.5 // indirect
	github.com/cockroachdb/redact v1.1.8 // indirect
	github.com/cockroachdb/tokenbucket v0.0.0-20230807174530-cc333fc44b06 // indirect
	github.com/containerd/console v1.0.5 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/fifo v1.1.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containernetworking/cni v1.3.1 // indirect
	github.com/containernetworking/plugins v1.9.1 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/expr-lang/expr v1.17.8 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.15 // indirect
	github.com/getsentry/sentry-go v0.49.0 // indirect
	github.com/go-chi/chi/v5 v5.3.2 // indirect
	github.com/go-json-experiment/json v0.0.0-20260820222146-c27c302e5fc3 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/analysis v1.0.0 // indirect
	github.com/go-openapi/errors v0.22.9 // indirect
	github.com/go-openapi/jsonpointer v1.0.2 // indirect
	github.com/go-openapi/jsonreference v1.0.3 // indirect
	github.com/go-openapi/loads v0.25.3 // indirect
	github.com/go-openapi/runtime v0.33.2 // indirect
	github.com/go-openapi/runtime/server-middleware v0.33.2 // indirect
	github.com/go-openapi/spec v1.0.1 // indirect
	github.com/go-openapi/strfmt v0.27.2 // indirect
	github.com/go-openapi/swag v0.29.2 // indirect
	github.com/go-openapi/swag/cmdutils v0.29.2 // indirect
	github.com/go-openapi/swag/conv v0.29.2 // indirect
	github.com/go-openapi/swag/fileutils v0.29.2 // indirect
	github.com/go-openapi/swag/jsonutils v0.29.2 // indirect
	github.com/go-openapi/swag/loading v0.29.2 // indirect
	github.com/go-openapi/swag/mangling v0.29.2 // indirect
	github.com/go-openapi/swag/netutils v0.29.2 // indirect
	github.com/go-openapi/swag/pools v0.29.2 // indirect
	github.com/go-openapi/swag/stringutils v0.29.2 // indirect
	github.com/go-openapi/swag/typeutils v0.29.2 // indirect
	github.com/go-openapi/swag/yamlutils v0.29.2 // indirect
	github.com/go-openapi/validate v1.0.0 // indirect
	github.com/go-playground/locales v0.14.2 // indirect
	github.com/go-playground/universal-translator v0.18.2 // indirect
	github.com/go-playground/validator/v10 v10.30.5 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/gofiber/fiber/v2 v2.52.15 // indirect
	github.com/gofiber/schema v1.8.8 // indirect
	github.com/gofiber/utils/v2 v2.6.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gookit/color v1.6.1 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.31.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-memdb v1.3.5 // indirect
	github.com/hashicorp/go-metrics v0.7.0 // indirect
	github.com/hashicorp/go-msgpack/v2 v2.1.5 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-sockaddr v1.0.7 // indirect
	github.com/hashicorp/go-uuid v1.0.2 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/klauspost/compress v1.20.1 // indirect
	github.com/knadh/koanf/maps v0.1.3 // indirect
	github.com/knadh/koanf/parsers/hcl v1.0.1 // indirect
	github.com/knadh/koanf/providers/confmap v1.0.1 // indirect
	github.com/knadh/koanf/providers/env/v2 v2.0.1 // indirect
	github.com/knadh/koanf/providers/file v1.2.1 // indirect
	github.com/knadh/koanf/v2 v2.3.7 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/leodido/go-urn v1.5.0 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/lithammer/fuzzysearch v1.1.8 // indirect
	github.com/lni/goutils v1.4.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/lufia/plan9stats v0.0.0-20260802145828-341c2f0c90b5 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.30 // indirect
	github.com/mattn/go-shellwords v1.0.15 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/sys/atomicwriter v0.1.0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/molecule-man/go-brrr v1.1.1 // indirect
	github.com/morikuni/aec v1.1.0 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oklog/ulid/v2 v2.1.2 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/panjf2000/ants/v2 v2.12.1 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.31 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20260916203055-22a1a467d9f0 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	github.com/prometheus/common v0.72.0 // indirect
	github.com/prometheus/procfs v0.22.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rogpeppe/go-internal v1.16.0 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/rs/zerolog v1.35.1 // indirect
	github.com/samber/do/v2 v2.1.0 // indirect
	github.com/samber/go-singleflightx v1.0.0 // indirect
	github.com/samber/go-type-to-string v1.8.0 // indirect
	github.com/samber/hot v0.13.1 // indirect
	github.com/samber/slog-common v0.22.0 // indirect
	github.com/samber/slog-zerolog/v2 v2.9.2 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/sean-/seed v0.0.0-20170313163322-e2103e2c3529 // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/sony/gobreaker v1.0.0 // indirect
	github.com/tinylib/msgp v1.6.5 // indirect
	github.com/tklauser/go-sysconf v0.4.0 // indirect
	github.com/tklauser/numcpus v0.12.0 // indirect
	github.com/unrolled/secure v1.17.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.74.0 // indirect
	github.com/valyala/fastrand v1.1.0 // indirect
	github.com/valyala/histogram v1.2.0 // indirect
	github.com/vishvananda/netlink v1.3.1 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/vulcand/oxy/v2 v2.2.0 // indirect
	github.com/xhit/go-str2duration/v2 v2.2.0 // indirect
	github.com/xo/terminfo v1.2.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.etcd.io/bbolt v1.5.0 // indirect
	go.lsp.dev/jsonrpc2 v1.0.1 // indirect
	go.lsp.dev/protocol v1.0.1 // indirect
	go.lsp.dev/uri v1.0.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.71.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.1 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.6 // indirect
	golang.org/x/exp v0.0.0-20260908205506-85c1c2202aba // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/term v0.46.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260928230214-8a89bd6388cc // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	resty.dev/v3 v3.0.0-rc.4 // indirect
)
