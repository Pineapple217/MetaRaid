module github.com/Pineapple217/MetaRaid

go 1.23.0

require (
	github.com/knadh/koanf v1.5.0
	github.com/marcboeker/go-duckdb v1.8.3
	github.com/mitchellh/mapstructure v1.5.0
	github.com/redis/go-redis/v9 v9.7.0
	github.com/vmihailenco/msgpack/v5 v5.4.1
	github.com/zmb3/spotify/v2 v2.4.2
	golang.org/x/oauth2 v0.23.0
	gopkg.in/yaml.v3 v3.0.1
)

// temp fix until merge and release
replace github.com/zmb3/spotify/v2 => github.com/Pineapple217/spotify/v2 v2.4.4

require (
	github.com/apache/arrow-go/v18 v18.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/google/flatbuffers v24.3.25+incompatible // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/klauspost/cpuid/v2 v2.2.8 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/zeebo/xxh3 v1.0.2 // indirect
	golang.org/x/exp v0.0.0-20240909161429-701f63a606c0 // indirect
	golang.org/x/mod v0.21.0 // indirect
	golang.org/x/sync v0.8.0 // indirect
	golang.org/x/sys v0.26.0 // indirect
	golang.org/x/tools v0.26.0 // indirect
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
)
