module github.com/lyonbrown4d/orch

// The `go` line is the minimum language version for this module; `go mod tidy` raises it to match
// transitive requirements (currently 1.26.x). Toolchain selection must be >= this `go` line — you
// cannot pin toolchain below `go` to “fix” a dependency; resolve outdated deps via version bumps or
// `replace`. Use GOTOOLCHAIN=local only to forbid auto-download of another toolchain (offline CI).
go 1.26.2

require (
	github.com/adrg/xdg v0.5.3
	github.com/arcgolabs/authx v0.3.2
	github.com/arcgolabs/authx/http/fiber v0.4.0
	github.com/arcgolabs/authx/jwt v0.3.0
	github.com/arcgolabs/clientx v0.1.3
	github.com/arcgolabs/collectionx/graph v0.9.0
	github.com/arcgolabs/collectionx/list v0.9.0
	github.com/arcgolabs/collectionx/mapping v0.9.0
	github.com/arcgolabs/collectionx/set v0.9.0
	github.com/arcgolabs/configx v0.6.1
	github.com/arcgolabs/configx/format/hcl v0.6.1
	github.com/arcgolabs/dix v0.11.1
	github.com/arcgolabs/dnsx/dnsserver v0.1.3
	github.com/arcgolabs/httpx v0.1.8
	github.com/arcgolabs/httpx/adapter/fiber v0.1.8
	github.com/arcgolabs/logx v0.1.3
	github.com/arcgolabs/mapper v0.2.1
	github.com/arcgolabs/observabilityx v0.4.0
	github.com/arcgolabs/plano v0.8.0
	github.com/arcgolabs/plano/lsp v0.8.0
	github.com/arcgolabs/vale v0.1.6
	github.com/cenkalti/backoff/v5 v5.0.3
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/compose-spec/compose-go/v2 v2.11.0
	github.com/containerd/errdefs v1.0.0
	github.com/coreos/go-systemd/v22 v22.7.0
	github.com/danielgtaylor/huma/v2 v2.38.0
	github.com/docker/docker v28.5.2+incompatible
	github.com/docker/go-connections v0.7.0
	github.com/firecracker-microvm/firecracker-go-sdk v1.0.0
	github.com/go-co-op/gocron/v2 v2.21.2
	github.com/gofiber/contrib/v3/prometheus v0.0.0-20260407033854-7fe5a88dc834
	github.com/gofiber/fiber/v3 v3.3.0
	github.com/hashicorp/memberlist v0.5.4
	github.com/lni/dragonboat/v4 v4.0.0-20250723143628-076c7f6497dc
	github.com/miekg/dns v1.1.72
	github.com/prometheus/client_golang v1.23.2
	github.com/pterm/pterm v0.12.83
	github.com/samber/lo v1.53.0
	github.com/samber/mo v1.17.0
	github.com/samber/oops v1.22.0
	github.com/shirou/gopsutil/v4 v4.26.5
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
	go.opentelemetry.io/otel/sdk/metric v1.44.0
	golang.org/x/crypto v0.53.0
	golang.org/x/sys v0.46.0
	golang.zx2c4.com/wireguard v0.0.0-20260522210424-ecfc5a8d5446
	google.golang.org/grpc v1.81.1
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/cri-api v0.36.2
)

require (
	github.com/arcgolabs/collectionx/bitset v0.9.0 // indirect
	github.com/vulcand/oxy/v2 v2.1.1 // indirect
)

require (
	atomicgo.dev/cursor v0.2.0 // indirect
	atomicgo.dev/keyboard v0.2.10 // indirect
	atomicgo.dev/schedule v0.1.0 // indirect
	github.com/DataDog/zstd v1.5.7 // indirect
	github.com/DmitriyVTitov/size v1.5.0 // indirect
	github.com/HdrHistogram/hdrhistogram-go v1.2.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/VictoriaMetrics/metrics v1.18.1 // indirect
	github.com/andybalholm/brotli v1.2.1 // indirect
	github.com/arcgolabs/collectionx/interval v0.9.0 // indirect
	github.com/arcgolabs/collectionx/prefix v0.9.0 // indirect
	github.com/arcgolabs/dnsx/dnsclient v0.1.3 // indirect
	github.com/arcgolabs/httpx/adapter/std v0.1.8 // indirect
	github.com/arcgolabs/pkg/option v0.0.3 // indirect
	github.com/arcgolabs/storx v0.3.0 // indirect
	github.com/arcgolabs/storx/bboltx v0.5.0 // indirect
	github.com/arcgolabs/storx/codec v0.1.0 // indirect
	github.com/arcgolabs/storx/keycodec v0.2.0 // indirect
	github.com/arcgolabs/storx/observer v0.1.0 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmatcuk/doublestar/v4 v4.10.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/x/ansi v0.11.7 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.15 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cockroachdb/errors v1.9.1 // indirect
	github.com/cockroachdb/logtags v0.0.0-20241215232642-bb51bb14a506 // indirect
	github.com/cockroachdb/pebble v0.0.0-20221207173255-0f086d933dac // indirect
	github.com/cockroachdb/redact v1.1.8 // indirect
	github.com/containerd/console v1.0.5 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/fifo v1.0.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containernetworking/cni v1.0.1 // indirect
	github.com/containernetworking/plugins v1.0.1 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/ebitengine/purego v0.10.1 // indirect
	github.com/expr-lang/expr v1.17.8 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/getsentry/sentry-go v0.12.0 // indirect
	github.com/go-chi/chi/v5 v5.3.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/analysis v0.22.3 // indirect
	github.com/go-openapi/errors v0.21.1 // indirect
	github.com/go-openapi/jsonpointer v0.20.3 // indirect
	github.com/go-openapi/jsonreference v0.20.5 // indirect
	github.com/go-openapi/loads v0.21.6 // indirect
	github.com/go-openapi/runtime v0.24.2 // indirect
	github.com/go-openapi/spec v0.20.15 // indirect
	github.com/go-openapi/strfmt v0.22.2 // indirect
	github.com/go-openapi/swag v0.22.10 // indirect
	github.com/go-openapi/validate v0.22.6 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.3 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gofiber/fiber/v2 v2.52.13 // indirect
	github.com/gofiber/schema v1.8.0 // indirect
	github.com/gofiber/utils/v2 v2.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gookit/color v1.6.1 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-memdb v1.3.5 // indirect
	github.com/hashicorp/go-metrics v0.5.4 // indirect
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
	github.com/josharian/intern v1.0.0 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/knadh/koanf/parsers/hcl v1.0.0 // indirect
	github.com/knadh/koanf/providers/confmap v1.0.0 // indirect
	github.com/knadh/koanf/providers/env/v2 v2.0.0 // indirect
	github.com/knadh/koanf/providers/file v1.2.1 // indirect
	github.com/knadh/koanf/v2 v2.3.5 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/lithammer/fuzzysearch v1.1.8 // indirect
	github.com/lni/goutils v1.4.0 // indirect
	github.com/lni/vfs v0.2.1-0.20220616104132-8852fd867376 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20260330125221-c963978e514e // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/mattn/go-runewidth v0.0.24 // indirect
	github.com/mattn/go-shellwords v1.0.13 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/sys/atomicwriter v0.1.0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/morikuni/aec v1.1.0 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/oklog/ulid/v2 v2.1.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/panjf2000/ants/v2 v2.12.1 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.27 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.68.1 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rogpeppe/go-internal v1.15.0 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/rs/zerolog v1.35.1 // indirect
	github.com/samber/do/v2 v2.0.0 // indirect
	github.com/samber/go-singleflightx v0.3.2 // indirect
	github.com/samber/go-type-to-string v1.8.0 // indirect
	github.com/samber/hot v0.13.0 // indirect
	github.com/samber/slog-common v0.22.0 // indirect
	github.com/samber/slog-zerolog/v2 v2.9.2 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/sean-/seed v0.0.0-20170313163322-e2103e2c3529 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/sony/gobreaker v1.0.0 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/unrolled/secure v1.17.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.71.0 // indirect
	github.com/valyala/fastrand v1.1.0 // indirect
	github.com/valyala/histogram v1.2.0 // indirect
	github.com/vishvananda/netlink v1.1.1-0.20210330154013-f5de75959ad5 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.etcd.io/bbolt v1.4.3 // indirect
	go.lsp.dev/jsonrpc2 v0.10.0 // indirect
	go.lsp.dev/pkg v0.0.0-20210717090340-384b27a52fb2 // indirect
	go.lsp.dev/protocol v0.12.0 // indirect
	go.lsp.dev/uri v0.3.0 // indirect
	go.mongodb.org/mongo-driver v1.14.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.68.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.6 // indirect
	golang.org/x/exp v0.0.0-20260611194520-c48552f49976 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/term v0.44.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.46.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260622175928-b703f567277d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260622175928-b703f567277d // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gotest.tools/v3 v3.5.2 // indirect
	resty.dev/v3 v3.0.0-rc.2 // indirect
)
