package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Config struct {
	AppCommand               string `json:"app_command"`
	CheckURL                 string `json:"check_url"`
	IntervalSeconds          int    `json:"interval_seconds"`
	LogFile                  string `json:"log_file"`
	MaxFailuresBeforeRestart int    `json:"max_failures_before_restart"`
}

type Monitor struct {
	cfg         Config
	cmd         *exec.Cmd
	mu          sync.Mutex
	failCounter int
}

func loadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return Config{}, err
	}

	if cfg.IntervalSeconds <= 0 {
		cfg.IntervalSeconds = 5
	}
	if cfg.MaxFailuresBeforeRestart <= 0 {
		cfg.MaxFailuresBeforeRestart = 1
	}

	return cfg, nil
}

func setupLogging(path string) error {
	if path == "" {
		// лог по умолчанию в stdout
		return nil
	}
	if err := os.MkdirAll("/var/log", 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	log.SetOutput(f)
	return nil
}

func (m *Monitor) startApp() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd != nil && m.cmd.Process != nil {
		return errors.New("app already running")
	}

	log.Printf("Starting app: %s\n", m.cfg.AppCommand)
	cmd := exec.Command(m.cfg.AppCommand)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	m.cmd = cmd

	// Отслеживаем завершение процесса в отдельной горутине
	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			log.Printf("App exited with error: %v\n", err)
		} else {
			log.Println("App exited normally")
		}
		m.cmd = nil
	}()

	return nil
}

func (m *Monitor) stopApp() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd == nil || m.cmd.Process == nil {
		return
	}

	log.Println("Stopping app...")
	_ = m.cmd.Process.Kill()
	m.cmd = nil
}

func (m *Monitor) restartApp() {
	log.Println("Restarting app...")
	m.stopApp()
	if err := m.startApp(); err != nil {
		log.Printf("Failed to restart app: %v\n", err)
	}
	m.failCounter = 0
}

func (m *Monitor) checkOnce() {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	resp, err := client.Get(m.cfg.CheckURL)
	if err != nil {
		log.Printf("Health check failed: %v\n", err)
		m.failCounter++
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Printf("Health check bad status: %d\n", resp.StatusCode)
			m.failCounter++
		} else {
			log.Printf("Health check OK")
			m.failCounter = 0
		}
	}

	// если приложение умерло само по себе — тоже рестартуем
	m.mu.Lock()
	appRunning := m.cmd != nil && m.cmd.Process != nil
	m.mu.Unlock()
	if !appRunning {
		log.Println("Detected app process not running, restarting...")
		m.restartApp()
		return
	}

	if m.failCounter >= m.cfg.MaxFailuresBeforeRestart {
		log.Printf("Failures >= %d, restarting app", m.cfg.MaxFailuresBeforeRestart)
		m.restartApp()
	}
}

func main() {
	configPath := os.Getenv("MONITOR_CONFIG")
	if configPath == "" {
		configPath = "/app/config.json"
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Cannot load config: %v\n", err)
	}

	if err := setupLogging(cfg.LogFile); err != nil {
		log.Fatalf("Cannot setup logging: %v\n", err)
	}

	log.Println("Monitor starting with config:", cfg)

	monitor := &Monitor{cfg: cfg}

	if err := monitor.startApp(); err != nil {
		log.Fatalf("Cannot start app: %v\n", err)
	}

	ticker := time.NewTicker(time.Duration(cfg.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			monitor.checkOnce()
		}
	}
}
