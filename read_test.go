package fread_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pramindanata/fread"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead_ValidJSON(t *testing.T) {
	path := writeTempJSON(t, "valid.json", `{"name":"Alice","age":30,"roles":["admin","user"]}`)

	fileReader := newFileReader(t)

	var got testData
	require.NoError(t, fileReader.Read(path, &got))

	want := testData{Name: "Alice", Age: 30, Roles: []string{"admin", "user"}}
	assert.Equal(t, want, got)
}

func TestRead_IntoMap(t *testing.T) {
	path := writeTempJSON(t, "map.json", `{"key":"value","count":42}`)

	fileReader := newFileReader(t)

	var got map[string]any
	require.NoError(t, fileReader.Read(path, &got))

	assert.Equal(t, "value", got["key"])
	assert.Equal(t, float64(42), got["count"])
}

func TestRead_InvalidJSON(t *testing.T) {
	path := writeTempJSON(t, "invalid.json", `{"name":`)

	fileReader := newFileReader(t)

	var got testData
	err := fileReader.Read(path, &got)

	var syntaxErr *json.SyntaxError
	require.Error(t, err)
	assert.ErrorAs(t, err, &syntaxErr)
}

func TestRead_FileNotFound(t *testing.T) {
	path := filepath.Join("temp", "does-not-exist.json")
	if err := os.Remove(path); err != nil {
		require.ErrorIs(t, err, os.ErrNotExist)
	}

	fileReader := newFileReader(t)

	var got testData
	require.ErrorIs(t, fileReader.Read(path, &got), os.ErrNotExist)
}

func TestRead_RejectsNonPointerTarget(t *testing.T) {
	path := writeTempJSON(t, "non-pointer.json", `{"name":"Alice","age":30}`)

	fileReader := newFileReader(t)

	require.Error(t, fileReader.Read(path, testData{}))
}

func TestRead_ReReadsWhenTargetTypeChanges(t *testing.T) {
	path := writeTempJSON(t, "target-type.json", `{"name":"Alice","age":30}`)

	fileReader := newFileReader(t)

	var asStruct testData
	require.NoError(t, fileReader.Read(path, &asStruct))
	assert.Equal(t, "Alice", asStruct.Name)

	var asMap map[string]any
	require.NoError(t, fileReader.Read(path, &asMap))
	assert.Equal(t, "Alice", asMap["name"])
}

func TestRead_CachesFileContents(t *testing.T) {
	path := writeTempJSON(t, "cached.json", `{"name":"Alice","age":30}`)

	fileReader := newFileReader(t)

	var before testData
	require.NoError(t, fileReader.Read(path, &before))
	assert.Equal(t, "Alice", before.Name)

	require.NoError(t, os.WriteFile(path, []byte(`{"name":"Bob","age":31}`), 0o644))

	var after testData
	require.NoError(t, fileReader.Read(path, &after))
	assert.Equal(t, "Alice", after.Name, "expected cached value when watcher is not started")
}

func TestRead_InvalidatesCacheOnChange(t *testing.T) {
	path := writeTempJSON(t, "changing.json", `{"name":"Alice","age":30}`)

	logger := &testLogger{}
	fileReader, err := fread.New(logger)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, fileReader.Close())
	})

	fileReader.Start()

	var before testData
	require.NoError(t, fileReader.Read(path, &before))
	assert.Equal(t, "Alice", before.Name)

	require.NoError(t, os.WriteFile(path, []byte(`{"name":"Bob","age":31}`), 0o644))

	require.Eventually(t, func() bool {
		return logger.count() > 0
	}, 2*time.Second, 10*time.Millisecond, "expected a file changed log entry")

	var after testData
	require.NoError(t, fileReader.Read(path, &after))
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

func newFileReader(t *testing.T) *fread.Read {
	t.Helper()

	fileReader, err := fread.New(&testLogger{})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, fileReader.Close())
	})

	return fileReader
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
