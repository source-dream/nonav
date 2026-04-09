package httpserver

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"nonav/server/internal/config"
)

type FRPProxySpec struct {
	ShareID    int64
	ProxyName  string
	LocalHost  string
	LocalPort  int
	RemotePort int
}

type FRPProcessManager struct {
	cfg     config.Config
	service string
	logs    *SystemLogBuffer
	mu      sync.Mutex
	procs   map[int64]*exec.Cmd
	specs   map[int64]FRPProxySpec
	fails   map[int64]int
}

type FRPProcessSnapshot struct {
	Enabled         bool
	DesiredCount    int
	RunningCount    int
	RestartingCount int
}

func NewFRPProcessManager(cfg config.Config, logs *SystemLogBuffer, service string) *FRPProcessManager {
	return &FRPProcessManager{
		cfg:     cfg,
		service: service,
		logs:    logs,
		procs:   make(map[int64]*exec.Cmd),
		specs:   make(map[int64]FRPProxySpec),
		fails:   make(map[int64]int),
	}
}

func (m *FRPProcessManager) StartProxy(spec FRPProxySpec) error {
	m.mu.Lock()
	m.specs[spec.ShareID] = spec
	m.fails[spec.ShareID] = 0
	m.mu.Unlock()

	return m.startProxyOnce(spec)
}

func (m *FRPProcessManager) startProxyOnce(spec FRPProxySpec) error {
	m.mu.Lock()
	if _, exists := m.procs[spec.ShareID]; exists {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	args := []string{
		"tcp",
		"-s", m.cfg.FRPServerAddr,
		"-P", strconv.Itoa(m.cfg.FRPServerPort),
		"-n", spec.ProxyName,
		"-i", spec.LocalHost,
		"-l", strconv.Itoa(spec.LocalPort),
		"-r", strconv.Itoa(spec.RemotePort),
		"--log-file", "console",
		"--log-level", "warn",
	}
	if m.cfg.FRPAuthToken != "" {
		args = append(args, "-t", m.cfg.FRPAuthToken)
	}

	cmd := exec.Command(m.cfg.FRPClientBin, args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start frpc proxy process: %w", err)
	}
	m.record("info", "frpc_started", fmt.Sprintf("frpc proxy started (share=%d)", spec.ShareID), fmt.Sprintf("proxy=%s", spec.ProxyName), fmt.Sprintf("remote_port=%d", spec.RemotePort))

	m.mu.Lock()
	m.procs[spec.ShareID] = cmd
	m.mu.Unlock()
	startedAt := time.Now()

	go func(shareID int64, process *exec.Cmd, captured *bytes.Buffer) {
		err := process.Wait()
		if err != nil {
			msg := strings.TrimSpace(captured.String())
			if msg != "" {
				log.Printf("frpc proxy process exited with error (share=%d): %v, output: %s", shareID, err, msg)
				m.record("error", "frpc_exited", fmt.Sprintf("frpc proxy exited with error (share=%d)", shareID), fmt.Sprintf("error=%v", err), "output="+msg)
			} else {
				log.Printf("frpc proxy process exited with error (share=%d): %v", shareID, err)
				m.record("error", "frpc_exited", fmt.Sprintf("frpc proxy exited with error (share=%d)", shareID), fmt.Sprintf("error=%v", err))
			}
		} else {
			m.record("info", "frpc_exited", fmt.Sprintf("frpc proxy exited (share=%d)", shareID))
		}

		m.mu.Lock()
		current := m.procs[shareID]
		delete(m.procs, shareID)
		spec, shouldRestart := m.specs[shareID]
		failCount := m.fails[shareID]
		uptime := time.Since(startedAt)
		if uptime < 3*time.Second {
			failCount++
			m.fails[shareID] = failCount
		} else {
			failCount = 0
			m.fails[shareID] = 0
		}
		m.mu.Unlock()

		if current != process {
			return
		}

		if shouldRestart {
			if failCount >= 6 {
				log.Printf("frpc proxy process disabled after repeated failures (share=%d)", shareID)
				m.record("error", "frpc_disabled", fmt.Sprintf("frpc proxy disabled after repeated failures (share=%d)", shareID), fmt.Sprintf("fail_count=%d", failCount))
				m.mu.Lock()
				delete(m.specs, shareID)
				delete(m.fails, shareID)
				m.mu.Unlock()
				return
			}

			backoff := restartBackoff(failCount)
			time.Sleep(backoff)

			m.mu.Lock()
			_, stillDesired := m.specs[shareID]
			_, alreadyRunning := m.procs[shareID]
			m.mu.Unlock()
			if !stillDesired || alreadyRunning {
				return
			}

			if restartErr := m.startProxyOnce(spec); restartErr != nil {
				log.Printf("frpc proxy process restart failed (share=%d): %v", shareID, restartErr)
				m.record("error", "frpc_restart_failed", fmt.Sprintf("frpc proxy restart failed (share=%d)", shareID), fmt.Sprintf("error=%v", restartErr))
			}
		}
	}(spec.ShareID, cmd, &output)

	time.Sleep(200 * time.Millisecond)
	return nil
}

func (m *FRPProcessManager) StopProxy(shareID int64) error {
	m.mu.Lock()
	cmd, exists := m.procs[shareID]
	delete(m.specs, shareID)
	delete(m.fails, shareID)
	if exists {
		delete(m.procs, shareID)
	}
	m.mu.Unlock()

	if !exists || cmd == nil || cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Kill(); err != nil {
		return fmt.Errorf("stop frpc proxy process: %w", err)
	}
	m.record("info", "frpc_stopped", fmt.Sprintf("frpc proxy stopped (share=%d)", shareID))

	return nil
}

func (m *FRPProcessManager) StopAll() {
	m.mu.Lock()
	ids := make([]int64, 0, len(m.specs))
	for id := range m.specs {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		_ = m.StopProxy(id)
	}
}

func (m *FRPProcessManager) Snapshot() FRPProcessSnapshot {
	if m == nil {
		return FRPProcessSnapshot{}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	restarting := 0
	for shareID := range m.specs {
		if _, running := m.procs[shareID]; running {
			continue
		}
		restarting++
	}

	return FRPProcessSnapshot{
		Enabled:         m.cfg.ForceFRP,
		DesiredCount:    len(m.specs),
		RunningCount:    len(m.procs),
		RestartingCount: restarting,
	}
}

func (m *FRPProcessManager) record(level string, event string, message string, details ...string) {
	if m == nil || m.logs == nil {
		return
	}
	m.logs.Add(m.service, "frpc", level, event, "-", message, details...)
}

func restartBackoff(failCount int) time.Duration {
	if failCount <= 0 {
		return 1 * time.Second
	}

	if failCount > 5 {
		failCount = 5
	}

	seconds := 1 << (failCount - 1)
	return time.Duration(seconds) * time.Second
}
