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

func newReader(t *testing.T) *jreader.JReader {
	t.Helper()

	reader, err := jreader.New(&testLogger{})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, reader.Close())
	})

	return reader
}

func writeTempJSON(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join("temp", name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	t.Cleanup(func() {
		require.NoError(t, os.Remove(path))
	})

	return path
}
