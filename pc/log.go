//go:build windows

// Tiny diagnostic log written next to PhoneLock.exe (PhoneLock.log).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const appVersion = "1.4"

var (
	logMu   sync.Mutex
	logPath string
)

func initLog(exe string) {
	logPath = filepath.Join(filepath.Dir(exe), "PhoneLock.log")
	if fi, err := os.Stat(logPath); err == nil && fi.Size() > 512*1024 {
		os.Remove(logPath)
	}
}

func logf(format string, a ...any) {
	if logPath == "" {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s  %s\r\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, a...))
}
