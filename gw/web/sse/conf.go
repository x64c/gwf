package sse

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Conf is what the service runs on: the values that hold for every stream on the app's path. Every value is required.
type Conf struct {
	FrameWriteTimeoutSecs int `json:"frame_write_timeout_secs"` // REQUIRED (> 0). How long writing one frame (an event or a heartbeat, with its flush) may take; a client that cannot take it within this is treated as gone, so no stream can hold a stop or a drain open.
	RetryMillis           int `json:"retry_millis"`             // REQUIRED (> 0). The reconnection wait sent with retry: when the service ends a stream itself (its source ended, a drain, a stop). Keep it above how long a restart takes, or a client's return lands while nothing serves it.
}

// LoadConf reads <appRoot>/config/.web-sse.json, rejecting unknown members, and validates what it read.
func LoadConf(appRoot string) (Conf, error) {
	path := filepath.Join(appRoot, "config", ".web-sse.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return Conf{}, err
	}
	var conf Conf
	if err = json.Unmarshal(b, &conf, json.RejectUnknownMembers(true)); err != nil {
		return Conf{}, fmt.Errorf("sse conf %s: %w", path, err)
	}
	if err = conf.Validate(); err != nil {
		return Conf{}, fmt.Errorf("sse conf %s: %w", path, err)
	}
	return conf, nil
}

// Validate refuses a conf with a value that is unset or cannot describe what it names.
func (c Conf) Validate() error {
	if c.FrameWriteTimeoutSecs <= 0 {
		return fmt.Errorf("frame_write_timeout_secs must be set (seconds > 0): got %d", c.FrameWriteTimeoutSecs)
	}
	if c.RetryMillis <= 0 {
		return fmt.Errorf("retry_millis must be set (milliseconds > 0): got %d", c.RetryMillis)
	}
	return nil
}

func (c Conf) frameWriteTimeout() time.Duration {
	return time.Duration(c.FrameWriteTimeoutSecs) * time.Second
}
