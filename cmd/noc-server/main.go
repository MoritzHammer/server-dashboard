package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
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

//go:embed all:web
var embeddedWeb embed.FS

// ─── API RESPONSE TYPES ───

type DashboardPayload struct {
	Mode             string                `json:"mode"`
	Timestamp        string                `json:"timestamp"`
	PollIntervalSec  int                   `json:"poll_interval_sec"`
	Hostname         string                `json:"hostname"`
	HostIP           string                `json:"host_ip"`
	UptimeHuman      string                `json:"uptime_human"`
	Sources          []string              `json:"sources"`
	Verdict          VerdictInfo           `json:"verdict"`
	ExecutiveSummary []ExecStat            `json:"executive_summary"`
	CriticalServices []ServiceStatus       `json:"critical_services"`
	Infrastructure   InfraStatus           `json:"infrastructure"`
	ContainerHealth  ContainerHealth       `json:"container_health"`
	Containers       []DockerContainerInfo `json:"containers"`
	SSLCerts         []SSLCheckResult      `json:"ssl_certs"`
	WUD              WUDSnapshot           `json:"wud"`
	Beszel           BeszelSnapshot        `json:"beszel"`
	Kuma             KumaPublicSnapshot    `json:"kuma"`
	Embeds           EmbedConfig           `json:"embeds"`
	Incidents        []IncidentItem        `json:"incidents"`
}

type VerdictInfo struct {
	Level string `json:"level"` // "ok", "warn", "crit"
	Text  string `json:"text"`
}

type ExecStat struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Value   string   `json:"value"`
	Source  string   `json:"source"`
	Status  string   `json:"status"` // "ok", "warn", "crit"
	Sub     string   `json:"sub"`
	Details []string `json:"details"`
}

type ServiceStatus struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Category        string `json:"category"`
	Target          string `json:"target"`
	Online          bool   `json:"online"`
	Latency         string `json:"latency"`
	HTTPCode        int    `json:"http_code,omitempty"`
	ContainerStatus string `json:"container_status,omitempty"`
	Error           string `json:"error,omitempty"`
}

type GaugeStatus struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Value     float64  `json:"value"`
	Unit      string   `json:"unit"`
	Status    string   `json:"status"` // "ok", "warn", "crit"
	StateTag  string   `json:"state_tag"`
	Detail    string   `json:"detail"`
	Threshold string   `json:"threshold"`
	Extra     []string `json:"extra"`
}

type NetworkStatus struct {
	Interface    string    `json:"interface"`
	InboundStr   string    `json:"inbound_str"`
	OutboundStr  string    `json:"outbound_str"`
	InboundMbps  float64   `json:"inbound_mbps"`
	OutboundMbps float64   `json:"outbound_mbps"`
	RxTotalGB    float64   `json:"rx_total_gb"`
	TxTotalGB    float64   `json:"tx_total_gb"`
	InHistory    []float64 `json:"in_history"`
	OutHistory   []float64 `json:"out_history"`
}

type InfraStatus struct {
	Source     string        `json:"source"`
	Gauges     []GaugeStatus `json:"gauges"`
	Network    NetworkStatus `json:"network"`
	CPUHistory []float64     `json:"cpu_history"`
	RAMHistory []float64     `json:"ram_history"`
}

type RestartEntry struct {
	Name   string `json:"name"`
	Count  int    `json:"count"`
	Reason string `json:"reason"`
	State  string `json:"state"`
}

type DockerContainerInfo struct {
	Name       string   `json:"name"`
	Stack      string   `json:"stack"`
	Image      string   `json:"image"`
	State      string   `json:"state"`
	StatusText string   `json:"status_text"`
	Health     string   `json:"health"` // "healthy", "unhealthy", "running", "stopped"
	Restarts   int      `json:"restarts"`
	OOMKilled  bool     `json:"oom_killed"`
	Ports      []string `json:"ports"`
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

type WUDUpdateItem struct {
	Container   string `json:"container"`
	Image       string `json:"image"`
	CurrentTag  string `json:"current_tag"`
	NewTag      string `json:"new_tag"`
	SemverDiff  string `json:"semver_diff"`
	CautionDB   bool   `json:"caution_db"`
	ReleaseLink string `json:"release_link,omitempty"`
}

type WUDSnapshot struct {
	Available        bool            `json:"available"`
	URL              string          `json:"url"`
	TotalWatched     int             `json:"total_watched"`
	UpdatesAvailable int             `json:"updates_available"`
	Updates          []WUDUpdateItem `json:"updates"`
	Error            string          `json:"error,omitempty"`
}

type BeszelHostItem struct {
	Name   string  `json:"name"`
	Host   string  `json:"host"`
	Status string  `json:"status"`
	CPU    float64 `json:"cpu"`
	Mem    float64 `json:"mem"`
	Disk   float64 `json:"disk"`
	Temp   float64 `json:"temp"`
}

type BeszelSnapshot struct {
	Available bool             `json:"available"`
	Reachable bool             `json:"reachable"`
	URL       string           `json:"url"`
	Online    int              `json:"online"`
	Total     int              `json:"total"`
	Hosts     []BeszelHostItem `json:"hosts"`
}

type KumaPublicMonitor struct {
	Name      string  `json:"name"`
	Online    bool    `json:"online"`
	LatencyMs float64 `json:"latency_ms"`
	CertDays  int     `json:"cert_days,omitempty"`
}

type KumaPublicSnapshot struct {
	Available bool                `json:"available"`
	Reachable bool                `json:"reachable"`
	Online    int                 `json:"online"`
	Total     int                 `json:"total"`
	Monitors  []KumaPublicMonitor `json:"monitors"`
}

type EmbedConfig struct {
	BeszelURL      string `json:"beszel_url"`
	BeszelFrameURL string `json:"beszel_frame_url"`
	BeszelOnline   bool   `json:"beszel_online"`
	KumaURL        string `json:"kuma_url"`
	KumaFrameURL   string `json:"kuma_frame_url"`
	KumaOnline     bool   `json:"kuma_online"`
	ProxyEnabled   bool   `json:"proxy_enabled"`
	WUDURL         string `json:"wud_url"`
	WUDOnline      bool   `json:"wud_online"`
	DozzleURL      string `json:"dozzle_url"`
}

type IncidentItem struct {
	Level  string `json:"level"` // "crit", "warn", "info"
	Badge  string `json:"badge"`
	Title  string `json:"title"`
	Meta   string `json:"meta"`
	Detail string `json:"detail"`
	Time   string `json:"time"`
	Rank   int    `json:"-"`
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
	HostIP           string
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
	UptimeKumaEmbed  string
	UptimeKumaKey    string
	UptimeKumaSlug   string
	BeszelURL        string
	BeszelEmbed      string
	BeszelEmail      string
	BeszelPassword   string
	EnableEmbedProxy bool
	WUDURL           string
	WUDUser          string
	WUDPass          string
	DozzleURL        string
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
	hostIP := envOr("HOST_IP", "192.168.178.90")
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
		fmt.Sprintf("Vaultwarden|Passwort-Manager|http://%s:8081", hostIP),
		fmt.Sprintf("Immich|Fotoverwaltung|http://%s:2283", hostIP),
		fmt.Sprintf("Paperless-ngx|Dokumente|http://%s:8000", hostIP),
		fmt.Sprintf("Nginx Proxy Manager|Reverse Proxy & SSL|http://%s:81", hostIP),
		fmt.Sprintf("Uptime Kuma|Monitoring|http://%s:3001", hostIP),
		fmt.Sprintf("Website|info.moritzhammer.org|http://%s:8080", hostIP),
	}, ",")

	sslRaw := envOr("SSL_DOMAINS", "info.moritzhammer.org")
	var sslDomains []string
	if !strings.EqualFold(sslRaw, "none") {
		sslDomains = parseCSV(sslRaw)
	}

	kumaURL := strings.TrimRight(envOr("UPTIME_KUMA_URL", fmt.Sprintf("http://%s:3001", hostIP)), "/")
	kumaSlug := envOr("UPTIME_KUMA_STATUS_SLUG", "overview")
	kumaEmbed := strings.TrimRight(envOr("UPTIME_KUMA_EMBED_URL", ""), "/")
	if kumaEmbed == "" {
		if kumaSlug != "" {
			kumaEmbed = kumaURL + "/status/" + kumaSlug
		} else {
			kumaEmbed = kumaURL
		}
	}

	beszelURL := strings.TrimRight(envOr("BESZEL_URL", fmt.Sprintf("http://%s:8099", hostIP)), "/")
	beszelEmbed := strings.TrimRight(envOr("BESZEL_EMBED_URL", beszelURL), "/")
	proxyRaw := strings.ToLower(envOr("ENABLE_EMBED_PROXY", "true"))
	enableProxy := proxyRaw != "false" && proxyRaw != "0" && proxyRaw != "no"
	wudURL := strings.TrimRight(envOr("WUD_URL", ""), "/")
	dozzleURL := strings.TrimRight(envOr("DOZZLE_URL", fmt.Sprintf("http://%s:8888", hostIP)), "/")

	return Config{
		ListenAddr:       envOr("LISTEN_ADDR", ":80"),
		HostIP:           hostIP,
		PollInterval:     time.Duration(pollSec) * time.Second,
		DockerSocket:     envOr("DOCKER_SOCKET", "/var/run/docker.sock"),
		ProcDir:          procDir,
		RootFSPath:       rootfs,
		NetInterface:     envOr("NET_INTERFACE", ""),
		CriticalServices: parseServices(envOr("CRITICAL_SERVICES", defaultCrit)),
		ExtraServices:    parseServices(envOr("EXTRA_SERVICES", "")),
		Hosts:            parseHosts(envOr("HOSTS", "Docker-Host|local")),
		SSLDomains:       sslDomains,
		UptimeKumaURL:    kumaURL,
		UptimeKumaEmbed:  kumaEmbed,
		UptimeKumaKey:    envOr("UPTIME_KUMA_API_KEY", ""),
		UptimeKumaSlug:   kumaSlug,
		BeszelURL:        beszelURL,
		BeszelEmbed:      beszelEmbed,
		BeszelEmail:      envOr("BESZEL_EMAIL", ""),
		BeszelPassword:   envOr("BESZEL_PASSWORD", ""),
		EnableEmbedProxy: enableProxy,
		WUDURL:           wudURL,
		WUDUser:          envOr("WUD_USER", ""),
		WUDPass:          envOr("WUD_PASS", ""),
		DozzleURL:        dozzleURL,
		PrometheusURL:    strings.TrimRight(envOr("PROMETHEUS_URL", ""), "/"),
	}
}

// ─── SYSTEM SAMPLER (/proc CPU, RAM, DISK, NETWORK + 60-POINT HISTORY) ───

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

type memDetails struct {
	pct      float64
	usedGB   float64
	totalGB  float64
	availGB  float64
	cachedGB float64
}

type diskDetails struct {
	pct     float64
	usedGB  float64
	totalGB float64
	freeGB  float64
	mount   string
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
	rxTotalGB   float64
	txTotalGB   float64
	activeIface string
	cpuHist     []float64
	ramHist     []float64
	inHist      []float64
	outHist     []float64
}

func NewSystemSampler(procDir, rootfs, netIface string) *SystemSampler {
	return &SystemSampler{
		procDir:    procDir,
		rootfsPath: rootfs,
		netIface:   netIface,
		cpuHist:    make([]float64, 0, 60),
		ramHist:    make([]float64, 0, 60),
		inHist:     make([]float64, 0, 60),
		outHist:    make([]float64, 0, 60),
	}
}

func (s *SystemSampler) Start(ctx context.Context) {
	s.sampleOnce()
	go func() {
		// Fast warmup sample so first /api/status already has live CPU & network deltas
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
			s.sampleOnce()
		}
		ticker := time.NewTicker(2 * time.Second)
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
				idle += vals[4]
			}
			return cpuSample{idle: idle, total: total}, nil
		}
	}
	return cpuSample{}, fmt.Errorf("cpu line not found")
}

type ifaceStat struct {
	name string
	rx   uint64
	tx   uint64
}

func isVirtualIface(iface string) bool {
	return iface == "lo" ||
		strings.HasPrefix(iface, "docker") ||
		strings.HasPrefix(iface, "br-") ||
		strings.HasPrefix(iface, "veth") ||
		strings.HasPrefix(iface, "virbr") ||
		strings.HasPrefix(iface, "vmbr") ||
		strings.HasPrefix(iface, "cni") ||
		strings.HasPrefix(iface, "flannel") ||
		strings.HasPrefix(iface, "cali") ||
		strings.HasPrefix(iface, "tun") ||
		strings.HasPrefix(iface, "tap") ||
		strings.HasPrefix(iface, "wg") ||
		strings.HasPrefix(iface, "tailscale") ||
		strings.HasPrefix(iface, "zt") ||
		strings.HasPrefix(iface, "dummy")
}

func parseProcNetDev(path string) ([]ifaceStat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var stats []ifaceStat
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		iface := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])
		if len(fields) < 10 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		stats = append(stats, ifaceStat{name: iface, rx: rx, tx: tx})
	}
	return stats, nil
}

func parseSysClassNet(baseDir string) ([]ifaceStat, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}
	var stats []ifaceStat
	for _, e := range entries {
		name := e.Name()
		rxB, err1 := os.ReadFile(filepath.Join(baseDir, name, "statistics/rx_bytes"))
		txB, err2 := os.ReadFile(filepath.Join(baseDir, name, "statistics/tx_bytes"))
		if err1 != nil || err2 != nil {
			continue
		}
		rx, _ := strconv.ParseUint(strings.TrimSpace(string(rxB)), 10, 64)
		tx, _ := strconv.ParseUint(strings.TrimSpace(string(txB)), 10, 64)
		stats = append(stats, ifaceStat{name: name, rx: rx, tx: tx})
	}
	return stats, nil
}

func (s *SystemSampler) pickNetSample(all []ifaceStat) (netSample, bool) {
	var selected []ifaceStat
	for _, st := range all {
		if s.netIface != "" {
			if st.name == s.netIface {
				selected = append(selected, st)
			}
			continue
		}
		if isVirtualIface(st.name) {
			continue
		}
		selected = append(selected, st)
	}
	// Fallback: if all non-lo interfaces were bridges (e.g. vmbr0-only host), include non-lo with traffic
	if len(selected) == 0 && s.netIface == "" {
		for _, st := range all {
			if st.name != "lo" && (st.rx > 0 || st.tx > 0) {
				selected = append(selected, st)
			}
		}
	}
	if len(selected) == 0 {
		return netSample{}, false
	}

	sort.Slice(selected, func(i, j int) bool {
		return (selected[i].rx + selected[i].tx) > (selected[j].rx + selected[j].tx)
	})

	var rxSum, txSum uint64
	var activeNames []string
	for _, st := range selected {
		rxSum += st.rx
		txSum += st.tx
		if st.rx > 0 || st.tx > 0 {
			activeNames = append(activeNames, st.name)
		}
	}
	if len(activeNames) == 0 {
		activeNames = append(activeNames, selected[0].name)
	}
	label := activeNames[0]
	if len(activeNames) > 1 {
		label = strings.Join(activeNames[:minInt(2, len(activeNames))], "+")
	}
	return netSample{rxBytes: rxSum, txBytes: txSum, at: time.Now(), iface: label}, true
}

func (s *SystemSampler) readNet() (netSample, error) {
	// 1. Prefer host PID 1's net namespace (/host/proc/1/net/dev):
	//    /host/proc/net is a symlink to self/net, which inside Docker only sees the container's bridge NIC!
	if s.procDir != "/proc" {
		if stats, err := parseProcNetDev(filepath.Join(s.procDir, "1/net/dev")); err == nil {
			if ns, ok := s.pickNetSample(stats); ok {
				return ns, nil
			}
		}
	}
	// 2. Secondary host source: /host/sys/class/net (if bind-mounted)
	if stats, err := parseSysClassNet("/host/sys/class/net"); err == nil {
		if ns, ok := s.pickNetSample(stats); ok {
			return ns, nil
		}
	}
	// 3. Local fallback (/proc/net/dev)
	stats, err := parseProcNetDev(filepath.Join(s.procDir, "net/dev"))
	if err != nil {
		return netSample{}, err
	}
	if ns, ok := s.pickNetSample(stats); ok {
		return ns, nil
	}
	return netSample{at: time.Now(), iface: "eth0"}, nil
}

func (s *SystemSampler) sampleOnce() {
	cNow, cErr := s.readCPU()
	nNow, nErr := s.readNet()
	mem := readMemInfo(s.procDir)

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
	s.cpuHist = appendRing(s.cpuHist, s.cpuPct, 60)
	s.ramHist = appendRing(s.ramHist, mem.pct, 60)

	if nErr == nil {
		s.activeIface = nNow.iface
		s.rxTotalGB = math.Round((float64(nNow.rxBytes)/1_073_741_824.0)*100) / 100
		s.txTotalGB = math.Round((float64(nNow.txBytes)/1_073_741_824.0)*100) / 100
		if s.prevNet != nil {
			secs := nNow.at.Sub(s.prevNet.at).Seconds()
			if secs > 0.2 && nNow.rxBytes >= s.prevNet.rxBytes && nNow.txBytes >= s.prevNet.txBytes {
				inMbps := float64(nNow.rxBytes-s.prevNet.rxBytes) * 8.0 / secs / 1_000_000.0
				outMbps := float64(nNow.txBytes-s.prevNet.txBytes) * 8.0 / secs / 1_000_000.0
				// Keep 0.1 Kbit/s resolution so low-traffic homelabs never snap to 0
				s.inMbps = math.Round(inMbps*10000) / 10000
				s.outMbps = math.Round(outMbps*10000) / 10000
				s.inHist = appendRing(s.inHist, s.inMbps, 60)
				s.outHist = appendRing(s.outHist, s.outMbps, 60)
			}
		}
		np := nNow
		s.prevNet = &np
	}
}

type systemSnapshot struct {
	cpuPct     float64
	load1      string
	load5      string
	load15     string
	cores      int
	mem        memDetails
	disk       diskDetails
	net        NetworkStatus
	cpuHistory []float64
	ramHistory []float64
	uptimeStr  string
	hostName   string
}

func (s *SystemSampler) Snapshot() systemSnapshot {
	s.mu.RLock()
	cpu := s.cpuPct
	inMbps := s.inMbps
	outMbps := s.outMbps
	rxGB := s.rxTotalGB
	txGB := s.txTotalGB
	iface := s.activeIface
	cpuHist := append([]float64(nil), s.cpuHist...)
	ramHist := append([]float64(nil), s.ramHist...)
	inHist := append([]float64(nil), s.inHist...)
	outHist := append([]float64(nil), s.outHist...)
	s.mu.RUnlock()

	if iface == "" {
		iface = "eth0"
	}
	if len(inHist) < 2 {
		inHist = []float64{0, 0}
		outHist = []float64{0, 0}
	}
	if len(cpuHist) < 2 {
		cpuHist = []float64{cpu, cpu}
	}
	mem := readMemInfo(s.procDir)
	if len(ramHist) < 2 {
		ramHist = []float64{mem.pct, mem.pct}
	}
	l1, l5, l15 := readLoadAvg(s.procDir)

	return systemSnapshot{
		cpuPct:     cpu,
		load1:      l1,
		load5:      l5,
		load15:     l15,
		cores:      runtime.NumCPU(),
		mem:        mem,
		disk:       readDiskInfo(s.rootfsPath),
		cpuHistory: cpuHist,
		ramHistory: ramHist,
		uptimeStr:  readUptime(s.procDir),
		hostName:   readHostName(s.procDir),
		net: NetworkStatus{
			Interface:    iface,
			InboundStr:   formatRate(inMbps),
			OutboundStr:  formatRate(outMbps),
			InboundMbps:  inMbps,
			OutboundMbps: outMbps,
			RxTotalGB:    rxGB,
			TxTotalGB:    txGB,
			InHistory:    inHist,
			OutHistory:   outHist,
		},
	}
}

func readLoadAvg(procDir string) (string, string, string) {
	b, err := os.ReadFile(filepath.Join(procDir, "loadavg"))
	if err != nil {
		return "0.00", "0.00", "0.00"
	}
	f := strings.Fields(string(b))
	if len(f) >= 3 {
		return f[0], f[1], f[2]
	}
	return "0.00", "0.00", "0.00"
}

func readMemInfo(procDir string) memDetails {
	f, err := os.Open(filepath.Join(procDir, "meminfo"))
	if err != nil {
		return memDetails{}
	}
	defer f.Close()
	var totalKB, availKB, cachedKB, buffersKB uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			totalKB = v
		case "MemAvailable:":
			availKB = v
		case "Cached:":
			cachedKB = v
		case "Buffers:":
			buffersKB = v
		}
	}
	if totalKB == 0 {
		return memDetails{}
	}
	usedKB := totalKB - availKB
	return memDetails{
		pct:      math.Round((float64(usedKB)/float64(totalKB))*1000) / 10,
		usedGB:   math.Round((float64(usedKB)/1048576.0)*10) / 10,
		totalGB:  math.Round((float64(totalKB)/1048576.0)*10) / 10,
		availGB:  math.Round((float64(availKB)/1048576.0)*10) / 10,
		cachedGB: math.Round((float64(cachedKB+buffersKB)/1048576.0)*10) / 10,
	}
}

func readDiskInfo(path string) diskDetails {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return diskDetails{mount: path}
	}
	total := float64(st.Blocks) * float64(st.Bsize)
	avail := float64(st.Bavail) * float64(st.Bsize)
	if total <= 0 {
		return diskDetails{mount: path}
	}
	used := total - avail
	return diskDetails{
		pct:     math.Round((used/total)*1000) / 10,
		usedGB:  math.Round((used/1_073_741_824.0)*10) / 10,
		totalGB: math.Round((total/1_073_741_824.0)*10) / 10,
		freeGB:  math.Round((avail/1_073_741_824.0)*10) / 10,
		mount:   path,
	}
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
	if mbps < 0.01 {
		return fmt.Sprintf("%.1f Kbit/s", mbps*1000.0)
	}
	if mbps < 1.0 {
		return fmt.Sprintf("%.0f Kbit/s", mbps*1000.0)
	}
	if mbps >= 100.0 {
		return fmt.Sprintf("%.0f Mbit/s", mbps)
	}
	return fmt.Sprintf("%.1f Mbit/s", mbps)
}

// ─── DOCKER ENGINE SOCKET CLIENT ───

type dockerPort struct {
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

type dockerContainerSummary struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Ports  []dockerPort      `json:"Ports"`
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
	Containers      []DockerContainerInfo
	UnhealthyNames  []string
	RunningByName   map[string]bool
	UnhealthyByName map[string]bool
	StatusByName    map[string]string
	DiscoveredWUD   string
	DiscoveredBesz  string
}

func collectDocker(socketPath, hostIP string) DockerSnapshot {
	snap := DockerSnapshot{
		TopRestarts:     []RestartEntry{},
		Containers:      []DockerContainerInfo{},
		RunningByName:   map[string]bool{},
		UnhealthyByName: map[string]bool{},
		StatusByName:    map[string]string{},
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

	for _, c := range list {
		if strings.ToLower(c.State) != "running" {
			continue
		}
		img := strings.ToLower(c.Image)
		cname := ""
		if len(c.Names) > 0 {
			cname = strings.ToLower(strings.TrimPrefix(c.Names[0], "/"))
		}
		if snap.DiscoveredWUD == "" && (strings.Contains(img, "wud") || strings.Contains(img, "whats-up-docker") || cname == "wud") {
			for _, p := range c.Ports {
				if p.PublicPort > 0 && p.Type == "tcp" {
					snap.DiscoveredWUD = fmt.Sprintf("http://%s:%d", hostIP, p.PublicPort)
					break
				}
			}
		}
		if snap.DiscoveredBesz == "" && strings.Contains(img, "beszel") && !strings.Contains(img, "agent") {
			for _, p := range c.Ports {
				if p.PublicPort > 0 && p.Type == "tcp" {
					snap.DiscoveredBesz = fmt.Sprintf("http://%s:%d", hostIP, p.PublicPort)
					break
				}
			}
		}
	}

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

	restartsList := make([]RestartEntry, 0)
	allContainers := make([]DockerContainerInfo, 0, len(results))

	for _, item := range results {
		name := "container"
		if len(item.sum.Names) > 0 {
			name = strings.TrimPrefix(item.sum.Names[0], "/")
		}
		lowerName := strings.ToLower(name)
		state := strings.ToLower(item.sum.State)
		stack := item.sum.Labels["com.docker.compose.project"]
		if stack == "" {
			stack = "standalone"
		}

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

		seenPort := map[string]bool{}
		ports := make([]string, 0)
		for _, p := range item.sum.Ports {
			if p.PublicPort > 0 {
				lbl := fmt.Sprintf(":%d→%d/%s", p.PublicPort, p.PrivatePort, p.Type)
				if !seenPort[lbl] {
					seenPort[lbl] = true
					ports = append(ports, lbl)
				}
			}
		}

		effHealth := "stopped"
		if state == "running" {
			snap.RunningByName[lowerName] = true
			snap.StatusByName[lowerName] = item.sum.Status
			if healthStatus == "unhealthy" {
				effHealth = "unhealthy"
				snap.Unhealthy++
				snap.Total++
				snap.UnhealthyByName[lowerName] = true
				snap.UnhealthyNames = append(snap.UnhealthyNames, name)
			} else if healthStatus == "healthy" {
				effHealth = "healthy"
				snap.Healthy++
				snap.Total++
			} else {
				effHealth = "running"
				snap.Healthy++
				snap.Total++
			}
		} else if state == "restarting" || (state == "exited" && exitCode != 0) || oom {
			effHealth = "unhealthy"
			snap.Unhealthy++
			snap.Total++
			snap.UnhealthyByName[lowerName] = true
			snap.UnhealthyNames = append(snap.UnhealthyNames, name)
			snap.StatusByName[lowerName] = item.sum.Status
		} else {
			snap.StatusByName[lowerName] = item.sum.Status
		}

		allContainers = append(allContainers, DockerContainerInfo{
			Name:       name,
			Stack:      stack,
			Image:      item.sum.Image,
			State:      state,
			StatusText: item.sum.Status,
			Health:     effHealth,
			Restarts:   rc,
			OOMKilled:  oom,
			Ports:      ports,
		})

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

	sort.Slice(allContainers, func(i, j int) bool {
		order := func(h string) int {
			switch h {
			case "unhealthy":
				return 0
			case "healthy":
				return 1
			case "running":
				return 2
			default:
				return 3
			}
		}
		oi, oj := order(allContainers[i].Health), order(allContainers[j].Health)
		if oi != oj {
			return oi < oj
		}
		return allContainers[i].Name < allContainers[j].Name
	})

	sort.Slice(restartsList, func(i, j int) bool {
		if restartsList[i].Count != restartsList[j].Count {
			return restartsList[i].Count > restartsList[j].Count
		}
		return restartsList[i].Name < restartsList[j].Name
	})
	if len(restartsList) > 4 {
		restartsList = restartsList[:4]
	}

	snap.Containers = allContainers
	snap.TopRestarts = restartsList
	return snap
}

// ─── WUD (WHAT'S UP DOCKER) LIVE API CLIENT ───

type wudAPIContainer struct {
	Name            string `json:"name"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Image           struct {
		Name string `json:"name"`
		Tag  struct {
			Value string `json:"value"`
		} `json:"tag"`
	} `json:"image"`
	UpdateKind struct {
		Kind        string `json:"kind"`
		LocalValue  string `json:"localValue"`
		RemoteValue string `json:"remoteValue"`
		SemverDiff  string `json:"semverDiff"`
	} `json:"updateKind"`
	Result struct {
		Tag  string `json:"tag"`
		Link string `json:"link"`
	} `json:"result"`
}

func isDatabaseSensitiveApp(name, image string) bool {
	s := strings.ToLower(name + " " + image)
	for _, kw := range []string{"immich", "paperless", "vaultwarden", "authelia", "nginx", "postgres", "mariadb", "mysql", "redis"} {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

func collectWUD(wudURL, user, pass string) WUDSnapshot {
	snap := WUDSnapshot{
		URL:     wudURL,
		Updates: []WUDUpdateItem{},
	}
	if wudURL == "" {
		return snap
	}

	client := &http.Client{
		Timeout: 4 * time.Second,
		Transport: &http.Transport{
			Proxy:           nil,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest("GET", wudURL+"/api/containers", nil)
	if err != nil {
		snap.Error = "Ungültige WUD_URL"
		return snap
	}
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	resp, err := client.Do(req)
	if err != nil {
		snap.Error = "WUD nicht erreichbar"
		return snap
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		snap.Error = "WUD erfordert Basic Auth (WUD_USER / WUD_PASS)"
		return snap
	}
	if resp.StatusCode != 200 {
		snap.Error = fmt.Sprintf("WUD HTTP %d", resp.StatusCode)
		return snap
	}

	var items []wudAPIContainer
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		snap.Error = "WUD JSON ungültig"
		return snap
	}

	snap.Available = true
	snap.TotalWatched = len(items)
	for _, c := range items {
		if !c.UpdateAvailable {
			continue
		}
		curTag := c.UpdateKind.LocalValue
		if curTag == "" {
			curTag = c.Image.Tag.Value
		}
		newTag := c.UpdateKind.RemoteValue
		if newTag == "" {
			newTag = c.Result.Tag
		}
		if newTag == "" {
			newTag = "neuer Digest"
		}
		diff := strings.ToLower(c.UpdateKind.SemverDiff)
		if diff == "" && strings.ToLower(c.UpdateKind.Kind) == "digest" {
			diff = "digest"
		}
		snap.Updates = append(snap.Updates, WUDUpdateItem{
			Container:   c.Name,
			Image:       c.Image.Name,
			CurrentTag:  curTag,
			NewTag:      newTag,
			SemverDiff:  diff,
			CautionDB:   isDatabaseSensitiveApp(c.Name, c.Image.Name),
			ReleaseLink: c.Result.Link,
		})
	}
	snap.UpdatesAvailable = len(snap.Updates)

	sort.Slice(snap.Updates, func(i, j int) bool {
		rank := func(u WUDUpdateItem) int {
			if u.SemverDiff == "major" {
				return 0
			}
			if u.CautionDB {
				return 1
			}
			if u.SemverDiff == "minor" {
				return 2
			}
			return 3
		}
		ri, rj := rank(snap.Updates[i]), rank(snap.Updates[j])
		if ri != rj {
			return ri < rj
		}
		return snap.Updates[i].Container < snap.Updates[j].Container
	})
	return snap
}

// ─── BESZEL POCKETBASE LIVE CLIENT ───

type BeszelClient struct {
	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

func (bc *BeszelClient) Collect(beszelURL, email, password string) BeszelSnapshot {
	snap := BeszelSnapshot{
		URL:   beszelURL,
		Hosts: []BeszelHostItem{},
	}
	if beszelURL == "" {
		return snap
	}

	client := &http.Client{
		Timeout: 4 * time.Second,
		Transport: &http.Transport{
			Proxy:           nil,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	if r, err := client.Get(beszelURL + "/api/health"); err == nil {
		snap.Reachable = r.StatusCode < 500
		r.Body.Close()
	} else if r2, err2 := client.Get(beszelURL); err2 == nil {
		snap.Reachable = r2.StatusCode < 500
		r2.Body.Close()
	}

	if !snap.Reachable {
		return snap
	}

	if email != "" && password != "" {
		tok := bc.getToken(client, beszelURL, email, password, false)
		if tok != "" {
			items, ok := fetchBeszelSystems(client, beszelURL, tok)
			if !ok {
				tok = bc.getToken(client, beszelURL, email, password, true)
				if tok != "" {
					items, ok = fetchBeszelSystems(client, beszelURL, tok)
				}
			}
			if ok && len(items) > 0 {
				snap.Available = true
				snap.Hosts = items
				snap.Total = len(items)
				for _, h := range items {
					if h.Status == "up" {
						snap.Online++
					}
				}
			}
		}
	}

	return snap
}

func (bc *BeszelClient) getToken(client *http.Client, baseURL, email, password string, force bool) string {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	if !force && bc.token != "" && time.Now().Before(bc.tokenExp) {
		return bc.token
	}
	payload, _ := json.Marshal(map[string]string{"identity": email, "password": password})
	resp, err := client.Post(baseURL+"/api/collections/users/auth-with-password", "application/json", bytes.NewReader(payload))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ""
	}
	var out struct {
		Token string `json:"token"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) == nil && out.Token != "" {
		bc.token = out.Token
		bc.tokenExp = time.Now().Add(6 * time.Hour)
		return bc.token
	}
	return ""
}

func fetchBeszelSystems(client *http.Client, baseURL, token string) ([]BeszelHostItem, bool) {
	req, err := http.NewRequest("GET", baseURL+"/api/collections/systems/records?perPage=50", nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("Authorization", token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, false
	}
	var out struct {
		Items []struct {
			Name   string `json:"name"`
			Host   string `json:"host"`
			Status string `json:"status"`
			Info   struct {
				CPU  float64 `json:"cpu"`
				MP   float64 `json:"mp"`
				DP   float64 `json:"dp"`
				Temp float64 `json:"t"`
			} `json:"info"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, false
	}
	hosts := make([]BeszelHostItem, 0, len(out.Items))
	for _, it := range out.Items {
		hosts = append(hosts, BeszelHostItem{
			Name:   it.Name,
			Host:   it.Host,
			Status: strings.ToLower(it.Status),
			CPU:    math.Round(it.Info.CPU*10) / 10,
			Mem:    math.Round(it.Info.MP*10) / 10,
			Disk:   math.Round(it.Info.DP*10) / 10,
			Temp:   math.Round(it.Info.Temp*10) / 10,
		})
	}
	return hosts, len(hosts) > 0
}

// ─── UPTIME KUMA CLIENT ───

type KumaMonitor struct {
	Name      string
	Status    int
	LatencyMs float64
	CertDays  float64
	HasCert   bool
}

type KumaSnapshot struct {
	Available bool
	Reachable bool
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
	client := &http.Client{
		Timeout: 4 * time.Second,
		Transport: &http.Transport{
			Proxy:           nil,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	if r, err := client.Get(baseURL); err == nil {
		snap.Reachable = r.StatusCode < 500
		r.Body.Close()
	}

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
			snap.Monitors[mon.Name] = KumaMonitor{Name: mon.Name, Status: status, LatencyMs: ping}
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

// ─── PROMETHEUS OPTIONAL QUERY ───

func queryPrometheusScalar(baseURL, promql string) (float64, bool) {
	if baseURL == "" {
		return 0, false
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(baseURL + "/api/v1/query?query=" + url.QueryEscape(promql))
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

// ─── SERVICE / HOST / SSL PROBERS ───

func matchContainerStatus(serviceName string, dockerSnap DockerSnapshot) string {
	if !dockerSnap.Available {
		return ""
	}
	kw := strings.ToLower(strings.Split(serviceName, " ")[0])
	kw = strings.Split(kw, "-")[0]
	for cname, statusText := range dockerSnap.StatusByName {
		if strings.Contains(cname, kw) {
			return cname + " (" + statusText + ")"
		}
	}
	return ""
}

func probeService(def ServiceDef, dockerSnap DockerSnapshot, kumaSnap KumaSnapshot) ServiceStatus {
	st := ServiceStatus{
		ID:              slugify(def.Name),
		Name:            def.Name,
		Category:        def.Category,
		Target:          def.Target,
		ContainerStatus: matchContainerStatus(def.Name, dockerSnap),
	}

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

	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			Proxy:           nil,
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
	st.HTTPCode = resp.StatusCode
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
		Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	resp, err := client.Get(def.Target)
	if err != nil {
		return false, "Nicht erreichbar"
	}
	resp.Body.Close()
	return resp.StatusCode < 500, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

type SSLCheckResult struct {
	Domain   string `json:"domain"`
	DaysLeft int    `json:"days_left"`
	Valid    bool   `json:"valid"`
	Issuer   string `json:"issuer,omitempty"`
	NotAfter string `json:"not_after,omitempty"`
	ErrText  string `json:"error,omitempty"`
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
	leaf := certs[0]
	days := int(time.Until(leaf.NotAfter).Hours() / 24.0)
	issuer := leaf.Issuer.CommonName
	if issuer == "" && len(leaf.Issuer.Organization) > 0 {
		issuer = leaf.Issuer.Organization[0]
	}
	return SSLCheckResult{
		Domain:   domain,
		DaysLeft: days,
		Valid:    days >= 0,
		Issuer:   issuer,
		NotAfter: leaf.NotAfter.Format("02.01.2006"),
	}
}

// ─── MAIN LIVE COLLECTOR ───

type LiveCollector struct {
	cfg     Config
	sampler *SystemSampler
	beszel  *BeszelClient
	mu      sync.RWMutex
	latest  DashboardPayload
}

func NewLiveCollector(cfg Config) *LiveCollector {
	lc := &LiveCollector{
		cfg:     cfg,
		sampler: NewSystemSampler(cfg.ProcDir, cfg.RootFSPath, cfg.NetInterface),
		beszel:  &BeszelClient{},
	}
	lc.seedInitial()
	return lc
}

func (lc *LiveCollector) seedInitial() {
	sys := lc.sampler.Snapshot()
	svcs := make([]ServiceStatus, 0, len(lc.cfg.CriticalServices))
	for _, sd := range lc.cfg.CriticalServices {
		svcs = append(svcs, ServiceStatus{
			ID:       slugify(sd.Name),
			Name:     sd.Name,
			Category: sd.Category,
			Target:   sd.Target,
			Online:   true,
			Latency:  "prüfe…",
		})
	}
	diskDetailStr := fmt.Sprintf("%.0f / %.0f GB", sys.disk.usedGB, sys.disk.totalGB)
	if sys.disk.totalGB >= 1000 {
		diskDetailStr = fmt.Sprintf("%.0f GB / %.2f TB", sys.disk.usedGB, sys.disk.totalGB/1024.0)
	}
	cpuSt, cpuTag := thresholdState(sys.cpuPct, 70, 85)
	ramSt, ramTag := thresholdState(sys.mem.pct, 75, 90)
	diskSt, diskTag := thresholdState(sys.disk.pct, 80, 90)

	lc.latest = DashboardPayload{
		Mode:            "live",
		Timestamp:       time.Now().Format(time.RFC3339),
		PollIntervalSec: int(lc.cfg.PollInterval.Seconds()),
		Hostname:        nonEmpty(sys.hostName, lc.cfg.HostIP),
		HostIP:          lc.cfg.HostIP,
		UptimeHuman:     sys.uptimeStr,
		Sources:         []string{"Host /proc"},
		Verdict:         VerdictInfo{Level: "ok", Text: "LIVE-SYNC LÄUFT · PRÜFE DIENSTE…"},
		ExecutiveSummary: []ExecStat{
			{ID: "services-online", Label: "Dienste online", Value: fmt.Sprintf("%d / %d", len(svcs), len(svcs)), Source: "Live-Probe", Status: "ok", Sub: "Prüfe Dienste…", Details: []string{"Erster Live-Check läuft…"}},
			{ID: "hosts-online", Label: "Hosts online", Value: "1 / 1", Source: "Host-Check", Status: "ok", Sub: fmt.Sprintf("%s · Uptime %s", nonEmpty(sys.hostName, lc.cfg.HostIP), sys.uptimeStr), Details: []string{"Host erreichbar"}},
			{ID: "containers-healthy", Label: "Container healthy", Value: "– / –", Source: "Docker", Status: "ok", Sub: "Lese Docker Engine…", Details: []string{"Initialisiere Docker Socket…"}},
			{ID: "ssl-status", Label: "SSL-Zertifikate", Value: "OK", Source: "Live TLS", Status: "ok", Sub: "Prüfe Zertifikate…", Details: []string{"TLS-Check läuft…"}},
		},
		CriticalServices: svcs,
		Infrastructure: InfraStatus{
			Source:     fmt.Sprintf("Live /proc · Host %s", nonEmpty(sys.hostName, lc.cfg.HostIP)),
			CPUHistory: sys.cpuHistory,
			RAMHistory: sys.ramHistory,
			Gauges: []GaugeStatus{
				{ID: "cpu", Label: "CPU-Last", Value: sys.cpuPct, Unit: "%", Status: cpuSt, StateTag: cpuTag, Detail: fmt.Sprintf("%d vCPU · Load %s", sys.cores, sys.load1), Threshold: "Grün < 70 % · Gelb 70–85 % · Rot > 85 %", Extra: []string{fmt.Sprintf("Load 1m / 5m / 15m: %s · %s · %s", sys.load1, sys.load5, sys.load15)}},
				{ID: "ram", Label: "Arbeitsspeicher", Value: sys.mem.pct, Unit: "%", Status: ramSt, StateTag: ramTag, Detail: fmt.Sprintf("%.1f / %.1f GB belegt", sys.mem.usedGB, sys.mem.totalGB), Threshold: "Grün < 75 % · Gelb 75–90 % · Rot > 90 %", Extra: []string{fmt.Sprintf("Verfügbar: %.1f GB · Cache: %.1f GB", sys.mem.availGB, sys.mem.cachedGB)}},
				{ID: "disk", Label: "Datenträger", Value: sys.disk.pct, Unit: "%", Status: diskSt, StateTag: diskTag, Detail: diskDetailStr, Threshold: "Grün < 80 % · Gelb 80–90 % · Rot > 90 %", Extra: []string{fmt.Sprintf("Frei: %.1f GB · Mount: %s", sys.disk.freeGB, sys.disk.mount)}},
			},
			Network: sys.net,
		},
		ContainerHealth: ContainerHealth{Available: false, Source: "Docker Engine", TopRestarts: []RestartEntry{}},
		Containers:      []DockerContainerInfo{},
		SSLCerts:        []SSLCheckResult{},
		WUD:             WUDSnapshot{Updates: []WUDUpdateItem{}},
		Beszel:          BeszelSnapshot{URL: lc.cfg.BeszelURL, Hosts: []BeszelHostItem{}},
		Kuma:            KumaPublicSnapshot{Monitors: []KumaPublicMonitor{}},
		Embeds: EmbedConfig{
			BeszelURL:      lc.cfg.BeszelEmbed,
			BeszelFrameURL: computeFrameURL(lc.cfg.BeszelEmbed, lc.cfg.EnableEmbedProxy, "/beszel"),
			KumaURL:        lc.cfg.UptimeKumaEmbed,
			KumaFrameURL:   computeFrameURL(lc.cfg.UptimeKumaEmbed, lc.cfg.EnableEmbedProxy, ""),
			ProxyEnabled:   lc.cfg.EnableEmbedProxy,
			DozzleURL:      lc.cfg.DozzleURL,
		},
		Incidents: []IncidentItem{
			{Level: "info", Badge: "SYNC", Title: "Initialer Live-Check läuft…", Meta: "Docker Engine · Dienste · TLS · WUD", Detail: "Erste vollständige Live-Abfrage dauert 1–2 Sekunden.", Time: "JETZT"},
		},
	}
}

func (lc *LiveCollector) Start(ctx context.Context) {
	lc.sampler.Start(ctx)
	ticker := time.NewTicker(lc.cfg.PollInterval)
	go func() {
		defer ticker.Stop()
		time.Sleep(600 * time.Millisecond)
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
	// Overlay freshest 2-second in-memory host CPU/RAM/Network telemetry on every poll
	sys := lc.sampler.Snapshot()
	p.Timestamp = time.Now().Format(time.RFC3339)
	p.UptimeHuman = sys.uptimeStr
	p.Infrastructure.Network = sys.net
	p.Infrastructure.CPUHistory = sys.cpuHistory
	p.Infrastructure.RAMHistory = sys.ramHistory
	if len(p.Infrastructure.Gauges) >= 2 {
		gauges := append([]GaugeStatus(nil), p.Infrastructure.Gauges...)
		cpuSt, cpuTag := thresholdState(sys.cpuPct, 70, 85)
		gauges[0].Value = sys.cpuPct
		gauges[0].Status = cpuSt
		gauges[0].StateTag = cpuTag
		gauges[0].Detail = fmt.Sprintf("%d vCPU · Load %s", sys.cores, sys.load1)
		ramSt, ramTag := thresholdState(sys.mem.pct, 75, 90)
		gauges[1].Value = sys.mem.pct
		gauges[1].Status = ramSt
		gauges[1].StateTag = ramTag
		gauges[1].Detail = fmt.Sprintf("%.1f / %.1f GB belegt", sys.mem.usedGB, sys.mem.totalGB)
		p.Infrastructure.Gauges = gauges
	}
	return p
}

func (lc *LiveCollector) refresh() {
	var sources []string
	var incidents []IncidentItem

	dockerSnap := collectDocker(lc.cfg.DockerSocket, lc.cfg.HostIP)
	if dockerSnap.Available {
		sources = append(sources, "Docker Engine")
	}

	sys := lc.sampler.Snapshot()
	hostName := sys.hostName
	if dockerSnap.Hostname != "" {
		hostName = dockerSnap.Hostname
	}
	sources = append(sources, "Host /proc")

	wudURL := lc.cfg.WUDURL
	if wudURL == "" {
		wudURL = dockerSnap.DiscoveredWUD
	}
	beszelURL := lc.cfg.BeszelURL
	if beszelURL == "" && dockerSnap.DiscoveredBesz != "" {
		beszelURL = dockerSnap.DiscoveredBesz
	}
	beszelEmbed := lc.cfg.BeszelEmbed
	if beszelEmbed == "" {
		beszelEmbed = beszelURL
	}

	var (
		kumaSnap   KumaSnapshot
		beszelSnap BeszelSnapshot
		wudSnap    WUDSnapshot
		promActive bool
		promUp     float64
		promAll    float64
	)

	var preWg sync.WaitGroup
	preWg.Add(4)
	go func() {
		defer preWg.Done()
		kumaSnap = collectUptimeKuma(lc.cfg.UptimeKumaURL, lc.cfg.UptimeKumaKey, lc.cfg.UptimeKumaSlug)
	}()
	go func() {
		defer preWg.Done()
		beszelSnap = lc.beszel.Collect(beszelURL, lc.cfg.BeszelEmail, lc.cfg.BeszelPassword)
	}()
	go func() {
		defer preWg.Done()
		wudSnap = collectWUD(wudURL, lc.cfg.WUDUser, lc.cfg.WUDPass)
	}()
	go func() {
		defer preWg.Done()
		u, ok1 := queryPrometheusScalar(lc.cfg.PrometheusURL, "count(up == 1)")
		a, ok2 := queryPrometheusScalar(lc.cfg.PrometheusURL, "count(up)")
		if ok1 && ok2 && a > 0 {
			promActive = true
			promUp = u
			promAll = a
		}
	}()
	preWg.Wait()

	if kumaSnap.Available {
		sources = append(sources, "Uptime Kuma")
	}
	if beszelSnap.Available {
		sources = append(sources, "Beszel")
	}
	if wudSnap.Available {
		sources = append(sources, "WUD")
	}
	if promActive {
		sources = append(sources, "Prometheus")
	}

	critResults := make([]ServiceStatus, len(lc.cfg.CriticalServices))
	extraResults := make([]ServiceStatus, len(lc.cfg.ExtraServices))
	type hostRes struct {
		name   string
		online bool
		detail string
	}
	hostResults := make([]hostRes, len(lc.cfg.Hosts))
	sslResults := make([]SSLCheckResult, len(lc.cfg.SSLDomains))

	var wg sync.WaitGroup
	for i, sdef := range lc.cfg.CriticalServices {
		wg.Add(1)
		go func(idx int, sd ServiceDef) {
			defer wg.Done()
			critResults[idx] = probeService(sd, dockerSnap, kumaSnap)
		}(i, sdef)
	}
	for i, sdef := range lc.cfg.ExtraServices {
		wg.Add(1)
		go func(idx int, sd ServiceDef) {
			defer wg.Done()
			extraResults[idx] = probeService(sd, dockerSnap, kumaSnap)
		}(i, sdef)
	}
	for i, hdef := range lc.cfg.Hosts {
		wg.Add(1)
		go func(idx int, hd HostDef) {
			defer wg.Done()
			ok, det := probeHost(hd)
			hostResults[idx] = hostRes{name: hd.Name, online: ok, detail: det}
		}(i, hdef)
	}
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

	// Build public Kuma list
	kumaPub := KumaPublicSnapshot{
		Available: kumaSnap.Available,
		Reachable: kumaSnap.Reachable,
		Online:    kumaSnap.Online,
		Total:     kumaSnap.Total,
		Monitors:  []KumaPublicMonitor{},
	}
	for _, m := range kumaSnap.Monitors {
		kumaPub.Monitors = append(kumaPub.Monitors, KumaPublicMonitor{
			Name:      m.Name,
			Online:    m.Status == 1,
			LatencyMs: math.Round(m.LatencyMs),
			CertDays:  int(m.CertDays),
		})
	}
	sort.Slice(kumaPub.Monitors, func(i, j int) bool {
		if kumaPub.Monitors[i].Online != kumaPub.Monitors[j].Online {
			return !kumaPub.Monitors[i].Online
		}
		return kumaPub.Monitors[i].Name < kumaPub.Monitors[j].Name
	})

	// ─── ZEILE 1: EXECUTIVE SUMMARY (WITH EXPANDABLE DETAILS!) ───
	svcOnline, svcTotal := 0, 0
	svcSource := "Live-Probe"
	svcDetails := make([]string, 0)
	if kumaSnap.Available {
		svcOnline = kumaSnap.Online
		svcTotal = kumaSnap.Total
		svcSource = "Uptime Kuma"
		for _, m := range kumaPub.Monitors {
			stTxt := "ONLINE"
			if !m.Online {
				stTxt = "OFFLINE"
			}
			svcDetails = append(svcDetails, fmt.Sprintf("%s: %s (%.0f ms)", m.Name, stTxt, m.LatencyMs))
		}
	} else {
		for _, s := range critResults {
			svcTotal++
			if s.Online {
				svcOnline++
				svcDetails = append(svcDetails, fmt.Sprintf("✓ %s (%s)", s.Name, s.Latency))
			} else {
				svcDetails = append(svcDetails, fmt.Sprintf("✕ %s (%s)", s.Name, s.Latency))
			}
		}
		for _, s := range extraResults {
			svcTotal++
			if s.Online {
				svcOnline++
				svcDetails = append(svcDetails, fmt.Sprintf("✓ %s (%s)", s.Name, s.Latency))
			} else {
				svcDetails = append(svcDetails, fmt.Sprintf("✕ %s (%s)", s.Name, s.Latency))
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

	hostsOnline, hostsTotal := 0, 0
	hostSource := "Host-Check"
	hostDetails := make([]string, 0)
	if beszelSnap.Available {
		hostsOnline = beszelSnap.Online
		hostsTotal = beszelSnap.Total
		hostSource = "Beszel"
		for _, h := range beszelSnap.Hosts {
			hostDetails = append(hostDetails, fmt.Sprintf("%s (%s): CPU %.0f%% · RAM %.0f%% · Disk %.0f%%", h.Name, strings.ToUpper(h.Status), h.CPU, h.Mem, h.Disk))
			if h.Status != "up" {
				incidents = append(incidents, IncidentItem{
					Level:  "crit",
					Badge:  "KRITISCH",
					Title:  fmt.Sprintf("Host offline · %s", h.Name),
					Meta:   fmt.Sprintf("Beszel · Status: %s (%s)", h.Status, h.Host),
					Detail: "Beszel meldet keinen aktiven Agent-Heartbeat. Erreichbarkeit und Stromversorgung prüfen.",
					Time:   "AKTIV",
					Rank:   1,
				})
			}
		}
	} else if promActive {
		hostsOnline = int(promUp)
		hostsTotal = int(promAll)
		hostSource = "Prometheus"
		hostDetails = append(hostDetails, fmt.Sprintf("Prometheus Targets up: %d / %d", hostsOnline, hostsTotal))
	} else {
		for _, h := range hostResults {
			hostsTotal++
			if h.online {
				hostsOnline++
				hostDetails = append(hostDetails, fmt.Sprintf("✓ %s (%s · Uptime %s)", h.name, h.detail, sys.uptimeStr))
			} else {
				hostDetails = append(hostDetails, fmt.Sprintf("✕ %s (%s)", h.name, h.detail))
				incidents = append(incidents, IncidentItem{
					Level:  "crit",
					Badge:  "KRITISCH",
					Title:  fmt.Sprintf("Host offline · %s", h.name),
					Meta:   fmt.Sprintf("Host-Check · %s", h.detail),
					Detail: "Host antwortet nicht auf den Live-Netzwerkcheck.",
					Time:   "AKTIV",
					Rank:   1,
				})
			}
		}
	}
	hostStatus := "ok"
	hostSub := fmt.Sprintf("%s · Uptime %s", hostName, sys.uptimeStr)
	if hostsOnline < hostsTotal {
		hostStatus = "crit"
		hostSub = fmt.Sprintf("%d Host(s) nicht erreichbar", hostsTotal-hostsOnline)
	}

	cntValue := "n/a"
	cntStatus := "warn"
	cntSub := "Docker Socket (/var/run/docker.sock) nicht gemountet"
	cntDetails := make([]string, 0)
	if dockerSnap.Available {
		cntValue = fmt.Sprintf("%d / %d", dockerSnap.Healthy, dockerSnap.Total)
		if dockerSnap.Unhealthy > 0 {
			cntStatus = "crit"
			cntSub = fmt.Sprintf("%d Container UNHEALTHY / gestoppt", dockerSnap.Unhealthy)
		} else {
			cntStatus = "ok"
			cntSub = fmt.Sprintf("%d laufende Container · 0 unhealthy", dockerSnap.Healthy)
		}
		cntDetails = append(cntDetails,
			fmt.Sprintf("Healthy / Laufend: %d", dockerSnap.Healthy),
			fmt.Sprintf("Unhealthy / Fehler: %d", dockerSnap.Unhealthy),
			fmt.Sprintf("Neustarts gesamt: %d", dockerSnap.Restarts),
		)
		if wudSnap.Available {
			cntDetails = append(cntDetails, fmt.Sprintf("WUD Updates verfügbar: %d / %d", wudSnap.UpdatesAvailable, wudSnap.TotalWatched))
		}
	}

	sslValue := "OK"
	sslStatus := "ok"
	sslSource := "Live TLS"
	sslSub := "Keine SSL-Domains konfiguriert"
	sslDetails := make([]string, 0)
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
			sslDetails = append(sslDetails, fmt.Sprintf("✕ %s: %s", r.Domain, r.ErrText))
		} else {
			sslDetails = append(sslDetails, fmt.Sprintf("✓ %s: noch %d Tage (bis %s · %s)", r.Domain, r.DaysLeft, r.NotAfter, r.Issuer))
			if r.DaysLeft < minSSL {
				minSSL = r.DaysLeft
				minSSLDom = r.Domain
			}
		}
	}

	if sslFailed != "" {
		sslValue = "FEHLER"
		sslStatus = "crit"
		sslSub = fmt.Sprintf("TLS-Fehler bei %s", sslFailed)
		incidents = append(incidents, IncidentItem{
			Level:  "crit",
			Badge:  "KRITISCH",
			Title:  fmt.Sprintf("SSL/TLS-Prüfung fehlgeschlagen · %s", sslFailed),
			Meta:   "Live TLS · Port 443 oder Zertifikat fehlerhaft",
			Detail: "Prüfe Nginx Proxy Manager SSL-Zertifikate und Cloudflare DNS/Proxy-Einstellungen.",
			Time:   "AKTIV",
			Rank:   2,
		})
	} else if minSSL < 9999 {
		sslSub = fmt.Sprintf("Min. %d Tage · %s", minSSL, minSSLDom)
		if minSSL < 7 {
			sslValue = fmt.Sprintf("%d TAGE", minSSL)
			sslStatus = "crit"
			incidents = append(incidents, IncidentItem{
				Level:  "crit",
				Badge:  "KRITISCH",
				Title:  fmt.Sprintf("SSL läuft in %d Tagen ab · %s", minSSL, minSSLDom),
				Meta:   sslSource + " · Sofortige Erneuerung erforderlich",
				Detail: "Zertifikat im Nginx Proxy Manager manuell erneuern und Let's Encrypt Logs prüfen.",
				Time:   fmt.Sprintf("%dd Rest", minSSL),
				Rank:   2,
			})
		} else if minSSL <= 21 {
			sslValue = fmt.Sprintf("%d TAGE", minSSL)
			sslStatus = "warn"
			incidents = append(incidents, IncidentItem{
				Level:  "warn",
				Badge:  "WARNUNG",
				Title:  fmt.Sprintf("SSL läuft in %d Tagen ab · %s", minSSL, minSSLDom),
				Meta:   sslSource + " · Auto-Renewal beobachten",
				Detail: "Zertifikat befindet sich im Erneuerungsfenster von Let's Encrypt.",
				Time:   fmt.Sprintf("%dd Rest", minSSL),
				Rank:   5,
			})
		}
	}

	// ─── INCIDENTS FROM CRITICAL SERVICES ───
	critOffCount := 0
	for _, s := range critResults {
		if !s.Online {
			critOffCount++
			incidents = append(incidents, IncidentItem{
				Level:  "crit",
				Badge:  "KRITISCH",
				Title:  fmt.Sprintf("%s OFFLINE (%s)", s.Name, s.Latency),
				Meta:   fmt.Sprintf("Ziel: %s · %s", s.Target, s.Error),
				Detail: fmt.Sprintf("Container-Status: %s. Prüfe Logs in Dozzle oder Portainer.", nonEmpty(s.ContainerStatus, "nicht zugeordnet")),
				Time:   "AKTIV",
				Rank:   1,
			})
		}
	}

	// ─── INFRASTRUCTURE GAUGES (WITH EXPANDABLE TECHNICAL DETAILS) ───
	cpuSt, cpuTag := thresholdState(sys.cpuPct, 70, 85)
	ramSt, ramTag := thresholdState(sys.mem.pct, 75, 90)
	diskSt, diskTag := thresholdState(sys.disk.pct, 80, 90)

	if cpuSt == "crit" {
		incidents = append(incidents, IncidentItem{Level: "crit", Badge: "KRITISCH", Title: fmt.Sprintf("CPU-Last kritisch (%.0f %%)", sys.cpuPct), Meta: fmt.Sprintf("Host %s · Load %s", hostName, sys.load1), Detail: fmt.Sprintf("Load Average: %s / %s / %s auf %d vCPUs.", sys.load1, sys.load5, sys.load15, sys.cores), Time: "AKTIV", Rank: 2})
	} else if cpuSt == "warn" {
		incidents = append(incidents, IncidentItem{Level: "warn", Badge: "WARNUNG", Title: fmt.Sprintf("CPU-Last erhöht (%.0f %%)", sys.cpuPct), Meta: fmt.Sprintf("Host %s · Load %s", hostName, sys.load1), Detail: fmt.Sprintf("Load Average: %s / %s / %s auf %d vCPUs.", sys.load1, sys.load5, sys.load15, sys.cores), Time: "AKTIV", Rank: 4})
	}
	if ramSt == "crit" {
		incidents = append(incidents, IncidentItem{Level: "crit", Badge: "KRITISCH", Title: fmt.Sprintf("Arbeitsspeicher kritisch (%.0f %%)", sys.mem.pct), Meta: fmt.Sprintf("%.1f / %.1f GB belegt", sys.mem.usedGB, sys.mem.totalGB), Detail: fmt.Sprintf("Verfügbar: %.1f GB · Buffers/Cache: %.1f GB.", sys.mem.availGB, sys.mem.cachedGB), Time: "AKTIV", Rank: 2})
	} else if ramSt == "warn" {
		incidents = append(incidents, IncidentItem{Level: "warn", Badge: "WARNUNG", Title: fmt.Sprintf("Arbeitsspeicher erhöht (%.0f %%)", sys.mem.pct), Meta: fmt.Sprintf("%.1f / %.1f GB belegt", sys.mem.usedGB, sys.mem.totalGB), Detail: fmt.Sprintf("Verfügbar: %.1f GB · Buffers/Cache: %.1f GB.", sys.mem.availGB, sys.mem.cachedGB), Time: "AKTIV", Rank: 4})
	}
	if diskSt == "crit" {
		incidents = append(incidents, IncidentItem{Level: "crit", Badge: "KRITISCH", Title: fmt.Sprintf("Datenträger kritisch voll (%.0f %%)", sys.disk.pct), Meta: fmt.Sprintf("%.0f / %.0f GB belegt", sys.disk.usedGB, sys.disk.totalGB), Detail: fmt.Sprintf("Nur noch %.1f GB frei auf %s. Docker Images (`docker system prune`) oder Immich/Backup-Storage prüfen.", sys.disk.freeGB, sys.disk.mount), Time: "AKTIV", Rank: 2})
	} else if diskSt == "warn" {
		incidents = append(incidents, IncidentItem{Level: "warn", Badge: "WARNUNG", Title: fmt.Sprintf("Datenträger über Warnschwelle (%.0f %%)", sys.disk.pct), Meta: fmt.Sprintf("%.0f / %.0f GB belegt", sys.disk.usedGB, sys.disk.totalGB), Detail: fmt.Sprintf("Freier Speicher: %.1f GB auf %s.", sys.disk.freeGB, sys.disk.mount), Time: "AKTIV", Rank: 4})
	}

	// ─── CONTAINER & WUD INCIDENTS ───
	for _, uname := range dockerSnap.UnhealthyNames {
		incidents = append(incidents, IncidentItem{
			Level:  "crit",
			Badge:  "KRITISCH",
			Title:  fmt.Sprintf("Container unhealthy: %s", uname),
			Meta:   "Docker Engine · Healthcheck fehlgeschlagen oder abgestürzt",
			Detail: "Container im Reiter Live-Telemetrie oder über Dozzle prüfen.",
			Time:   "AKTIV",
			Rank:   2,
		})
	}
	for _, r := range dockerSnap.TopRestarts {
		incidents = append(incidents, IncidentItem{
			Level:  "warn",
			Badge:  "WARNUNG",
			Title:  fmt.Sprintf("%s restart x%d", r.Name, r.Count),
			Meta:   fmt.Sprintf("Docker Engine · %s", r.Reason),
			Detail: fmt.Sprintf("Aktueller Zustand: %s. Neustarts über Dozzle-Logs analysieren.", r.State),
			Time:   "Container",
			Rank:   5,
		})
	}
	if wudSnap.Available && wudSnap.UpdatesAvailable > 0 {
		names := make([]string, 0, minInt(3, len(wudSnap.Updates)))
		for i := 0; i < minInt(3, len(wudSnap.Updates)); i++ {
			names = append(names, wudSnap.Updates[i].Container)
		}
		incidents = append(incidents, IncidentItem{
			Level:  "warn",
			Badge:  "UPDATE",
			Title:  fmt.Sprintf("WUD: %d Container-Update(s) verfügbar", wudSnap.UpdatesAvailable),
			Meta:   fmt.Sprintf("Betroffen: %s", strings.Join(names, ", ")),
			Detail: "Öffne den Reiter WUD Updates für Version-Diffs und DB-Backup-Hinweise.",
			Time:   "WUD",
			Rank:   6,
		})
	}

	if len(incidents) < 4 {
		if minSSL < 9999 && minSSL > 21 {
			incidents = append(incidents, IncidentItem{
				Level:  "info",
				Badge:  "INFO",
				Title:  fmt.Sprintf("SSL-Zertifikate gültig (noch %d Tage)", minSSL),
				Meta:   fmt.Sprintf("%s · Kürzeste Laufzeit: %s", sslSource, minSSLDom),
				Detail: strings.Join(sslDetails, " · "),
				Time:   "OK",
				Rank:   8,
			})
		}
		if wudSnap.Available && wudSnap.UpdatesAvailable == 0 {
			incidents = append(incidents, IncidentItem{
				Level:  "info",
				Badge:  "INFO",
				Title:  fmt.Sprintf("WUD: Alle %d überwachten Container aktuell", wudSnap.TotalWatched),
				Meta:   "What's Up Docker · Keine ausstehenden Image-Updates",
				Detail: "Letzter Live-Abgleich über WUD API erfolgreich.",
				Time:   "AKTUELL",
				Rank:   8,
			})
		}
		if dockerSnap.Available {
			incidents = append(incidents, IncidentItem{
				Level:  "info",
				Badge:  "INFO",
				Title:  fmt.Sprintf("Docker Engine stabil · %d / %d Container healthy", dockerSnap.Healthy, dockerSnap.Total),
				Meta:   fmt.Sprintf("Host %s · %d Neustarts registriert", hostName, dockerSnap.Restarts),
				Detail: "Alle Container-Details und Stacks findest du live im Reiter Live-Telemetrie.",
				Time:   "LIVE",
				Rank:   9,
			})
		}
		incidents = append(incidents, IncidentItem{
			Level:  "info",
			Badge:  "INFO",
			Title:  fmt.Sprintf("Host %s online · Uptime %s", hostName, sys.uptimeStr),
			Meta:   fmt.Sprintf("Aktive Quellen: %s", strings.Join(sources, " · ")),
			Detail: fmt.Sprintf("Load Average: %s / %s / %s · RAM %.1f/%.1f GB · RX %.2f GB / TX %.2f GB.", sys.load1, sys.load5, sys.load15, sys.mem.usedGB, sys.mem.totalGB, sys.net.RxTotalGB, sys.net.TxTotalGB),
			Time:   "SYNC",
			Rank:   10,
		})
	}

	sort.SliceStable(incidents, func(i, j int) bool {
		return incidents[i].Rank < incidents[j].Rank
	})
	if len(incidents) > 5 {
		incidents = incidents[:5]
	}

	// ─── GLOBAL EXECUTIVE VERDICT ───
	verdict := VerdictInfo{Level: "ok", Text: "ALLE SYSTEME NOMINAL · KEIN EINGRIFF ERFORDERLICH"}
	hasCrit := critOffCount > 0 || svcStatus == "crit" || hostStatus == "crit" || cntStatus == "crit" || sslStatus == "crit" || cpuSt == "crit" || ramSt == "crit" || diskSt == "crit"
	hasWarn := svcStatus == "warn" || hostStatus == "warn" || sslStatus == "warn" || cpuSt == "warn" || ramSt == "warn" || diskSt == "warn" || dockerSnap.Restarts > 0

	if hasCrit {
		verdict = VerdictInfo{Level: "crit", Text: "EINGRIFF ERFORDERLICH · KRITISCHE STÖRUNG ERKANNT"}
		if critOffCount > 0 {
			verdict.Text = fmt.Sprintf("EINGRIFF ERFORDERLICH · %d KRITISCHER DIENST OFFLINE", critOffCount)
		} else if dockerSnap.Unhealthy > 0 {
			verdict.Text = fmt.Sprintf("EINGRIFF ERFORDERLICH · %d CONTAINER UNHEALTHY", dockerSnap.Unhealthy)
		}
	} else if hasWarn {
		verdict = VerdictInfo{Level: "warn", Text: "WARNUNG AKTIV · RESSOURCEN / RESTARTS / SSL PRÜFEN"}
	}

	diskDetailStr := fmt.Sprintf("%.0f / %.0f GB", sys.disk.usedGB, sys.disk.totalGB)
	if sys.disk.totalGB >= 1000 {
		diskDetailStr = fmt.Sprintf("%.0f GB / %.2f TB", sys.disk.usedGB, sys.disk.totalGB/1024.0)
	}

	payload := DashboardPayload{
		Mode:            "live",
		Timestamp:       time.Now().Format(time.RFC3339),
		PollIntervalSec: int(lc.cfg.PollInterval.Seconds()),
		Hostname:        hostName,
		HostIP:          lc.cfg.HostIP,
		UptimeHuman:     sys.uptimeStr,
		Sources:         sources,
		Verdict:         verdict,
		ExecutiveSummary: []ExecStat{
			{ID: "services-online", Label: "Dienste online", Value: fmt.Sprintf("%d / %d", svcOnline, svcTotal), Source: svcSource, Status: svcStatus, Sub: svcSub, Details: svcDetails},
			{ID: "hosts-online", Label: "Hosts online", Value: fmt.Sprintf("%d / %d", hostsOnline, hostsTotal), Source: hostSource, Status: hostStatus, Sub: hostSub, Details: hostDetails},
			{ID: "containers-healthy", Label: "Container healthy", Value: cntValue, Source: "Docker Engine", Status: cntStatus, Sub: cntSub, Details: cntDetails},
			{ID: "ssl-status", Label: "SSL-Zertifikate", Value: sslValue, Source: sslSource, Status: sslStatus, Sub: sslSub, Details: sslDetails},
		},
		CriticalServices: critResults,
		Infrastructure: InfraStatus{
			Source:     fmt.Sprintf("Live /proc · Host %s", hostName),
			CPUHistory: sys.cpuHistory,
			RAMHistory: sys.ramHistory,
			Gauges: []GaugeStatus{
				{
					ID: "cpu", Label: "CPU-Last", Value: sys.cpuPct, Unit: "%", Status: cpuSt, StateTag: cpuTag,
					Detail: fmt.Sprintf("%d vCPU · Load %s", sys.cores, sys.load1), Threshold: "Grün < 70 % · Gelb 70–85 % · Rot > 85 %",
					Extra: []string{fmt.Sprintf("Load 1m / 5m / 15m: %s · %s · %s", sys.load1, sys.load5, sys.load15), fmt.Sprintf("Kerne: %d vCPUs · Uptime: %s", sys.cores, sys.uptimeStr)},
				},
				{
					ID: "ram", Label: "Arbeitsspeicher", Value: sys.mem.pct, Unit: "%", Status: ramSt, StateTag: ramTag,
					Detail: fmt.Sprintf("%.1f / %.1f GB belegt", sys.mem.usedGB, sys.mem.totalGB), Threshold: "Grün < 75 % · Gelb 75–90 % · Rot > 90 %",
					Extra: []string{fmt.Sprintf("Verfügbar: %.1f GB · Belegt: %.1f GB", sys.mem.availGB, sys.mem.usedGB), fmt.Sprintf("Buffers & Page-Cache: %.1f GB", sys.mem.cachedGB)},
				},
				{
					ID: "disk", Label: "Datenträger", Value: sys.disk.pct, Unit: "%", Status: diskSt, StateTag: diskTag,
					Detail: diskDetailStr, Threshold: "Grün < 80 % · Gelb 80–90 % · Rot > 90 %",
					Extra: []string{fmt.Sprintf("Frei: %.1f GB · Mountpoint: %s", sys.disk.freeGB, sys.disk.mount), fmt.Sprintf("Belegt: %.1f GB von %.1f GB", sys.disk.usedGB, sys.disk.totalGB)},
				},
			},
			Network: sys.net,
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
		Containers: dockerSnap.Containers,
		SSLCerts:   sslResults,
		WUD:        wudSnap,
		Beszel:     beszelSnap,
		Kuma:       kumaPub,
		Embeds: EmbedConfig{
			BeszelURL:      beszelEmbed,
			BeszelFrameURL: computeFrameURL(beszelEmbed, lc.cfg.EnableEmbedProxy, "/beszel"),
			BeszelOnline:   beszelSnap.Reachable,
			KumaURL:        lc.cfg.UptimeKumaEmbed,
			KumaFrameURL:   computeFrameURL(lc.cfg.UptimeKumaEmbed, lc.cfg.EnableEmbedProxy, ""),
			KumaOnline:     kumaSnap.Reachable,
			ProxyEnabled:   lc.cfg.EnableEmbedProxy,
			WUDURL:         wudURL,
			WUDOnline:      wudSnap.Available,
			DozzleURL:      lc.cfg.DozzleURL,
		},
		Incidents: incidents,
	}

	lc.mu.Lock()
	lc.latest = payload
	lc.mu.Unlock()
}

func computeFrameURL(directURL string, proxyEnabled bool, rootAlias string) string {
	if !proxyEnabled || directURL == "" {
		return directURL
	}
	u, err := url.Parse(directURL)
	if err != nil {
		return directURL
	}
	if u.Path == "" || u.Path == "/" {
		if rootAlias != "" {
			if u.RawQuery != "" {
				return rootAlias + "?" + u.RawQuery
			}
			return rootAlias
		}
		return directURL
	}
	p := u.Path
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	return p
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

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
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

// ─── SAME-ORIGIN EMBED REVERSE PROXY (BESZEL & UPTIME KUMA) ───
// Strips X-Frame-Options & CSP frame-ancestors so Beszel (:8099) & Kuma (:3001)
// render cleanly inside the dashboard tabs without browser cross-origin blocks.

type embedCtxKey int

const ctxKeyRootAlias embedCtxKey = 1

func injectRootPathRewrite(html []byte) []byte {
	snippet := []byte(`<script>try{history.replaceState(null,"","/")}catch(e){}</script>`)
	lower := bytes.ToLower(html)
	if idx := bytes.Index(lower, []byte("<head>")); idx != -1 {
		pos := idx + len("<head>")
		out := make([]byte, 0, len(html)+len(snippet))
		out = append(out, html[:pos]...)
		out = append(out, snippet...)
		out = append(out, html[pos:]...)
		return out
	}
	return append(snippet, html...)
}

func newEmbedProxy(name, rawTarget string) (*httputil.ReverseProxy, *url.URL) {
	if rawTarget == "" {
		return nil, nil
	}
	target, err := url.Parse(rawTarget)
	if err != nil || target.Host == "" {
		return nil, nil
	}
	originStr := target.Scheme + "://" + target.Host
	transport := &http.Transport{
		Proxy:           nil,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          64,
		IdleConnTimeout:       60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	proxy := &httputil.ReverseProxy{
		Transport:     transport,
		FlushInterval: -1, // Instant flush for PocketBase SSE (/api/realtime) & Socket.IO
		Director: func(req *http.Request) {
			if req.Context().Value(ctxKeyRootAlias) == true {
				req.URL.Path = "/"
				req.URL.RawPath = ""
				req.Header.Del("Accept-Encoding") // Request plain HTML so we can inject history.replaceState
			}
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			if req.Header.Get("Origin") != "" {
				req.Header.Set("Origin", originStr)
			}
			if ref := req.Header.Get("Referer"); ref != "" {
				if ru, err := url.Parse(ref); err == nil {
					ru.Scheme = target.Scheme
					ru.Host = target.Host
					req.Header.Set("Referer", ru.String())
				}
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("X-Frame-Options")
			resp.Header.Del("Content-Security-Policy")
			resp.Header.Del("Content-Security-Policy-Report-Only")
			if loc := resp.Header.Get("Location"); loc != "" && strings.HasPrefix(loc, originStr) {
				rel := strings.TrimPrefix(loc, originStr)
				if rel != "" && rel != "/" {
					resp.Header.Set("Location", rel)
				}
			}
			if resp.Request != nil && resp.Request.Context().Value(ctxKeyRootAlias) == true &&
				strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") &&
				resp.Header.Get("Content-Encoding") == "" {
				body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
				resp.Body.Close()
				if err == nil {
					body = injectRootPathRewrite(body)
					resp.Body = io.NopCloser(bytes.NewReader(body))
					resp.ContentLength = int64(len(body))
					resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
					resp.Header.Del("Transfer-Encoding")
				}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = fmt.Fprintf(w, `<!doctype html><html lang="de"><head><meta charset="utf-8"><style>body{margin:0;padding:24px;font-family:system-ui,sans-serif;background:#121826;color:#f6f5f1}h3{margin:0 0 8px;color:#f87171}code{background:rgba(255,255,255,.08);padding:2px 6px;border-radius:5px}a{color:#38bdf8}</style></head><body><h3>%s aktuell nicht erreichbar</h3><p>Ziel: <code>%s</code> (%s)</p><p><a href="%s" target="_blank" rel="noopener">Direkt in neuem Tab öffnen ↗</a></p></body></html>`,
				name, originStr, err.Error(), rawTarget)
		},
	}
	return proxy, target
}

func probeUpstreamExists(client *http.Client, base *url.URL, reqPath string) bool {
	if base == nil {
		return false
	}
	u := base.Scheme + "://" + base.Host + reqPath
	req, err := http.NewRequest(http.MethodHead, u, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusMethodNotAllowed {
		req2, err2 := http.NewRequest(http.MethodGet, u, nil)
		if err2 != nil {
			return false
		}
		resp2, err2 := client.Do(req2)
		if err2 != nil {
			return false
		}
		_, _ = io.CopyN(io.Discard, resp2.Body, 64)
		resp2.Body.Close()
		return resp2.StatusCode >= 200 && resp2.StatusCode < 400
	}
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

func isKumaPath(p string) bool {
	return p == "/status" ||
		strings.HasPrefix(p, "/status/") ||
		strings.HasPrefix(p, "/api/status-page/") ||
		strings.HasPrefix(p, "/api/badge/") ||
		strings.HasPrefix(p, "/api/entry-page") ||
		strings.HasPrefix(p, "/upload/") ||
		strings.HasPrefix(p, "/socket.io/") ||
		p == "/icon.svg"
}

func isBeszelPath(p string) bool {
	return p == "/system" ||
		strings.HasPrefix(p, "/system/") ||
		p == "/settings" ||
		strings.HasPrefix(p, "/settings/") ||
		strings.HasPrefix(p, "/api/") ||
		strings.HasPrefix(p, "/static/") ||
		strings.HasPrefix(p, "/_/")
}

// ─── HTTP SERVER (SERVES EMBEDDED WEB UI DIRECTLY FROM RAM + SAME-ORIGIN EMBED PROXY) ───

func main() {
	cfg := loadConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector := NewLiveCollector(cfg)
	collector.Start(ctx)

	webSub, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		log.Fatalf("embed fs error: %v", err)
	}
	fileServer := http.FileServer(http.FS(webSub))

	// Build atomic single-document HTML in RAM (CSS + JS inlined) so browser caches
	// can NEVER serve mismatched HTML + stale CSS/JS after a Portainer redeploy.
	indexRaw, err := fs.ReadFile(webSub, "index.html")
	if err != nil {
		log.Fatalf("read embedded index.html: %v", err)
	}
	cssRaw, err := fs.ReadFile(webSub, "css/style.css")
	if err != nil {
		log.Fatalf("read embedded css/style.css: %v", err)
	}
	jsRaw, err := fs.ReadFile(webSub, "js/main.js")
	if err != nil {
		log.Fatalf("read embedded js/main.js: %v", err)
	}
	atomicHTMLStr := string(indexRaw)
	atomicHTMLStr = strings.Replace(atomicHTMLStr, `<link rel="stylesheet" href="/css/style.css" />`, "<style>\n"+string(cssRaw)+"\n</style>", 1)
	atomicHTMLStr = strings.Replace(atomicHTMLStr, `<script src="/js/main.js" defer></script>`, "<script>\n"+string(jsRaw)+"\n</script>", 1)
	atomicIndexHTML := []byte(atomicHTMLStr)

	var beszelProxy, kumaProxy *httputil.ReverseProxy
	var beszelTarget, kumaTarget *url.URL
	if cfg.EnableEmbedProxy {
		beszelProxy, beszelTarget = newEmbedProxy("Beszel", cfg.BeszelURL)
		kumaProxy, kumaTarget = newEmbedProxy("Uptime Kuma", cfg.UptimeKumaURL)
	}
	probeClient := &http.Client{
		Timeout:   2 * time.Second,
		Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	var assetOwner sync.Map // map[path]string ("kuma" | "beszel")

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		payload := collector.Latest()
		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path

		// 1. Root NOC Dashboard (atomic inline CSS+JS, never stale-cached)
		if p == "/" || p == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
			w.Header().Set("Pragma", "no-cache")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(atomicIndexHTML)
			return
		}

		// 2. Embedded static assets (self-hosted WOFF2 fonts, favicon, direct CSS/JS)
		cleanRel := strings.TrimPrefix(filepath.Clean(p), "/")
		if cleanRel != "" && !strings.HasPrefix(cleanRel, "..") {
			if info, err := fs.Stat(webSub, cleanRel); err == nil && !info.IsDir() {
				if strings.HasPrefix(p, "/fonts/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// 3. Same-origin embed reverse proxy for Uptime Kuma & Beszel
		if cfg.EnableEmbedProxy {
			if (p == "/beszel" || p == "/beszel/") && beszelProxy != nil {
				ctx := context.WithValue(r.Context(), ctxKeyRootAlias, true)
				beszelProxy.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			if isKumaPath(p) && kumaProxy != nil {
				kumaProxy.ServeHTTP(w, r)
				return
			}
			if isBeszelPath(p) && beszelProxy != nil {
				beszelProxy.ServeHTTP(w, r)
				return
			}

			// Shared SPA asset / locale / manifest routing (/assets/*, /locales/*, etc.)
			if owner, ok := assetOwner.Load(p); ok {
				if owner == "kuma" && kumaProxy != nil {
					kumaProxy.ServeHTTP(w, r)
					return
				}
				if owner == "beszel" && beszelProxy != nil {
					beszelProxy.ServeHTTP(w, r)
					return
				}
			}

			// Determine preferred upstream from Referer or cached Referer owner
			preferKuma := false
			if ref := r.Header.Get("Referer"); ref != "" {
				if ru, err := url.Parse(ref); err == nil {
					if strings.HasPrefix(ru.Path, "/status") {
						preferKuma = true
					} else if prevOwner, ok := assetOwner.Load(ru.Path); ok && prevOwner == "kuma" {
						preferKuma = true
					}
				}
			}

			reqURI := r.URL.RequestURI()
			if preferKuma {
				if kumaProxy != nil && probeUpstreamExists(probeClient, kumaTarget, reqURI) {
					assetOwner.Store(p, "kuma")
					kumaProxy.ServeHTTP(w, r)
					return
				}
				if beszelProxy != nil && probeUpstreamExists(probeClient, beszelTarget, reqURI) {
					assetOwner.Store(p, "beszel")
					beszelProxy.ServeHTTP(w, r)
					return
				}
			} else {
				if beszelProxy != nil && probeUpstreamExists(probeClient, beszelTarget, reqURI) {
					assetOwner.Store(p, "beszel")
					beszelProxy.ServeHTTP(w, r)
					return
				}
				if kumaProxy != nil && probeUpstreamExists(probeClient, kumaTarget, reqURI) {
					assetOwner.Store(p, "kuma")
					kumaProxy.ServeHTTP(w, r)
					return
				}
			}
		}

		http.NotFound(w, r)
	})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("NOC Live Single-Binary Server listening on %s (host=%s, proc=%s, docker=%s, embedProxy=%v)",
		cfg.ListenAddr, cfg.HostIP, cfg.ProcDir, cfg.DockerSocket, cfg.EnableEmbedProxy)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
