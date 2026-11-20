package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ─── API RESPONSE TYPES ───

type DashboardPayload struct {
	Mode             string          `json:"mode"`
	Timestamp        string          `json:"timestamp"`
	PollIntervalSec  int             `json:"poll_interval_sec"`
	Hostname         string          `json:"hostname"`
	UptimeHuman      string          `json:"uptime_human"`
	Sources          []string        `json:"sources"`
	Verdict          VerdictInfo     `json:"verdict"`
	ExecutiveSummary []ExecStat      `json:"executive_summary"`
	CriticalServices []ServiceStatus `json:"critical_services"`
	Infrastructure   InfraStatus     `json:"infrastructure"`
	ContainerHealth  ContainerHealth `json:"container_health"`
	Incidents        []IncidentItem  `json:"incidents"`
}

type VerdictInfo struct {
	Level string `json:"level"` // "ok", "warn", "crit"
	Text  string `json:"text"`
}

type ExecStat struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Source string `json:"source"`
	Status string `json:"status"` // "ok", "warn", "crit"
	Sub    string `json:"sub"`
}

type ServiceStatus struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Target   string `json:"target"`
	Online   bool   `json:"online"`
	Latency  string `json:"latency"`
	Error    string `json:"error,omitempty"`
}

type GaugeStatus struct {
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Status    string  `json:"status"` // "ok", "warn", "crit"
	StateTag  string  `json:"state_tag"`
	Detail    string  `json:"detail"`
	Threshold string  `json:"threshold"`
}

type NetworkStatus struct {
	Interface    string    `json:"interface"`
	InboundStr   string    `json:"inbound_str"`
	OutboundStr  string    `json:"outbound_str"`
	InboundMbps  float64   `json:"inbound_mbps"`
	OutboundMbps float64   `json:"outbound_mbps"`
	InHistory    []float64 `json:"in_history"`
	OutHistory   []float64 `json:"out_history"`
}

type InfraStatus struct {
	Source  string        `json:"source"`
	Gauges  []GaugeStatus `json:"gauges"`
	Network NetworkStatus `json:"network"`
}

type RestartEntry struct {
	Name   string `json:"name"`
	Count  int    `json:"count"`
	Reason string `json:"reason"`
	State  string `json:"state"`
}

type ContainerHealth struct {
	Available   bool           `json:"available"`
	Source      string         `json:"source"`
	Healthy     int            `json:"healthy"`
	Unhealthy   int            `json:"unhealthy"`
	Total       int            `json:"total"`
	Restarts    int            `json:"restarts"`
	TopRestarts []RestartEntry `json:"top_restarts"`
}

type IncidentItem struct {
	Level string `json:"level"` // "crit", "warn", "info"
	Badge string `json:"badge"`
	Title string `json:"title"`
	Meta  string `json:"meta"`
	Time  string `json:"time"`
	Rank  int    `json:"-"`
}

// ─── CONFIGURATION ───

type ServiceDef struct {
	Name     string
	Category string
	Target   string
}

type HostDef struct {
	Name   string
	Target string
}

type Config struct {
	ListenAddr       string
	PublicDir        string
	PollInterval     time.Duration
	DockerSocket     string
	ProcDir          string
	RootFSPath       string
	NetInterface     string
	CriticalServices []ServiceDef
	ExtraServices    []ServiceDef
	Hosts            []HostDef
	SSLDomains       []string
	UptimeKumaURL    string
	UptimeKumaKey    string
	UptimeKumaSlug   string
	PrometheusURL    string
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func parseServices(raw string) []ServiceDef {
	var list []ServiceDef
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "|")
		if len(parts) == 3 {
			list = append(list, ServiceDef{
				Name:     strings.TrimSpace(parts[0]),
				Category: strings.TrimSpace(parts[1]),
				Target:   strings.TrimSpace(parts[2]),
			})
		} else if len(parts) == 2 {
			list = append(list, ServiceDef{
				Name:     strings.TrimSpace(parts[0]),
				Category: "Service",
				Target:   strings.TrimSpace(parts[1]),
			})
		}
	}
	return list
}

func parseHosts(raw string) []HostDef {
	var list []HostDef
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "|")
		if len(parts) >= 2 {
			list = append(list, HostDef{
				Name:   strings.TrimSpace(parts[0]),
				Target: strings.TrimSpace(parts[1]),
			})
		} else {
			list = append(list, HostDef{Name: entry, Target: "local"})
		}
	}
	return list
}

func parseCSV(raw string) []string {
	var out []string
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func loadConfig() Config {
	pollSec := 10
	if v, err := strconv.Atoi(envOr("POLL_INTERVAL", "10")); err == nil && v >= 3 && v <= 120 {
		pollSec = v
	}

	procDir := "/host/proc"
	if _, err := os.Stat(filepath.Join(procDir, "stat")); err != nil {
		procDir = "/proc"
	}

	rootfs := envOr("HOST_ROOTFS", "/host/rootfs")
	if _, err := os.Stat(rootfs); err != nil {
		rootfs = "/"
	}

	defaultCrit := strings.Join([]string{
		"Vaultwarden|Passwort-Tresor|http://192.168.178.90:8081",
		"Immich|Foto-Cloud|http://192.168.178.90:2283",
		"Paperless|Dokumente|http://192.168.178.90:8000",
		"Nginx Proxy|Edge Proxy & TLS|http://192.168.178.90:81",
		"Uptime Kuma|Monitoring|http://192.168.178.90:3001",
		"Website|info.moritzhammer.org|http://192.168.178.90:8080",
	}, ",")

	sslRaw := envOr("SSL_DOMAINS", "info.moritzhammer.org")
	var sslDomains []string
	if !strings.EqualFold(sslRaw, "none") {
		sslDomains = parseCSV(sslRaw)
	}

	return Config{
		ListenAddr:       envOr("LISTEN_ADDR", ":80"),
		PublicDir:        envOr("PUBLIC_DIR", "/srv/public"),
		PollInterval:     time.Duration(pollSec) * time.Second,
		DockerSocket:     envOr("DOCKER_SOCKET", "/var/run/docker.sock"),
		ProcDir:          procDir,
		RootFSPath:       rootfs,
		NetInterface:     envOr("NET_INTERFACE", ""),
		CriticalServices: parseServices(envOr("CRITICAL_SERVICES", defaultCrit)),
		ExtraServices:    parseServices(envOr("EXTRA_SERVICES", "")),
		Hosts:            parseHosts(envOr("HOSTS", "Docker-Host|local")),
		SSLDomains:       sslDomains,
		UptimeKumaURL:    strings.TrimRight(envOr("UPTIME_KUMA_URL", ""), "/"),
		UptimeKumaKey:    envOr("UPTIME_KUMA_API_KEY", ""),
		UptimeKumaSlug:   envOr("UPTIME_KUMA_STATUS_SLUG", ""),
		PrometheusURL:    strings.TrimRight(envOr("PROMETHEUS_URL", ""), "/"),
	}
}

// ─── SYSTEM SAMPLER (/proc CPU, RAM, DISK, NETWORK) ───

type cpuSample struct {
	idle  uint64
	total uint64
}

type netSample struct {
	rxBytes uint64
	txBytes uint64
	at      time.Time
	iface   string
}

type SystemSampler struct {
	mu          sync.RWMutex
	procDir     string
	rootfsPath  string
	netIface    string
	prevCPU     *cpuSample
	prevNet     *netSample
	cpuPct      float64
	inMbps      float64
	outMbps     float64
	activeIface string
	inHist      []float64
	outHist     []float64
}

func NewSystemSampler(procDir, rootfs, netIface string) *SystemSampler {
	return &SystemSampler{
		procDir:    procDir,
		rootfsPath: rootfs,
		netIface:   netIface,
		inHist:     make([]float64, 0, 16),
		outHist:    make([]float64, 0, 16),
	}
}

func (s *SystemSampler) Start(ctx context.Context) {
	s.sampleOnce()
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sampleOnce()
			}
		}
	}()
}

func (s *SystemSampler) readCPU() (cpuSample, error) {
	f, err := os.Open(filepath.Join(s.procDir, "stat"))
	if err != nil {
		return cpuSample{}, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 5 {
				break
			}
			var vals []uint64
			for _, fld := range fields[1:] {
				v, _ := strconv.ParseUint(fld, 10, 64)
				vals = append(vals, v)
			}
			var total uint64
			for _, v := range vals {
				total += v
			}
			idle := vals[3]
			if len(vals) > 4 {
				idle += vals[4] // iowait
			}
			return cpuSample{idle: idle, total: total}, nil
		}
	}
	return cpuSample{}, fmt.Errorf("cpu line not found")
}

func (s *SystemSampler) readNet() (netSample, error) {
	f, err := os.Open(filepath.Join(s.procDir, "net/dev"))
	if err != nil {
		return netSample{}, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var rxSum, txSum uint64
	var picked []string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		iface := strings.TrimSpace(parts[0])
		if s.netIface != "" {
			if iface != s.netIface {
				continue
			}
		} else {
			if iface == "lo" ||
				strings.HasPrefix(iface, "docker") ||
				strings.HasPrefix(iface, "br-") ||
				strings.HasPrefix(iface, "veth") ||
				strings.HasPrefix(iface, "virbr") ||
				strings.HasPrefix(iface, "cni") ||
				strings.HasPrefix(iface, "flannel") {
				continue
			}
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 10 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		rxSum += rx
		txSum += tx
		picked = append(picked, iface)
	}
	label := "eth0"
	if len(picked) == 1 {
		label = picked[0]
	} else if len(picked) > 1 {
		label = strings.Join(picked[:minInt(2, len(picked))], "+")
	}
	return netSample{rxBytes: rxSum, txBytes: txSum, at: time.Now(), iface: label}, nil
}

func (s *SystemSampler) sampleOnce() {
	cNow, cErr := s.readCPU()
	nNow, nErr := s.readNet()

	s.mu.Lock()
	defer s.mu.Unlock()

	if cErr == nil {
		if s.prevCPU != nil && cNow.total > s.prevCPU.total {
			dt := float64(cNow.total - s.prevCPU.total)
			di := float64(cNow.idle - s.prevCPU.idle)
			pct := (1.0 - di/dt) * 100.0
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
			s.cpuPct = math.Round(pct*10) / 10
		}
		cp := cNow
		s.prevCPU = &cp
	}

	if nErr == nil {
		s.activeIface = nNow.iface
		if s.prevNet != nil {
			secs := nNow.at.Sub(s.prevNet.at).Seconds()
			if secs > 0.2 && nNow.rxBytes >= s.prevNet.rxBytes && nNow.txBytes >= s.prevNet.txBytes {
				inMbps := float64(nNow.rxBytes-s.prevNet.rxBytes) * 8.0 / secs / 1_000_000.0
				outMbps := float64(nNow.txBytes-s.prevNet.txBytes) * 8.0 / secs / 1_000_000.0
				s.inMbps = math.Round(inMbps*100) / 100
				s.outMbps = math.Round(outMbps*100) / 100
				s.inHist = appendRing(s.inHist, s.inMbps, 16)
				s.outHist = appendRing(s.outHist, s.outMbps, 16)
			}
		}
		np := nNow
		s.prevNet = &np
	}
}

func (s *SystemSampler) Snapshot() (cpu float64, load1 string, cores int, ramPct float64, ramDetail string, diskPct float64, diskDetail string, netStatus NetworkStatus, uptimeStr string, hostName string) {
	s.mu.RLock()
	cpu = s.cpuPct
	inMbps := s.inMbps
	outMbps := s.outMbps
	iface := s.activeIface
	inHist := append([]float64(nil), s.inHist...)
	outHist := append([]float64(nil), s.outHist...)
	s.mu.RUnlock()

	if iface == "" {
		iface = "eth0"
	}
	if len(inHist) == 0 {
		inHist = []float64{0, 0}
		outHist = []float64{0, 0}
	}

	cores = runtime.NumCPU()
	load1 = readLoad1(s.procDir)
	ramPct, ramDetail = readMemInfo(s.procDir)
	diskPct, diskDetail = readDiskInfo(s.rootfsPath)
	uptimeStr = readUptime(s.procDir)
	hostName = readHostName(s.procDir)

	netStatus = NetworkStatus{
		Interface:    iface,
		InboundStr:   formatRate(inMbps),
		OutboundStr:  formatRate(outMbps),
		InboundMbps:  inMbps,
		OutboundMbps: outMbps,
		InHistory:    inHist,
		OutHistory:   outHist,
	}
	return
}

func readLoad1(procDir string) string {
	b, err := os.ReadFile(filepath.Join(procDir, "loadavg"))
	if err != nil {
		return "0.00"
	}
	fields := strings.Fields(string(b))
	if len(fields) > 0 {
		return fields[0]
	}
	return "0.00"
}

func readMemInfo(procDir string) (float64, string) {
	f, err := os.Open(filepath.Join(procDir, "meminfo"))
	if err != nil {
		return 0, "n/a"
	}
	defer f.Close()
	var totalKB, availKB uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				totalKB, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		} else if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				availKB, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		}
	}
	if totalKB == 0 {
		return 0, "n/a"
	}
	usedKB := totalKB - availKB
	pct := math.Round((float64(usedKB)/float64(totalKB))*1000) / 10
	usedGB := float64(usedKB) / 1048576.0
	totalGB := float64(totalKB) / 1048576.0
	return pct, fmt.Sprintf("%.1f / %.1f GB belegt", usedGB, totalGB)
}

func readDiskInfo(path string) (float64, string) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, "n/a"
	}
	total := float64(st.Blocks) * float64(st.Bsize)
	avail := float64(st.Bavail) * float64(st.Bsize)
	if total <= 0 {
		return 0, "n/a"
	}
	used := total - avail
	pct := math.Round((used/total)*1000) / 10
	usedGB := used / 1_073_741_824.0
	totalGB := total / 1_073_741_824.0
	if totalGB >= 1000 {
		return pct, fmt.Sprintf("%.0f GB / %.2f TB", usedGB, totalGB/1024.0)
	}
	return pct, fmt.Sprintf("%.0f / %.0f GB", usedGB, totalGB)
}

func readUptime(procDir string) string {
	b, err := os.ReadFile(filepath.Join(procDir, "uptime"))
	if err != nil {
		return "n/a"
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return "n/a"
	}
	secs, _ := strconv.ParseFloat(fields[0], 64)
	d := int(secs) / 86400
	h := (int(secs) % 86400) / 3600
	m := (int(secs) % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh", d, h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

func readHostName(procDir string) string {
	if b, err := os.ReadFile(filepath.Join(procDir, "sys/kernel/hostname")); err == nil {
		if h := strings.TrimSpace(string(b)); h != "" {
			return h
		}
	}
	h, _ := os.Hostname()
	return h
}

func formatRate(mbps float64) string {
	if mbps < 1.0 {
		return fmt.Sprintf("%.0f Kbit/s", mbps*1000.0)
	}
	if mbps >= 100.0 {
		return fmt.Sprintf("%.0f Mbit/s", mbps)
	}
	return fmt.Sprintf("%.1f Mbit/s", mbps)
}

// ─── DOCKER ENGINE SOCKET CLIENT ───

type dockerContainerSummary struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
}

type dockerContainerInspect struct {
	Name         string `json:"Name"`
	RestartCount int    `json:"RestartCount"`
	State        struct {
		Status    string `json:"Status"`
		Running   bool   `json:"Running"`
		OOMKilled bool   `json:"OOMKilled"`
		ExitCode  int    `json:"ExitCode"`
		Health    *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
}

type DockerSnapshot struct {
	Available       bool
	Hostname        string
	Healthy         int
	Unhealthy       int
	Total           int
	Restarts        int
	TopRestarts     []RestartEntry
	UnhealthyNames  []string
	OOMNames        []string
	RunningByName   map[string]bool
	UnhealthyByName map[string]bool
}

func collectDocker(socketPath string) DockerSnapshot {
	snap := DockerSnapshot{
		TopRestarts:     []RestartEntry{},
		RunningByName:   map[string]bool{},
		UnhealthyByName: map[string]bool{},
	}
	if _, err := os.Stat(socketPath); err != nil {
		return snap
	}

	client := &http.Client{
		Timeout: 4 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}

	// Host info
	if resp, err := client.Get("http://docker/info"); err == nil {
		var info struct {
			Name string `json:"Name"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&info)
		resp.Body.Close()
		snap.Hostname = info.Name
	}

	resp, err := client.Get("http://docker/containers/json?all=true")
	if err != nil {
		return snap
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return snap
	}

	var list []dockerContainerSummary
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return snap
	}
	snap.Available = true

	// Parallel inspect with worker pool of 8
	type res struct {
		sum dockerContainerSummary
		ins *dockerContainerInspect
	}
	results := make([]res, len(list))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, c := range list {
		wg.Add(1)
		go func(idx int, cs dockerContainerSummary) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var ins *dockerContainerInspect
			r, err := client.Get("http://docker/containers/" + cs.ID + "/json")
			if err == nil {
				if r.StatusCode == 200 {
					var d dockerContainerInspect
					if json.NewDecoder(r.Body).Decode(&d) == nil {
						ins = &d
					}
				}
				r.Body.Close()
			}
			results[idx] = res{sum: cs, ins: ins}
		}(i, c)
	}
	wg.Wait()

	var restartsList []RestartEntry
	for _, item := range results {
		name := "container"
		if len(item.sum.Names) > 0 {
			name = strings.TrimPrefix(item.sum.Names[0], "/")
		}
		lowerName := strings.ToLower(name)
		state := strings.ToLower(item.sum.State)
		rc := 0
		healthStatus := ""
		oom := false
		exitCode := 0

		if item.ins != nil {
			rc = item.ins.RestartCount
			oom = item.ins.State.OOMKilled
			exitCode = item.ins.State.ExitCode
			if item.ins.State.Health != nil {
				healthStatus = strings.ToLower(item.ins.State.Health.Status)
			}
		}

		if state == "running" {
			snap.RunningByName[lowerName] = true
			if healthStatus == "unhealthy" {
				snap.Unhealthy++
				snap.Total++
				snap.UnhealthyByName[lowerName] = true
				snap.UnhealthyNames = append(snap.UnhealthyNames, name)
			} else {
				snap.Healthy++
				snap.Total++
			}
		} else if state == "restarting" || (state == "exited" && exitCode != 0) || oom {
			snap.Unhealthy++
			snap.Total++
			snap.UnhealthyByName[lowerName] = true
			snap.UnhealthyNames = append(snap.UnhealthyNames, name)
		}

		if oom {
			snap.OOMNames = append(snap.OOMNames, name)
		}

		if rc > 0 {
			snap.Restarts += rc
			reason := fmt.Sprintf("Status: %s", item.sum.Status)
			if oom {
				reason = "OOMKilled · Speicherlimit überschritten"
			} else if healthStatus == "unhealthy" {
				reason = "Healthcheck fehlgeschlagen"
			}
			restartsList = append(restartsList, RestartEntry{
				Name:   name,
				Count:  rc,
				Reason: reason,
				State:  state,
			})
		}
	}

	sort.Slice(restartsList, func(i, j int) bool {
		if restartsList[i].Count != restartsList[j].Count {
			return restartsList[i].Count > restartsList[j].Count
		}
		return restartsList[i].Name < restartsList[j].Name
	})
	if len(restartsList) > 4 {
		restartsList = restartsList[:4]
	}
	snap.TopRestarts = restartsList
	return snap
}

// ─── UPTIME KUMA METRICS CLIENT (OPTIONAL) ───

type KumaMonitor struct {
	Name      string
	Status    int     // 1 = up, 0 = down
	LatencyMs float64 // -1 if unknown
	CertDays  float64 // -1 if unknown
	HasCert   bool
}

type KumaSnapshot struct {
	Available bool
	Total     int
	Online    int
	Monitors  map[string]KumaMonitor
	MinCert   float64
	MinDomain string
	HasCert   bool
}

func collectUptimeKuma(baseURL, apiKey, statusSlug string) KumaSnapshot {
	snap := KumaSnapshot{Monitors: map[string]KumaMonitor{}}
	if baseURL == "" {
		return snap
	}
	client := &http.Client{Timeout: 4 * time.Second}

	// Mode A: Prometheus /metrics endpoint (requires API key or public metrics)
	if apiKey != "" || statusSlug == "" {
		req, err := http.NewRequest("GET", baseURL+"/metrics", nil)
		if err == nil {
			if apiKey != "" {
				req.SetBasicAuth("", apiKey)
			}
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == 200 {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
				resp.Body.Close()
				parseKumaMetrics(string(body), &snap)
				if snap.Total > 0 {
					snap.Available = true
					return snap
				}
			} else if resp != nil {
				resp.Body.Close()
			}
		}
	}

	// Mode B: Public Status Page JSON API
	if statusSlug != "" {
		if collectKumaStatusPage(client, baseURL, statusSlug, &snap) {
			snap.Available = true
			return snap
		}
	}

	return snap
}

func parseKumaMetrics(text string, snap *KumaSnapshot) {
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "monitor_status{") {
			name := extractLabel(line, "monitor_name")
			val, ok := extractMetricVal(line)
			if name != "" && ok {
				m := snap.Monitors[name]
				m.Name = name
				m.Status = int(val)
				snap.Monitors[name] = m
			}
		} else if strings.HasPrefix(line, "monitor_response_time{") {
			name := extractLabel(line, "monitor_name")
			val, ok := extractMetricVal(line)
			if name != "" && ok {
				m := snap.Monitors[name]
				m.Name = name
				m.LatencyMs = val
				snap.Monitors[name] = m
			}
		} else if strings.HasPrefix(line, "monitor_cert_days_remaining{") {
			name := extractLabel(line, "monitor_name")
			val, ok := extractMetricVal(line)
			if name != "" && ok {
				m := snap.Monitors[name]
				m.Name = name
				m.CertDays = val
				m.HasCert = true
				snap.Monitors[name] = m
				if !snap.HasCert || val < snap.MinCert {
					snap.HasCert = true
					snap.MinCert = val
					snap.MinDomain = name
				}
			}
		}
	}
	for _, m := range snap.Monitors {
		snap.Total++
		if m.Status == 1 {
			snap.Online++
		}
	}
}

func collectKumaStatusPage(client *http.Client, baseURL, slug string, snap *KumaSnapshot) bool {
	r1, err := client.Get(baseURL + "/api/status-page/" + url.PathEscape(slug))
	if err != nil || r1.StatusCode != 200 {
		if r1 != nil {
			r1.Body.Close()
		}
		return false
	}
	var page struct {
		PublicGroupList []struct {
			MonitorList []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"monitorList"`
		} `json:"publicGroupList"`
	}
	_ = json.NewDecoder(r1.Body).Decode(&page)
	r1.Body.Close()

	r2, err := client.Get(baseURL + "/api/status-page/heartbeat/" + url.PathEscape(slug))
	if err != nil || r2.StatusCode != 200 {
		if r2 != nil {
			r2.Body.Close()
		}
		return false
	}
	var hb struct {
		HeartbeatList map[string][]struct {
			Status int     `json:"status"`
			Ping   float64 `json:"ping"`
		} `json:"heartbeatList"`
	}
	_ = json.NewDecoder(r2.Body).Decode(&hb)
	r2.Body.Close()

	for _, g := range page.PublicGroupList {
		for _, mon := range g.MonitorList {
			key := strconv.Itoa(mon.ID)
			beats := hb.HeartbeatList[key]
			status := 0
			ping := 0.0
			if len(beats) > 0 {
				last := beats[len(beats)-1]
				status = last.Status
				ping = last.Ping
			}
			snap.Monitors[mon.Name] = KumaMonitor{
				Name:      mon.Name,
				Status:    status,
				LatencyMs: ping,
			}
			snap.Total++
			if status == 1 {
				snap.Online++
			}
		}
	}
	return snap.Total > 0
}

func extractLabel(line, key string) string {
	needle := key + `="`
	idx := strings.Index(line, needle)
	if idx == -1 {
		return ""
	}
	rest := line[idx+len(needle):]
	end := strings.Index(rest, `"`)
	if end == -1 {
		return ""
	}
	return rest[:end]
}

func extractMetricVal(line string) (float64, bool) {
	idx := strings.LastIndex(line, "}")
	if idx == -1 {
		return 0, false
	}
	fields := strings.Fields(line[idx+1:])
	if len(fields) == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	return v, err == nil
}

// ─── PROMETHEUS OPTIONAL QUERY CLIENT ───

func queryPrometheusScalar(baseURL, promql string) (float64, bool) {
	if baseURL == "" {
		return 0, false
	}
	client := &http.Client{Timeout: 3 * time.Second}
	u := baseURL + "/api/v1/query?query=" + url.QueryEscape(promql)
	resp, err := client.Get(u)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		return 0, false
	}
	defer resp.Body.Close()
	var out struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Value []interface{} `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Status != "success" || len(out.Data.Result) == 0 {
		return 0, false
	}
	if len(out.Data.Result[0].Value) < 2 {
		return 0, false
	}
	s, ok := out.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

// ─── ACTIVE SERVICE / HOST / SSL PROBERS ───

func probeService(def ServiceDef, dockerSnap DockerSnapshot, kumaSnap KumaSnapshot) ServiceStatus {
	id := slugify(def.Name)
	st := ServiceStatus{
		ID:       id,
		Name:     def.Name,
		Category: def.Category,
		Target:   def.Target,
	}

	// If Uptime Kuma already monitors this exact service name, prefer its status
	if kumaSnap.Available {
		for kName, km := range kumaSnap.Monitors {
			if strings.EqualFold(kName, def.Name) {
				st.Online = km.Status == 1
				if st.Online {
					if km.LatencyMs > 0 {
						st.Latency = fmt.Sprintf("%.0f ms", km.LatencyMs)
					} else {
						st.Latency = "OK"
					}
				} else {
					st.Latency = "OFFLINE"
					st.Error = "Uptime Kuma meldet OFFLINE"
				}
				return st
			}
		}
	}

	target := def.Target
	if strings.HasPrefix(target, "docker://") {
		kw := strings.ToLower(strings.TrimPrefix(target, "docker://"))
		if !dockerSnap.Available {
			st.Online = false
			st.Latency = "NO SOCK"
			st.Error = "Docker Socket nicht verfügbar"
			return st
		}
		foundRunning := false
		foundUnhealthy := false
		for cname := range dockerSnap.RunningByName {
			if strings.Contains(cname, kw) {
				foundRunning = true
				if dockerSnap.UnhealthyByName[cname] {
					foundUnhealthy = true
				}
			}
		}
		st.Online = foundRunning && !foundUnhealthy
		if st.Online {
			st.Latency = "RUNNING"
		} else if foundUnhealthy {
			st.Latency = "UNHEALTHY"
			st.Error = "Container Healthcheck fehlgeschlagen"
		} else {
			st.Latency = "STOPPED"
			st.Error = "Container läuft nicht"
		}
		return st
	}

	if strings.HasPrefix(target, "tcp://") {
		addr := strings.TrimPrefix(target, "tcp://")
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
		dur := time.Since(start)
		if err != nil {
			st.Online = false
			st.Latency = "TIMEOUT"
			st.Error = err.Error()
			return st
		}
		conn.Close()
		st.Online = true
		st.Latency = fmt.Sprintf("%d ms", maxInt(1, int(dur.Milliseconds())))
		return st
	}

	// Default: HTTP / HTTPS check
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	start := time.Now()
	resp, err := client.Get(target)
	dur := time.Since(start)
	if err != nil {
		st.Online = false
		st.Latency = "TIMEOUT"
		st.Error = "Nicht erreichbar"
		return st
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		st.Online = false
		st.Latency = fmt.Sprintf("HTTP %d", resp.StatusCode)
		st.Error = fmt.Sprintf("HTTP %d Serverfehler", resp.StatusCode)
		return st
	}
	st.Online = true
	st.Latency = fmt.Sprintf("%d ms", maxInt(1, int(dur.Milliseconds())))
	return st
}

func probeHost(def HostDef) (bool, string) {
	if def.Target == "local" || def.Target == "" {
		return true, "Lokal aktiv"
	}
	if strings.HasPrefix(def.Target, "tcp://") {
		addr := strings.TrimPrefix(def.Target, "tcp://")
		conn, err := net.DialTimeout("tcp", addr, 2500*time.Millisecond)
		if err != nil {
			return false, "Timeout"
		}
		conn.Close()
		return true, "TCP OK"
	}
	client := &http.Client{
		Timeout:   2500 * time.Millisecond,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	resp, err := client.Get(def.Target)
	if err != nil {
		return false, "Nicht erreichbar"
	}
	resp.Body.Close()
	return resp.StatusCode < 500, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

type SSLCheckResult struct {
	Domain   string
	DaysLeft int
	Valid    bool
	ErrText  string
}

func checkSSLDomain(domain string) SSLCheckResult {
	addr := domain
	if !strings.Contains(addr, ":") {
		addr += ":443"
	}
	dialer := &net.Dialer{Timeout: 4 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: strings.Split(domain, ":")[0]})
	if err != nil {
		return SSLCheckResult{Domain: domain, Valid: false, ErrText: "TLS-Handshake fehlgeschlagen"}
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return SSLCheckResult{Domain: domain, Valid: false, ErrText: "Kein Zertifikat"}
	}
	days := int(time.Until(certs[0].NotAfter).Hours() / 24.0)
	return SSLCheckResult{Domain: domain, DaysLeft: days, Valid: days >= 0}
}

// ─── MAIN LIVE COLLECTOR ───

type LiveCollector struct {
	cfg     Config
	sampler *SystemSampler
	mu      sync.RWMutex
	latest  DashboardPayload
}

func NewLiveCollector(cfg Config) *LiveCollector {
	return &LiveCollector{
		cfg:     cfg,
		sampler: NewSystemSampler(cfg.ProcDir, cfg.RootFSPath, cfg.NetInterface),
	}
}

func (lc *LiveCollector) Start(ctx context.Context) {
	lc.sampler.Start(ctx)
	ticker := time.NewTicker(lc.cfg.PollInterval)
	go func() {
		defer ticker.Stop()
		time.Sleep(400 * time.Millisecond)
		lc.refresh()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				lc.refresh()
			}
		}
	}()
}

func (lc *LiveCollector) Latest() DashboardPayload {
	lc.mu.RLock()
	p := lc.latest
	lc.mu.RUnlock()
	if p.Mode == "" {
		lc.refresh()
		lc.mu.RLock()
		p = lc.latest
		lc.mu.RUnlock()
	}
	return p
}

func (lc *LiveCollector) refresh() {
	var sources []string
	var incidents []IncidentItem

	// 1. Docker Engine live metrics
	dockerSnap := collectDocker(lc.cfg.DockerSocket)
	if dockerSnap.Available {
		sources = append(sources, "Docker Socket")
	}

	// 2. Host system metrics (/proc)
	cpu, load1, cores, ramPct, ramDetail, diskPct, diskDetail, netSt, uptimeStr, hostName := lc.sampler.Snapshot()
	if dockerSnap.Hostname != "" {
		hostName = dockerSnap.Hostname
	}
	sources = append(sources, "Host /proc")

	// 3. Optional Uptime Kuma & Prometheus
	kumaSnap := collectUptimeKuma(lc.cfg.UptimeKumaURL, lc.cfg.UptimeKumaKey, lc.cfg.UptimeKumaSlug)
	if kumaSnap.Available {
		sources = append(sources, "Uptime Kuma")
	}
	promActive := false
	promHostsUp, okUp := queryPrometheusScalar(lc.cfg.PrometheusURL, "count(up == 1)")
	promHostsAll, okAll := queryPrometheusScalar(lc.cfg.PrometheusURL, "count(up)")
	if okUp && okAll && promHostsAll > 0 {
		promActive = true
		sources = append(sources, "Prometheus")
	}

	// 4. Probe Critical Services in parallel
	critResults := make([]ServiceStatus, len(lc.cfg.CriticalServices))
	var wg sync.WaitGroup
	for i, sdef := range lc.cfg.CriticalServices {
		wg.Add(1)
		go func(idx int, sd ServiceDef) {
			defer wg.Done()
			critResults[idx] = probeService(sd, dockerSnap, kumaSnap)
		}(i, sdef)
	}

	// Probe Extra Services
	extraResults := make([]ServiceStatus, len(lc.cfg.ExtraServices))
	for i, sdef := range lc.cfg.ExtraServices {
		wg.Add(1)
		go func(idx int, sd ServiceDef) {
			defer wg.Done()
			extraResults[idx] = probeService(sd, dockerSnap, kumaSnap)
		}(i, sdef)
	}

	// Probe Hosts
	type hostRes struct {
		name   string
		online bool
		detail string
	}
	hostResults := make([]hostRes, len(lc.cfg.Hosts))
	for i, hdef := range lc.cfg.Hosts {
		wg.Add(1)
		go func(idx int, hd HostDef) {
			defer wg.Done()
			ok, det := probeHost(hd)
			hostResults[idx] = hostRes{name: hd.Name, online: ok, detail: det}
		}(i, hdef)
	}

	// Check SSL Domains
	sslResults := make([]SSLCheckResult, len(lc.cfg.SSLDomains))
	for i, dom := range lc.cfg.SSLDomains {
		wg.Add(1)
		go func(idx int, d string) {
			defer wg.Done()
			sslResults[idx] = checkSSLDomain(d)
		}(i, dom)
	}
	wg.Wait()

	if len(lc.cfg.SSLDomains) > 0 {
		sources = append(sources, "Live TLS")
	}

	// ─── COMPUTE EXECUTIVE SUMMARY (ZEILE 1) ───
	// Panel 1: Dienste online
	svcOnline := 0
	svcTotal := 0
	svcSource := "Live-Probe"
	if kumaSnap.Available {
		svcOnline = kumaSnap.Online
		svcTotal = kumaSnap.Total
		svcSource = "Uptime Kuma"
	} else {
		for _, s := range critResults {
			svcTotal++
			if s.Online {
				svcOnline++
			}
		}
		for _, s := range extraResults {
			svcTotal++
			if s.Online {
				svcOnline++
			}
		}
	}
	svcStatus := "ok"
	svcSub := "100 % Verfügbarkeit · Live geprüft"
	if svcTotal == 0 {
		svcStatus = "warn"
		svcSub = "Keine Dienste konfiguriert"
	} else if svcOnline < svcTotal {
		svcStatus = "crit"
		svcSub = fmt.Sprintf("%d Dienst(e) OFFLINE", svcTotal-svcOnline)
	}

	// Panel 2: Hosts online
	hostsOnline := 0
	hostsTotal := 0
	hostSource := "Host-Check"
	if promActive {
		hostsOnline = int(promHostsUp)
		hostsTotal = int(promHostsAll)
		hostSource = "Prometheus"
	} else {
		for _, h := range hostResults {
			hostsTotal++
			if h.online {
				hostsOnline++
			} else {
				incidents = append(incidents, IncidentItem{
					Level: "crit",
					Badge: "KRITISCH",
					Title: fmt.Sprintf("Host offline · %s", h.name),
					Meta:  fmt.Sprintf("Host-Check · %s", h.detail),
					Time:  "AKTIV",
					Rank:  1,
				})
			}
		}
	}
	hostStatus := "ok"
	hostSub := fmt.Sprintf("%s · Uptime %s", hostName, uptimeStr)
	if hostsOnline < hostsTotal {
		hostStatus = "crit"
		hostSub = fmt.Sprintf("%d Host(s) nicht erreichbar", hostsTotal-hostsOnline)
	}

	// Panel 3: Container healthy
	cntValue := "n/a"
	cntStatus := "warn"
	cntSub := "Docker Socket (/var/run/docker.sock) nicht gemountet"
	if dockerSnap.Available {
		cntValue = fmt.Sprintf("%d / %d", dockerSnap.Healthy, dockerSnap.Total)
		if dockerSnap.Unhealthy > 0 {
			cntStatus = "crit"
			cntSub = fmt.Sprintf("%d Container UNHEALTHY / gestoppt", dockerSnap.Unhealthy)
		} else {
			cntStatus = "ok"
			cntSub = fmt.Sprintf("%d laufende Container · 0 unhealthy", dockerSnap.Healthy)
		}
	}

	// Panel 4: SSL-Zertifikate
	sslValue := "OK"
	sslStatus := "ok"
	sslSource := "Live TLS"
	sslSub := "Keine SSL-Domains konfiguriert"
	minSSL := 9999
	minSSLDom := ""
	sslFailed := ""

	if kumaSnap.Available && kumaSnap.HasCert {
		sslSource = "Uptime Kuma"
		minSSL = int(kumaSnap.MinCert)
		minSSLDom = kumaSnap.MinDomain
	}
	for _, r := range sslResults {
		if !r.Valid {
			sslFailed = r.Domain
		} else if r.DaysLeft < minSSL {
			minSSL = r.DaysLeft
			minSSLDom = r.Domain
		}
	}

	if sslFailed != "" {
		sslValue = "FEHLER"
		sslStatus = "crit"
		sslSub = fmt.Sprintf("TLS-Fehler bei %s", sslFailed)
		incidents = append(incidents, IncidentItem{
			Level: "crit",
			Badge: "KRITISCH",
			Title: fmt.Sprintf("SSL/TLS-Prüfung fehlgeschlagen · %s", sslFailed),
			Meta:  "Live TLS · Zertifikat ungültig oder Port 443 nicht erreichbar",
			Time:  "AKTIV",
			Rank:  2,
		})
	} else if minSSL < 9999 {
		sslSub = fmt.Sprintf("Min. %d Tage · %s", minSSL, minSSLDom)
		if minSSL < 7 {
			sslValue = fmt.Sprintf("%d TAGE", minSSL)
			sslStatus = "crit"
			incidents = append(incidents, IncidentItem{
				Level: "crit",
				Badge: "KRITISCH",
				Title: fmt.Sprintf("SSL läuft in %d Tagen ab · %s", minSSL, minSSLDom),
				Meta:  sslSource + " · Sofortige Erneuerung erforderlich",
				Time:  fmt.Sprintf("%dd Rest", minSSL),
				Rank:  2,
			})
		} else if minSSL <= 21 {
			sslValue = fmt.Sprintf("%d TAGE", minSSL)
			sslStatus = "warn"
			incidents = append(incidents, IncidentItem{
				Level: "warn",
				Badge: "WARNUNG",
				Title: fmt.Sprintf("SSL läuft in %d Tagen ab · %s", minSSL, minSSLDom),
				Meta:  sslSource + " · Let's Encrypt Auto-Renewal prüfen",
				Time:  fmt.Sprintf("%dd Rest", minSSL),
				Rank:  5,
			})
		} else {
			sslValue = "OK"
			sslStatus = "ok"
		}
	}

	// ─── INCIDENTS FROM CRITICAL SERVICES ───
	critOffCount := 0
	for _, s := range critResults {
		if !s.Online {
			critOffCount++
			incidents = append(incidents, IncidentItem{
				Level: "crit",
				Badge: "KRITISCH",
				Title: fmt.Sprintf("%s OFFLINE (%s)", s.Name, s.Latency),
				Meta:  fmt.Sprintf("Ziel: %s · %s", s.Target, s.Error),
				Time:  "AKTIV",
				Rank:  1,
			})
		}
	}

	// ─── INFRASTRUCTURE GAUGES (ZEILE 3) ───
	cpuSt, cpuTag := thresholdState(cpu, 70, 85)
	ramSt, ramTag := thresholdState(ramPct, 75, 90)
	diskSt, diskTag := thresholdState(diskPct, 80, 90)

	if cpuSt == "crit" {
		incidents = append(incidents, IncidentItem{Level: "crit", Badge: "KRITISCH", Title: fmt.Sprintf("CPU-Last kritisch (%.0f %%)", cpu), Meta: fmt.Sprintf("Host %s · Load %s · Schwelle > 85 %%", hostName, load1), Time: "AKTIV", Rank: 2})
	} else if cpuSt == "warn" {
		incidents = append(incidents, IncidentItem{Level: "warn", Badge: "WARNUNG", Title: fmt.Sprintf("CPU-Last erhöht (%.0f %%)", cpu), Meta: fmt.Sprintf("Host %s · Load %s · Bereich 70–85 %%", hostName, load1), Time: "AKTIV", Rank: 4})
	}
	if ramSt == "crit" {
		incidents = append(incidents, IncidentItem{Level: "crit", Badge: "KRITISCH", Title: fmt.Sprintf("Arbeitsspeicher kritisch (%.0f %%)", ramPct), Meta: ramDetail + " · Schwelle > 90 %", Time: "AKTIV", Rank: 2})
	} else if ramSt == "warn" {
		incidents = append(incidents, IncidentItem{Level: "warn", Badge: "WARNUNG", Title: fmt.Sprintf("Arbeitsspeicher erhöht (%.0f %%)", ramPct), Meta: ramDetail + " · Bereich 75–90 %", Time: "AKTIV", Rank: 4})
	}
	if diskSt == "crit" {
		incidents = append(incidents, IncidentItem{Level: "crit", Badge: "KRITISCH", Title: fmt.Sprintf("Datenträger kritisch voll (%.0f %%)", diskPct), Meta: diskDetail + " · Schwelle > 90 %", Time: "AKTIV", Rank: 2})
	} else if diskSt == "warn" {
		incidents = append(incidents, IncidentItem{Level: "warn", Badge: "WARNUNG", Title: fmt.Sprintf("Datenträger über Warnschwelle (%.0f %%)", diskPct), Meta: diskDetail + " · Bereich 80–90 %", Time: "AKTIV", Rank: 4})
	}

	// ─── CONTAINER HEALTH INCIDENTS (ZEILE 4) ───
	for _, uname := range dockerSnap.UnhealthyNames {
		incidents = append(incidents, IncidentItem{
			Level: "crit",
			Badge: "KRITISCH",
			Title: fmt.Sprintf("Container unhealthy: %s", uname),
			Meta:  "Docker Engine · Healthcheck fehlgeschlagen oder abgestürzt",
			Time:  "AKTIV",
			Rank:  2,
		})
	}
	for _, r := range dockerSnap.TopRestarts {
		incidents = append(incidents, IncidentItem{
			Level: "warn",
			Badge: "WARNUNG",
			Title: fmt.Sprintf("%s restart x%d", r.Name, r.Count),
			Meta:  fmt.Sprintf("Docker Engine · %s", r.Reason),
			Time:  "Container",
			Rank:  5,
		})
	}

	// Calm operational info items when nominal (fill up to max 4 items)
	if len(incidents) < 4 {
		if minSSL < 9999 && minSSL > 21 {
			incidents = append(incidents, IncidentItem{
				Level: "info",
				Badge: "INFO",
				Title: fmt.Sprintf("SSL-Zertifikate gültig (noch %d Tage)", minSSL),
				Meta:  fmt.Sprintf("%s · Kürzeste Laufzeit: %s", sslSource, minSSLDom),
				Time:  "OK",
				Rank:  8,
			})
		}
		if dockerSnap.Available {
			incidents = append(incidents, IncidentItem{
				Level: "info",
				Badge: "INFO",
				Title: fmt.Sprintf("Docker Engine stabil · %d / %d Container healthy", dockerSnap.Healthy, dockerSnap.Total),
				Meta:  fmt.Sprintf("Host %s · %d Neustarts registriert", hostName, dockerSnap.Restarts),
				Time:  "LIVE",
				Rank:  9,
			})
		}
		incidents = append(incidents, IncidentItem{
			Level: "info",
			Badge: "INFO",
			Title: fmt.Sprintf("Host %s online · Uptime %s", hostName, uptimeStr),
			Meta:  fmt.Sprintf("Aktive Quellen: %s", strings.Join(sources, " · ")),
			Time:  "SYNC",
			Rank:  10,
		})
	}

	sort.SliceStable(incidents, func(i, j int) bool {
		return incidents[i].Rank < incidents[j].Rank
	})
	if len(incidents) > 5 {
		incidents = incidents[:5]
	}

	// ─── GLOBAL EXECUTIVE VERDICT ───
	verdict := VerdictInfo{
		Level: "ok",
		Text:  "ALLE SYSTEME NOMINAL · KEIN EINGRIFF ERFORDERLICH",
	}
	hasCrit := critOffCount > 0 || svcStatus == "crit" || hostStatus == "crit" || cntStatus == "crit" || sslStatus == "crit" || cpuSt == "crit" || ramSt == "crit" || diskSt == "crit"
	hasWarn := svcStatus == "warn" || hostStatus == "warn" || sslStatus == "warn" || cpuSt == "warn" || ramSt == "warn" || diskSt == "warn" || dockerSnap.Restarts > 0

	if hasCrit {
		verdict = VerdictInfo{
			Level: "crit",
			Text:  "EINGRIFF ERFORDERLICH · KRITISCHE STÖRUNG ERKANNT",
		}
		if critOffCount > 0 {
			verdict.Text = fmt.Sprintf("EINGRIFF ERFORDERLICH · %d KRITISCHER DIENST OFFLINE", critOffCount)
		} else if dockerSnap.Unhealthy > 0 {
			verdict.Text = fmt.Sprintf("EINGRIFF ERFORDERLICH · %d CONTAINER UNHEALTHY", dockerSnap.Unhealthy)
		}
	} else if hasWarn {
		verdict = VerdictInfo{
			Level: "warn",
			Text:  "WARNUNG AKTIV · RESSOURCEN / RESTARTS / SSL PRÜFEN",
		}
	}

	payload := DashboardPayload{
		Mode:            "live",
		Timestamp:       time.Now().Format(time.RFC3339),
		PollIntervalSec: int(lc.cfg.PollInterval.Seconds()),
		Hostname:        hostName,
		UptimeHuman:     uptimeStr,
		Sources:         sources,
		Verdict:         verdict,
		ExecutiveSummary: []ExecStat{
			{ID: "services-online", Label: "Dienste online", Value: fmt.Sprintf("%d / %d", svcOnline, svcTotal), Source: svcSource, Status: svcStatus, Sub: svcSub},
			{ID: "hosts-online", Label: "Hosts online", Value: fmt.Sprintf("%d / %d", hostsOnline, hostsTotal), Source: hostSource, Status: hostStatus, Sub: hostSub},
			{ID: "containers-healthy", Label: "Container healthy", Value: cntValue, Source: "Docker Engine", Status: cntStatus, Sub: cntSub},
			{ID: "ssl-status", Label: "SSL-Zertifikate", Value: sslValue, Source: sslSource, Status: sslStatus, Sub: sslSub},
		},
		CriticalServices: critResults,
		Infrastructure: InfraStatus{
			Source: fmt.Sprintf("Live /proc · Host %s", hostName),
			Gauges: []GaugeStatus{
				{ID: "cpu", Label: "CPU-Last", Value: cpu, Unit: "%", Status: cpuSt, StateTag: cpuTag, Detail: fmt.Sprintf("%d vCPU · Load %s", cores, load1), Threshold: "Grün < 70 % · Gelb 70–85 % · Rot > 85 %"},
				{ID: "ram", Label: "Arbeitsspeicher", Value: ramPct, Unit: "%", Status: ramSt, StateTag: ramTag, Detail: ramDetail, Threshold: "Grün < 75 % · Gelb 75–90 % · Rot > 90 %"},
				{ID: "disk", Label: "Datenträger", Value: diskPct, Unit: "%", Status: diskSt, StateTag: diskTag, Detail: diskDetail, Threshold: "Grün < 80 % · Gelb 80–90 % · Rot > 90 %"},
			},
			Network: netSt,
		},
		ContainerHealth: ContainerHealth{
			Available:   dockerSnap.Available,
			Source:      "Docker Engine Socket (Live)",
			Healthy:     dockerSnap.Healthy,
			Unhealthy:   dockerSnap.Unhealthy,
			Total:       dockerSnap.Total,
			Restarts:    dockerSnap.Restarts,
			TopRestarts: dockerSnap.TopRestarts,
		},
		Incidents: incidents,
	}

	lc.mu.Lock()
	lc.latest = payload
	lc.mu.Unlock()
}

func thresholdState(val, warn, crit float64) (string, string) {
	if val > crit {
		return "crit", fmt.Sprintf("KRITISCH (>%.0f%%)", crit)
	}
	if val >= warn {
		return "warn", fmt.Sprintf("WARNUNG (%.0f–%.0f%%)", warn, crit)
	}
	return "ok", "NOMINAL"
}

func appendRing(s []float64, v float64, maxLen int) []float64 {
	s = append(s, v)
	if len(s) > maxLen {
		s = s[len(s)-maxLen:]
	}
	return s
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ─── HTTP SERVER ───

func main() {
	cfg := loadConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector := NewLiveCollector(cfg)
	collector.Start(ctx)

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		payload := collector.Latest()
		enc := json.NewEncoder(w)
		_ = enc.Encode(payload)
	})

	fileServer := http.FileServer(http.Dir(cfg.PublicDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fonts/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if strings.HasPrefix(r.URL.Path, "/css/") || strings.HasPrefix(r.URL.Path, "/js/") {
			w.Header().Set("Cache-Control", "public, max-age=300")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("NOC Live Server listening on %s (public=%s, proc=%s, docker=%s, poll=%s)",
		cfg.ListenAddr, cfg.PublicDir, cfg.ProcDir, cfg.DockerSocket, cfg.PollInterval)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
