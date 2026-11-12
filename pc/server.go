//go:build windows

// Wi-Fi side: a small HTTP API for the phone plus UDP auto-discovery.
package main

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	httpPort      = 47811
	discoveryPort = 47812
	discoverAsk   = "PHONELOCK_DISCOVER"
	discoverReply = "PHONELOCK_HERE:"
)

// Set at build time from shared/key.txt (see build.sh).
var secretKey string

func localIP() string {
	if c, err := net.Dial("udp", "8.8.8.8:80"); err == nil { // no packet is actually sent
		defer c.Close()
		if a, ok := c.LocalAddr().(*net.UDPAddr); ok && !a.IP.IsLoopback() {
			return a.IP.String()
		}
	}
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && n.IP.IsPrivate() {
				return n.IP.String()
			}
		}
	}
	return "unknown"
}

func pcName() string {
	h, _ := os.Hostname()
	if h == "" {
		h = "PC"
	}
	return h
}

func authorized(r *http.Request) bool {
	got := r.Header.Get("X-PhoneLock-Key")
	return subtle.ConstantTimeCompare([]byte(got), []byte(secretKey)) == 1
}

func writeStatus(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"locked": locked.Load(), "name": pcName()})
}

func handler(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if action != "status" && r.Method != http.MethodPost {
			http.Error(w, "use POST", http.StatusMethodNotAllowed)
			return
		}
		switch action {
		case "lock":
			setLocked(true)
		case "unlock":
			setLocked(false)
		case "toggle":
			setLocked(!locked.Load())
		}
		writeStatus(w)
	}
}

func startServer() error {
	mux := http.NewServeMux()
	for _, a := range []string{"status", "lock", "unlock", "toggle"} {
		mux.HandleFunc("/api/"+a, handler(a))
	}
	ln, err := net.Listen("tcp", ":47811")
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)
	go discoveryLoop()
	return nil
}

// Replies to the phone's broadcast so it can find this PC without typing an IP.
func discoveryLoop() {
	for {
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: discoveryPort})
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		buf := make([]byte, 256)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				break
			}
			if strings.TrimSpace(string(buf[:n])) == discoverAsk {
				conn.WriteToUDP([]byte(discoverReply+pcName()), from)
			}
		}
		conn.Close()
	}
}
