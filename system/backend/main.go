package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type DeviceStats struct {
	TotalMemoryMB uint64 `json:"totalMemoryMB"`
	UsedMemoryMB  uint64 `json:"usedMemoryMB"`
	TotalDiskBytes uint64 `json:"totalDiskBytes"`
	UsedDiskBytes  uint64 `json:"usedDiskBytes"`
	CPUPercent     float64 `json:"cpuPercent"`
}

type ProcessInfo struct {
	PID        int     `json:"pid"`
	Name       string  `json:"name"`
	MemPercent float64 `json:"memPercent"`
	RSSMB      int     `json:"rssMB"`
}

type FileInfo struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
}

func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func withRecover(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next(w, r)
	}
}

func sendJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

func handleDevice(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	var totalMemory uint64
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					kb, err := strconv.ParseUint(fields[1], 10, 64)
					if err == nil {
						totalMemory = kb * 1024
						break
					}
				}
			}
		}
	}
	if totalMemory == 0 {
		totalMemory = mem.Sys
	}

	cmd := exec.Command("df", "-B1", "/")
	out, err := cmd.Output()
	totalDisk := uint64(0)
	usedDisk := uint64(0)
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 4 {
				totalDisk, _ = strconv.ParseUint(fields[1], 10, 64)
				free, _ := strconv.ParseUint(fields[3], 10, 64)
				usedDisk = totalDisk - free
			}
		}
	}

	sendJSON(w, DeviceStats{
		TotalMemoryMB: totalMemory / (1024 * 1024),
		UsedMemoryMB:  mem.Alloc / (1024 * 1024),
		TotalDiskBytes: totalDisk,
		UsedDiskBytes:  usedDisk,
		CPUPercent:     avgCPU(),
	})
}

func avgCPU() float64 {
	cur := readCPU()
	dIdle := cur.idle - lastCPU.idle
	dTotal := cur.total - lastCPU.total
	lastCPU = cur
	if dTotal == 0 {
		return 0
	}
	return float64(dTotal-dIdle) / float64(dTotal) * 100
}

var lastCPU = cpuTimes{}

type cpuTimes struct {
	idle  uint64
	total uint64
}

func readCPU() cpuTimes {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuTimes{}
	}
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			values := make([]uint64, len(fields)-1)
			var idle uint64
			for i, f := range fields[1:] {
				v, _ := strconv.ParseUint(f, 10, 64)
				values[i] = v
				if i == 3 || i == 4 {
					idle += v
				}
			}
			var total uint64
			for _, v := range values {
				total += v
			}
			return cpuTimes{idle: idle, total: total}
		}
	}
	return cpuTimes{}
}

func handleTopProcesses(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if q := r.URL.Query().Get("limit"); q != "" {
		if parsed, err := strconv.Atoi(q); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	cmd := exec.Command("ps", "aux", "--sort=-%mem")
	out, err := cmd.Output()
	if err != nil {
		http.Error(w, "ps failed", http.StatusInternalServerError)
		return
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var results []ProcessInfo
	for i, line := range lines {
		if i == 0 || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}
		pid, _ := strconv.Atoi(fields[1])
		mem, _ := strconv.ParseFloat(fields[3], 64)
		if pid == 0 || fields[10] == "" {
			continue
		}
		var rssMB int
		cmd2 := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid))
		if rssOut, rssErr := cmd2.Output(); rssErr == nil {
			rssStr := strings.TrimSpace(string(rssOut))
			if value, convErr := strconv.Atoi(rssStr); convErr == nil {
				rssMB = value / 1024
			}
		}
		results = append(results, ProcessInfo{
			PID:        pid,
			Name:       fields[10],
			MemPercent: mem,
			RSSMB:      rssMB,
		})
		if len(results) >= limit {
			break
		}
	}
	sendJSON(w, results)
}

func handleTopFiles(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if q := r.URL.Query().Get("limit"); q != "" {
		if parsed, err := strconv.Atoi(q); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	var fileResults []FileInfo
	_ = walkFiles("/home", func(path string, size int64) {
		fileResults = append(fileResults, FileInfo{Path: path, SizeBytes: size})
	}, limit)
	if len(fileResults) > limit {
		fileResults = fileResults[:limit]
	}
	// Sort descending
	for i := 0; i < len(fileResults); i++ {
		for j := i + 1; j < len(fileResults); j++ {
			if fileResults[j].SizeBytes > fileResults[i].SizeBytes {
				fileResults[i], fileResults[j] = fileResults[j], fileResults[i]
			}
		}
	}
	if len(fileResults) > limit {
		fileResults = fileResults[:limit]
	}
	sendJSON(w, fileResults)
}

func walkFiles(root string, fn func(string, int64), max int) error {
	if max <= 0 {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		full := root + "/" + e.Name()
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.IsDir() && !isLink(full) {
			if err := walkFiles(full, fn, max); err != nil {
				return err
			}
		}
		fn(full, info.Size())
	}
	return nil
}

func isLink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

func serveReact(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		full := dir + r.URL.Path
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			full = dir + "/index.html"
		}
		b, err := os.ReadFile(full)
		if err != nil {
			http.ServeFile(w, r, dir+"/index.html")
			return
		}
		w.Header().Set("Content-Type", contentType(r.URL.Path))
		_, _ = w.Write(b)
	}
}

func contentType(path string) string {
	switch {
	case strings.HasSuffix(path, ".html"):
		return "text/html"
	case strings.HasSuffix(path, ".css"):
		return "text/css"
	case strings.HasSuffix(path, ".js"):
		return "application/javascript"
	case strings.HasSuffix(path, ".json"):
		return "application/json"
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	lastCPU = readCPU()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/device", withRecover(enableCORS(handleDevice)))
	mux.HandleFunc("/api/v1/top/processes", withRecover(enableCORS(handleTopProcesses)))
	mux.HandleFunc("/api/v1/top/files", withRecover(enableCORS(handleTopFiles)))
	mux.Handle("/", serveReact("/home/opc/system/frontend/dist"))
	log.Printf("system-monitor listening on 127.0.0.1:%s", port)
	if err := http.ListenAndServe("127.0.0.1:"+port, mux); err != nil {
		log.Fatal(err)
	}
}
