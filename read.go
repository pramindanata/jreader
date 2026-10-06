package fread

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}

type Read struct {
	mu             sync.RWMutex
	cacheMap       map[string]any
	watchedPathMap map[string]struct{}
	watcher        *fsnotify.Watcher
	logger         Logger
}

func New(logger Logger) (*Read, error) {
	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	return &Read{
		cacheMap:       make(map[string]any),
		watchedPathMap: make(map[string]struct{}),
		watcher:        watcher,
		logger:         logger,
	}, nil
}

func (r *Read) Start() {
	if r.watcher == nil {
		return
	}

	go r.watch()
}

func (r *Read) Close() error {
	if r.watcher == nil {
		return nil
	}

	return r.watcher.Close()
}

func (r *Read) Read(path string, value any) error {
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

func (r *Read) getCacheByKey(key string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	value, ok := r.cacheMap[key]

	return value, ok
}

func (r *Read) setCacheByKey(key string, value any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cacheMap == nil {
		r.cacheMap = make(map[string]any)
	}

	r.cacheMap[key] = value
}

func (r *Read) watchDir(dir string) error {
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

func (r *Read) watch() {
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

func (r *Read) handleFileEvent(event fsnotify.Event) {
	key := filepath.Clean(event.Name)

	r.mu.Lock()
	delete(r.cacheMap, key)
	r.mu.Unlock()

	if r.logger != nil {
		r.logger.Info("file changed", "path", key, "op", event.Op.String())
	}
}
