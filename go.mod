module github.com/emcfarlane/larking

go 1.18

require (
	github.com/bazelbuild/buildtools v0.0.0-20220215100907-23e2a9e4721a
	github.com/emcfarlane/starlarkassert v0.0.0-20220406142958-771296b4bdb6
	github.com/emcfarlane/starlarkproto v0.0.0-20210611214320-8feef53c0c82
	github.com/go-logr/logr v1.2.0
	github.com/go-logr/stdr v1.2.0
	github.com/go-openapi/spec v0.20.4
	github.com/google/go-cmp v0.5.6
	github.com/iancoleman/strcase v0.2.0
	github.com/peterh/liner v1.2.1
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8
	github.com/soheilhy/cmux v0.1.5
	go.starlark.net v0.0.0-20220328144851-d1966c6b9fcd
	gocloud.dev v0.24.0
	golang.org/x/net v0.0.0-20211123203042-d83791d6bcd9
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	google.golang.org/genproto v0.0.0-20211118181313-81c1377c94b1
	google.golang.org/grpc v1.42.0
	google.golang.org/protobuf v1.27.1
	modernc.org/sqlite v1.13.3
	nhooyr.io/websocket v1.8.7
)

require (
	cloud.google.com/go v0.94.0 // indirect
	cloud.google.com/go/secretmanager v0.1.0 // indirect
	cloud.google.com/go/storage v1.16.1 // indirect
	contrib.go.opencensus.io/integrations/ocsql v0.1.7 // indirect
	github.com/GoogleCloudPlatform/cloudsql-proxy v1.24.0 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/aws/aws-sdk-go v1.40.34 // indirect
	github.com/aws/aws-sdk-go-v2 v1.9.0 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.7.0 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.4.0 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.5.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.2.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.3.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.6.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssm v1.10.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.4.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.7.0 // indirect
	github.com/aws/smithy-go v1.8.0 // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.6 // indirect
	github.com/go-openapi/swag v0.19.15 // indirect
	github.com/go-sql-driver/mysql v1.6.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/google/wire v0.5.0 // indirect
	github.com/googleapis/gax-go/v2 v2.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/klauspost/compress v1.13.5 // indirect
	github.com/lib/pq v1.10.2 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20200410134404-eec4a21b6bb0 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	go.opencensus.io v0.23.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	go.uber.org/zap v1.19.0 // indirect
	golang.org/x/mod v0.5.1 // indirect
	golang.org/x/oauth2 v0.0.0-20210819190943-2bc19b11175f // indirect
	golang.org/x/sys v0.0.0-20220405210540-1e041c57c461 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac // indirect
	golang.org/x/tools v0.1.7 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/api v0.56.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	lukechampine.com/uint128 v1.1.1 // indirect
	modernc.org/cc/v3 v3.35.15 // indirect
	modernc.org/ccgo/v3 v3.12.49 // indirect
	modernc.org/libc v1.11.47 // indirect
	modernc.org/mathutil v1.4.1 // indirect
	modernc.org/memory v1.0.5 // indirect
	modernc.org/opt v0.1.1 // indirect
	modernc.org/strutil v1.1.1 // indirect
	modernc.org/token v1.0.0 // indirect
)

replace github.com/bazelbuild/buildtools => github.com/emcfarlane/buildtools v0.0.0-20220216022904-2d8ccb57d4be

replace go.starlark.net/starlarkstruct => ./starlarkstruct

//replace github.com/bazelbuild/buildtools => ../../bazelbuild/buildtools
