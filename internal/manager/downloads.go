package manager

import (
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/hash"
)

// DownloadStore manages single-use generated files for the UI to download.
type DownloadStore struct {
	m        map[string]*storeFile
	mutex    sync.Mutex
	now      func() time.Time
	tokenTTL time.Duration
}

type storeFile struct {
	path        string
	contentType string
	keep        bool
	ownerUserID string
	capability  authz.Capability
	expiresAt   time.Time
	expiryTimer *time.Timer
}

func NewDownloadStore() *DownloadStore {
	return &DownloadStore{
		m:        make(map[string]*storeFile),
		now:      time.Now,
		tokenTTL: 10 * time.Minute,
	}
}

func (s *DownloadStore) RegisterFile(fp string, contentType string, keep bool, principal authz.Principal, capability authz.Capability) (string, error) {
	const keyLength = 16
	const attempts = 100
	if err := authz.Require(principal, capability); err != nil {
		return "", err
	}

	var h string
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for attempt := 0; attempt < attempts; attempt++ {
		var err error
		h, err = hash.GenerateRandomKey(keyLength)
		if err != nil {
			return "", err
		}
		if _, exists := s.m[h]; !exists {
			file := &storeFile{
				path: fp, contentType: contentType, keep: keep,
				ownerUserID: principal.UserID, capability: capability,
				expiresAt: s.now().Add(s.tokenTTL),
			}
			s.m[h] = file
			file.expiryTimer = time.AfterFunc(s.tokenTTL, func() {
				s.expire(h, file)
			})
			return h, nil
		}
	}
	return "", errors.New("unable to allocate download token")
}

func (s *DownloadStore) Serve(hash string, principal authz.Principal, w http.ResponseWriter, r *http.Request) {
	s.mutex.Lock()
	f, ok := s.m[hash]
	if !ok || !principal.Owns(f.ownerUserID) || !principal.Allows(f.capability) || !s.now().Before(f.expiresAt) {
		if ok && !s.now().Before(f.expiresAt) {
			delete(s.m, hash)
			f.expiryTimer.Stop()
			if !f.keep {
				_ = os.Remove(f.path)
			}
		}
		s.mutex.Unlock()
		http.NotFound(w, r)
		return
	}
	delete(s.m, hash)
	f.expiryTimer.Stop()
	s.mutex.Unlock()

	if f.contentType != "" {
		w.Header().Add("Content-Type", f.contentType)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, f.path)
	if !f.keep {
		_ = os.Remove(f.path)
	}
}

func (s *DownloadStore) expire(hash string, expected *storeFile) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if current, ok := s.m[hash]; !ok || current != expected {
		return
	}
	delete(s.m, hash)
	if !expected.keep {
		_ = os.Remove(expected.path)
	}
}
