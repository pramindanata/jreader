# jreader

A small Go library that reads JSON files, caches the decoded value per path, and invalidates that cache when the file changes on disk.

- Caches the **unmarshaled** value, so repeat reads skip JSON parsing entirely.
- Watches the file's parent directory with [fsnotify](https://github.com/fsnotify/fsnotify), so atomic saves (write temp file + rename) are still detected.
- Logging is optional and limited to watcher events and watcher errors.

## Install

```sh
go get github.com/pramindanata/jreader
```

## Usage

```go
package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/pramindanata/jreader"
)

type config struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	reader, err := jreader.New(logger)

	if err != nil {
		panic(err)
	}

	defer reader.Close()

	reader.Start()

	for {
		var cfg config

		if err := reader.Read("config.json", &cfg); err != nil {
			logger.Error("read failed", "error", err)
		} else {
			logger.Info("loaded", "name", cfg.Name, "value", cfg.Value)
		}

		time.Sleep(3 * time.Second)
	}
}
```

A runnable version lives in [`demo/`](./demo). Start it from the module root with `go run ./demo`, then edit `demo/first.json` or `demo/second.json` and watch the printed output change on the next tick.

## API

### `func New(logger Logger) (*JReader, error)`

Creates a `JReader` together with its fsnotify watcher. `logger` may be `nil`.

### `func (r *JReader) Start()`

Starts the watcher loop in a background goroutine. Non-blocking.

### `func (r *JReader) Close() error`

Stops the watcher and releases its resources.

### `func (r *JReader) Read(path string, value any) error`

Loads the JSON file at `path` and decodes it into `value`, which must be a non-nil pointer.

On a cache hit with the same pointer type, the cached value is copied into `value` and the file is not touched. On a cache miss, the file is read, decoded, cached, and the parent directory is registered with the watcher.

### `type Logger interface`

```go
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}
```

`*slog.Logger` satisfies this interface as-is.

## Behaviour notes

- **Shallow copies.** Cache hits copy the value with `reflect.Value.Set`, so reference fields (`[]T`, `map`, `*T`) are shared with the cache. Treat returned values as read-only.
- **Type changes re-read the file.** The raw JSON is not retained, so reading the same path into a different type re-reads and re-decodes the file, replacing the cache entry.
- **Directories are watched, not files.** `Read` adds the file's parent directory to the watcher, which is what makes atomic saves detectable.
- **Duplicate events.** Editors commonly emit more than one filesystem event per save, and each one logs a `file changed` line. Invalidation itself is idempotent.

## Benchmarks

`reader_test.go` includes `BenchmarkRead`, which compares a plain `os.ReadFile` + `json.Unmarshal` against a warm `JReader` cache hit, both reading the same generated 5,000 item fixture (~387 KB):

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `BenchmarkRead/PlainReadFileAndUnmarshal` | ~6.5 ms | 2.18 MB | 35,030 |
| `BenchmarkRead/JReaderCached` | ~133 ns | 48 B | 2 |

Median of 5 runs on Linux/amd64 (Intel Core Ultra 5 125H): roughly **50,000x faster** on a cache hit. Reproduce with:

```sh
go test -run '^$' -bench . -benchmem ./...
```

Two caveats:

- The cached case is a **cache hit**. A cold read, the first call, or any call after the file changes does the same read and parse as the baseline, plus one `reflect.New`.
- The gap comes from skipping the read, the parse, and almost all of the allocation: a hit is a shallow value copy, so the decoded slice is shared with the cache instead of rebuilt. The ratio grows with payload size, so a small config file will show a far smaller difference.

## Development

```sh
gofmt -l .
go vet ./...
go test ./...
```

Tests live in `read_test.go` and use [testify](https://github.com/stretchr/testify).

## License

[MIT](./LICENSE)
