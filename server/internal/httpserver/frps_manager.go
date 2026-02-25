package httpserver

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"nonav/server/internal/config"
)

type FRPServerProcessManager struct {
	cfg      config.Config
	mu       sync.Mutex
	proc     *exec.Cmd
	desired  bool
	restarts int
}

func NewFRPServerProcessManager(cfg config.Config) *FRPServerProcessManager {
	return &FRPServerProcessManager{cfg: cfg}
}

func (m *FRPServerProcessManager) Start() error {
	if !m.cfg.EmbedFRPServer {
		return nil
	}

	m.mu.Lock()
	m.desired = true
	m.mu.Unlock()

	return m.startOnce()
}

func (m *FRPServerProcessManager) startOnce() error {
	m.mu.Lock()
	if m.proc != nil {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	args := []string{
		"--bind-addr", m.cfg.FRPServerBindAddr,
		"--bind-port", strconv.Itoa(m.cfg.FRPServerPort),
		"--log-file", "console",
		"--log-level", "warn",
	}
	if m.cfg.FRPAuthToken != "" {
		args = append(args, "-t", m.cfg.FRPAuthToken)
	}

	cmd := exec.Command(m.cfg.FRPServerBin, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start embedded frps: %w", err)
	}

	m.mu.Lock()
	m.proc = cmd
	m.mu.Unlock()

	go m.watch(cmd)
	return nil
}

func (m *FRPServerProcessManager) watch(process *exec.Cmd) {
	err := process.Wait()
	if err != nil {
		log.Printf("embedded frps exited: %v", err)
	}

	m.mu.Lock()
	current := m.proc
	m.proc = nil
	desired := m.desired
	m.mu.Unlock()

	if current != process || !desired {
		return
	}

	time.Sleep(1 * time.Second)
	if err := m.startOnce(); err != nil {
		log.Printf("embedded frps restart failed: %v", err)
	}
}

func (m *FRPServerProcessManager) Stop() error {
	m.mu.Lock()
	m.desired = false
	proc := m.proc
	m.proc = nil
	m.mu.Unlock()

	if proc == nil || proc.Process == nil {
		return nil
	}

	if err := proc.Process.Kill(); err != nil {
		return fmt.Errorf("stop embedded frps: %w", err)
	}

	return nil
}
