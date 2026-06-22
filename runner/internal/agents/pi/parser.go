package pi

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/agentsmesh/runner/internal/logger"
	"github.com/anthropics/agentsmesh/runner/internal/tokenusage"
)

type piParser struct{}

type piUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
}

type piEntry struct {
	Type    string `json:"type"`
	Message struct {
		Model string  `json:"model"`
		Usage piUsage `json:"usage"`
	} `json:"message"`
}

// Parse sums per-message token usage from pi session JSONL in the pod-local
// config dir (PI_CODING_AGENT_DIR, materialized at <sandbox>/pi-home by the
// agentfile + RegisterAgentHome). Sessions land under that dir's sessions/.
func (p *piParser) Parse(sandboxPath string, podStartedAt time.Time) (*tokenusage.TokenUsage, error) {
	usage := tokenusage.NewTokenUsage()
	if sandboxPath == "" {
		return nil, nil
	}
	sessionsDir := filepath.Join(sandboxPath, "pi-home", "sessions")
	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		return nil, nil
	}

	walkErr := filepath.WalkDir(sessionsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if !tokenusage.IsModifiedAfter(path, podStartedAt) {
			return nil
		}
		if perr := parsePiSessionFile(path, usage); perr != nil {
			logger.Pod().Warn("Pi parser: file parse error", "file", path, "error", perr)
		}
		return nil
	})
	if walkErr != nil {
		logger.Pod().Warn("Pi parser: walk error", "dir", sessionsDir, "error", walkErr)
	}

	if usage.IsEmpty() {
		return nil, nil
	}
	return usage, nil
}

func parsePiSessionFile(path string, usage *tokenusage.TokenUsage) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e piEntry
		if json.Unmarshal(line, &e) != nil || e.Type != "message" {
			continue
		}
		u := e.Message.Usage
		if e.Message.Model == "" || (u.Input == 0 && u.Output == 0 && u.CacheRead == 0 && u.CacheWrite == 0) {
			continue
		}
		usage.Add(e.Message.Model, u.Input, u.Output, u.CacheWrite, u.CacheRead)
	}
	return scanner.Err()
}
