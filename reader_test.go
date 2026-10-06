package jreader_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pramindanata/jreader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead_ValidJSON(t *testing.T) {
	path := writeTempJSON(t, "valid.json", `{"name":"Alice","age":30,"roles":["admin","user"]}`)

	reader := newReader(t)

	var got testData
	require.NoError(t, reader.Read(path, &got))

	want := testData{Name: "Alice", Age: 30, Roles: []string{"admin", "user"}}
	assert.Equal(t, want, got)
}

func TestRead_IntoMap(t *testing.T) {
	path := writeTempJSON(t, "map.json", `{"key":"value","count":42}`)

	reader := newReader(t)

	var got map[string]any
	require.NoError(t, reader.Read(path, &got))

	assert.Equal(t, "value", got["key"])
	assert.Equal(t, float64(42), got["count"])
}

func TestRead_InvalidJSON(t *testing.T) {
	path := writeTempJSON(t, "invalid.json", `{"name":`)

	reader := newReader(t)

	var got testData
	err := reader.Read(path, &got)

	var syntaxErr *json.SyntaxError
	require.Error(t, err)
	assert.ErrorAs(t, err, &syntaxErr)
}

func TestRead_FileNotFound(t *testing.T) {
	path := filepath.Join("temp", "does-not-exist.json")
	if err := os.Remove(path); err != nil {
		require.ErrorIs(t, err, os.ErrNotExist)
	}

	reader := newReader(t)

	var got testData
	require.ErrorIs(t, reader.Read(path, &got), os.ErrNotExist)
}

func TestRead_RejectsNonPointerTarget(t *testing.T) {
	path := writeTempJSON(t, "non-pointer.json", `{"name":"Alice","age":30}`)

	reader := newReader(t)

	require.Error(t, reader.Read(path, testData{}))
}

func TestRead_ReReadsWhenTargetTypeChanges(t *testing.T) {
	path := writeTempJSON(t, "target-type.json", `{"name":"Alice","age":30}`)

	reader := newReader(t)

	var asStruct testData
	require.NoError(t, reader.Read(path, &asStruct))
	assert.Equal(t, "Alice", asStruct.Name)

	var asMap map[string]any
	require.NoError(t, reader.Read(path, &asMap))
	assert.Equal(t, "Alice", asMap["name"])
}

func TestRead_CachesFileContents(t *testing.T) {
	path := writeTempJSON(t, "cached.json", `{"name":"Alice","age":30}`)

	reader := newReader(t)

	var before testData
	require.NoError(t, reader.Read(path, &before))
	assert.Equal(t, "Alice", before.Name)

	require.NoError(t, os.WriteFile(path, []byte(`{"name":"Bob","age":31}`), 0o644))

	var after testData
	require.NoError(t, reader.Read(path, &after))
	assert.Equal(t, "Alice", after.Name, "expected cached value when watcher is not started")
}

func TestRead_InvalidatesCacheOnChange(t *testing.T) {
	path := writeTempJSON(t, "changing.json", `{"name":"Alice","age":30}`)

	logger := &testLogger{}
	reader, err := jreader.New(logger)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, reader.Close())
	})

	reader.Start()

	var before testData
	require.NoError(t, reader.Read(path, &before))
	assert.Equal(t, "Alice", before.Name)

	require.NoError(t, os.WriteFile(path, []byte(`{"name":"Bob","age":31}`), 0o644))

	require.Eventually(t, func() bool {
		return logger.count() > 0
	}, 2*time.Second, 10*time.Millisecond, "expected a file changed log entry")

	var after testData
	require.NoError(t, reader.Read(path, &after))
	assert.Equal(t, "Bob", after.Name, "expected cache to be invalidated after file change")
}

type testData struct {
	Name  string   `json:"name"`
	Age   int      `json:"age"`
	Roles []string `json:"roles"`
}

type testLogger struct {
	mu    sync.Mutex
	infos []string
	errs  []string
}

func (l *testLogger) Info(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.infos = append(l.infos, fmt.Sprintf(msg, args...))
}

func (l *testLogger) Error(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.errs = append(l.errs, fmt.Sprintf(msg, args...))
}

func (l *testLogger) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return len(l.infos)
}

func newReader(t testing.TB) *jreader.JReader {
	t.Helper()

	reader, err := jreader.New(&testLogger{})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, reader.Close())
	})

	return reader
}

func writeTempJSON(t testing.TB, name, content string) string {
	t.Helper()

	path := filepath.Join("temp", name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	t.Cleanup(func() {
		require.NoError(t, os.Remove(path))
	})

	return path
}

func BenchmarkRead(b *testing.B) {
	path := writeTempJSON(b, "bench.json", string(createBenchJSON(b)))

	b.Run("PlainReadFileAndUnmarshal", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			data, err := os.ReadFile(path)
			if err != nil {
				b.Fatal(err)
			}

			var got benchPayload
			if err := json.Unmarshal(data, &got); err != nil {
				b.Fatal(err)
			}

			benchSink = got
		}
	})

	b.Run("JReaderCached", func(b *testing.B) {
		reader := newReader(b)

		var warm benchPayload
		require.NoError(b, reader.Read(path, &warm))

		b.ReportAllocs()

		for b.Loop() {
			var got benchPayload
			if err := reader.Read(path, &got); err != nil {
				b.Fatal(err)
			}

			benchSink = got
		}
	})
}

const benchItemCount = 5000

type benchItem struct {
	ID    int      `json:"id"`
	Name  string   `json:"name"`
	Tags  []string `json:"tags"`
	Value float64  `json:"value"`
}

type benchPayload struct {
	Items []benchItem `json:"items"`
}

var benchSink any

func createBenchJSON(tb testing.TB) []byte {
	tb.Helper()

	items := make([]benchItem, benchItemCount)

	for i := range items {
		items[i] = benchItem{
			ID:    i,
			Name:  fmt.Sprintf("item-%d", i),
			Tags:  []string{"alpha", "beta", "gamma"},
			Value: float64(i) * 1.5,
		}
	}

	data, err := json.Marshal(benchPayload{Items: items})
	require.NoError(tb, err)

	return data
}
