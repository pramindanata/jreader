package jreader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Logger receives the notifications emitted by Read while it watches files.
type Logger interface {
	// Info logs an informational message, such as a detected file change.
	Info(msg string, args ...any)

	// Error logs a watcher error.
	Error(msg string, args ...any)
}

// JReader decodes JSON files and caches the decoded value for every file path it
// has loaded. A cache entry is dropped when its watched file changes.
type JReader struct {
	mu             sync.RWMutex
	cacheMap       map[string]any
	watchedPathMap map[string]struct{}
	watcher        *fsnotify.Watcher
	logger         Logger
}

// New creates a JReader together with its file watcher.
//
// The supplied logger may be nil, in which case watcher events and watcher
// errors are silently discarded. Call Start to begin watching, and Close to
// release the watcher when the JReader is no longer needed.
func New(logger Logger) (*JReader, error) {
	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	return &JReader{
		cacheMap:       make(map[string]any),
		watchedPathMap: make(map[string]struct{}),
		watcher:        watcher,
		logger:         logger,
	}, nil
}

// Start watches the registered directories in a background goroutine and
// invalidates the matching cache entry whenever a watched file changes.
//
// Start does not block. It is a no-op when the watcher is unavailable.
func (r *JReader) Start() {
	if r.watcher == nil {
		return
	}

	go r.watch()
}

// Close stops the watcher and releases its resources. It is a no-op when the
// watcher is unavailable.
func (r *JReader) Close() error {
	if r.watcher == nil {
		return nil
	}

	return r.watcher.Close()
}

// Read loads the JSON file at path and decodes it into value, which must be a
// non-nil pointer.
//
// The decoded value is cached per path. On a later call with the same pointer
// type the cached value is copied into value without re-parsing the file. When
// the target type differs from the cached type, the file is read and decoded
// again and the cache entry is replaced.
//
// Read returns an error when value is not a non-nil pointer, the file cannot be
// read, or the file content is not valid JSON.
func (r *JReader) Read(path string, value any) error {
	target := reflect.ValueOf(value)

	if target.Kind() != reflect.Pointer || target.IsNil() {
		return fmt.Errorf("value must be a non-nil pointer, got %T", value)
	}

	key := filepath.Clean(path)

	if cached, ok := r.getCacheByKey(key); ok {
		cachedValue := reflect.ValueOf(cached)

		if cachedValue.Type() == target.Type() {
			target.Elem().Set(cachedValue.Elem())

			return nil
		}
	}

	data, err := os.ReadFile(key)

	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	if err := r.watchDir(filepath.Dir(key)); err != nil {
		return err
	}

	holder := reflect.New(target.Type().Elem())

	if err := json.Unmarshal(data, holder.Interface()); err != nil {
		return err
	}

	r.setCacheByKey(key, holder.Interface())
	target.Elem().Set(holder.Elem())

	return nil
}

func (r *JReader) getCacheByKey(key string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	value, ok := r.cacheMap[key]

	return value, ok
}

func (r *JReader) setCacheByKey(key string, value any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cacheMap == nil {
		r.cacheMap = make(map[string]any)
	}

	r.cacheMap[key] = value
}

func (r *JReader) watchDir(dir string) error {
	if r.watcher == nil {
		return nil
	}

	r.mu.RLock()
	_, ok := r.watchedPathMap[dir]
	r.mu.RUnlock()

	if ok {
		return nil
	}

	if err := r.watcher.Add(dir); err != nil {
		return fmt.Errorf("failed to watch directory %q: %w", dir, err)
	}

	r.mu.Lock()

	if r.watchedPathMap == nil {
		r.watchedPathMap = make(map[string]struct{})
	}

	r.watchedPathMap[dir] = struct{}{}
	r.mu.Unlock()

	return nil
}

func (r *JReader) watch() {
	for {
		select {
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}

			r.handleFileEvent(event)
		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}

			if r.logger != nil {
				r.logger.Error("file watcher error", "error", err)
			}
		}
	}
}

func (r *JReader) handleFileEvent(event fsnotify.Event) {
	key := filepath.Clean(event.Name)

	r.mu.Lock()
	delete(r.cacheMap, key)
	r.mu.Unlock()

	if r.logger != nil {
		r.logger.Info("file changed", "path", key, "op", event.Op.String())
	}
}
