package playback

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Session is one viewer's active HLS transcode: a running ffmpeg process
// writing segments and a playlist into Dir.
type Session struct {
	ID  string
	Dir string

	mu       sync.Mutex
	cmd      *exec.Cmd
	lastUsed time.Time
	failed   bool
	failErr  error
}

func (s *Session) touch() {
	s.mu.Lock()
	s.lastUsed = time.Now()
	s.mu.Unlock()
}

func (s *Session) idleSince(cutoff time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastUsed.Before(cutoff)
}

func (s *Session) markFailed(err error) {
	s.mu.Lock()
	s.failed = true
	s.failErr = err
	s.mu.Unlock()
}

// Failed reports whether this session's ffmpeg process exited with an
// error. A clean exit (the transcode completed successfully) does not
// count as failed — Failed only reports process-level failures, not
// normal completion.
func (s *Session) Failed() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed, s.failErr
}

func (s *Session) kill() {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// SessionManager owns every active transcode session: starting new ones,
// looking them up, and cleaning up idle ones. Entirely in-memory — nothing
// here is persisted, matching this product's principle that nothing is
// lost on restart and nothing ticks with no viewers (a server restart
// simply drops all active sessions).
type SessionManager struct {
	baseDir        string
	idleTimeout    time.Duration
	startupTimeout time.Duration

	mu       sync.Mutex
	sessions map[string]*Session
}

// defaultStartupTimeout bounds how long StartSession waits for ffmpeg to
// produce a playlist before giving up. 15s is generous relative to
// -preset veryfast's encode speed, leaving headroom for real content on
// modest hardware rather than the tight 5s this used to be hardcoded to.
const defaultStartupTimeout = 15 * time.Second

func NewSessionManager(baseDir string, idleTimeout time.Duration) *SessionManager {
	return &SessionManager{
		baseDir:        baseDir,
		idleTimeout:    idleTimeout,
		startupTimeout: defaultStartupTimeout,
		sessions:       make(map[string]*Session),
	}
}

// StartSession starts an ffmpeg process transcoding mediaPath to HLS
// beginning at offsetSec, and waits (bounded by a short startup timeout)
// for it to actually produce a playlist before returning — so a bad
// input/binary fails this call directly, rather than only surfacing later
// as a 404 on the first segment request.
func (m *SessionManager) StartSession(mediaPath string, offsetSec float64) (*Session, error) {
	id := uuid.New().String()
	dir := filepath.Join(m.baseDir, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating session directory: %w", err)
	}

	playlistPath := filepath.Join(dir, "playlist.m3u8")
	segmentPattern := filepath.Join(dir, "segment%03d.ts")

	// exec.Command, not exec.CommandContext: this process must outlive the
	// HTTP request that starts it. Its lifecycle is owned by this
	// SessionManager — torn down by the idle sweep (Sweep) or on server
	// shutdown (Close) — not by any single request's context.
	cmd := exec.Command("ffmpeg",
		"-y",
		"-ss", strconv.FormatFloat(offsetSec, 'f', 3, 64),
		"-i", mediaPath,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-c:a", "aac",
		"-force_key_frames", "expr:gte(t,n_forced*2)",
		"-nostats",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_playlist_type", "event",
		"-hls_segment_filename", segmentPattern,
		playlistPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("starting ffmpeg: %w", err)
	}

	sess := &Session{ID: id, Dir: dir, cmd: cmd, lastUsed: time.Now()}

	go func() {
		waitErr := cmd.Wait()
		if waitErr != nil {
			sess.markFailed(fmt.Errorf("ffmpeg exited: %w: %s", waitErr, stderr.String()))
		}
	}()

	deadline := time.Now().Add(m.startupTimeout)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(playlistPath); statErr == nil {
			m.mu.Lock()
			m.sessions[id] = sess
			m.mu.Unlock()
			return sess, nil
		}
		if failed, ferr := sess.Failed(); failed {
			os.RemoveAll(dir)
			return nil, fmt.Errorf("ffmpeg failed to start: %w", ferr)
		}
		time.Sleep(50 * time.Millisecond)
	}

	sess.kill()
	os.RemoveAll(dir)
	return nil, errors.New("ffmpeg did not produce a playlist within the startup timeout")
}

// Get returns the session with the given id, if one is currently tracked.
func (m *SessionManager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// Touch records that id was just accessed (a playlist or segment
// request), resetting its idle clock. A no-op if id isn't tracked.
func (m *SessionManager) Touch(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if ok {
		s.touch()
	}
}

// Sweep tears down (kills the process, removes the directory) every
// session that hasn't been touched since idleTimeout before now. Called
// periodically by Run in production; called directly with a controlled
// now in tests, so tests never wait out a real idle timeout.
func (m *SessionManager) Sweep(now time.Time) {
	cutoff := now.Add(-m.idleTimeout)

	m.mu.Lock()
	var idle []*Session
	for id, s := range m.sessions {
		if s.idleSince(cutoff) {
			idle = append(idle, s)
			delete(m.sessions, id)
		}
	}
	m.mu.Unlock()

	for _, s := range idle {
		s.kill()
		os.RemoveAll(s.Dir)
	}
}

// Run periodically sweeps idle sessions until ctx is cancelled. Intended
// to be started once, in its own goroutine, at server startup.
func (m *SessionManager) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.Sweep(now)
		}
	}
}

// Close tears down every currently active session immediately, regardless
// of idle time. Intended for graceful server shutdown, so no ffmpeg
// process is ever left running after the server process exits.
func (m *SessionManager) Close() {
	m.mu.Lock()
	all := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()

	for _, s := range all {
		s.kill()
		os.RemoveAll(s.Dir)
	}
}

// CleanOrphanedSessions removes any leftover session directories under
// baseDir from an unclean prior shutdown (e.g. the process was killed
// before Close() ran). Call once at startup, before serving traffic.
func CleanOrphanedSessions(baseDir string) error {
	entries, err := os.ReadDir(baseDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(baseDir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
