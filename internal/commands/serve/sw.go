// Package serve — service worker version injection.
//
// loadServiceWorker reads the embedded sw.js once at startup, substitutes
// the cache-version placeholder with the running build's semver, and
// returns the resulting bytes for cached serving. The placeholder is
// asserted present so a future templates refactor that drops it fails
// loud at startup rather than silently shipping un-versioned caches.
//
// Operator overlay precedence: see serveSwJs in overlay.go. An operator
// drop at <STATIC_PATH>/sw.js serves raw — the operator owns their cache
// version and the auto-bump path does not touch their bytes.
package serve

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"
	"time"
)

const cacheVersionPlaceholder = "__MARKGO_CACHE_VERSION__"

// swCacheVersion derives the cache-version string from a build-time semver.
// Strips a leading "v" so cache names read e.g. "markgo-precache-v3.17.0"
// rather than "markgo-precache-vv3.17.0". Empty input falls back to a
// per-startup "dev-<unix>" stamp so unstamped builds (make dev / air
// live-reload / plain go build) rotate their SW cache names on every server
// restart — otherwise a stale CacheStorage from a previous dev binary keeps
// serving old CSS/JS forever, because activate-cleanup only removes caches
// whose NAME differs, and "dev" never does.
func swCacheVersion(buildVersion string, now time.Time) string {
	v := strings.TrimPrefix(buildVersion, "v")
	if v == "" || v == "dev" {
		return fmt.Sprintf("dev-%d", now.Unix())
	}
	return v
}

// loadServiceWorker reads sw.js from the given embedded FS, validates the
// __MARKGO_CACHE_VERSION__ placeholder is present, and returns the bytes
// with the placeholder substituted to version. Returns a startup-time
// error if sw.js is missing or the placeholder is absent — both are
// build invariants and should never be observed at runtime.
//
// The returned modTime is time.Now() at substitution; SW scripts are
// fetched no-cache by browsers, so Last-Modified is not load-bearing.
func loadServiceWorker(embedded fs.FS, version string) ([]byte, time.Time, error) {
	raw, err := fs.ReadFile(embedded, "sw.js")
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("read embedded sw.js: %w", err)
	}
	if !bytes.Contains(raw, []byte(cacheVersionPlaceholder)) {
		return nil, time.Time{}, fmt.Errorf("embedded sw.js missing %s placeholder", cacheVersionPlaceholder)
	}
	substituted := bytes.ReplaceAll(raw, []byte(cacheVersionPlaceholder), []byte(swCacheVersion(version, time.Now())))
	return substituted, time.Now(), nil
}
