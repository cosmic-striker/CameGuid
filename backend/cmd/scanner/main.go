package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"camguid/internal/config"
	"camguid/internal/database"
	"camguid/internal/middleware"
	"camguid/internal/pkg/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type ScanRequest struct {
	ScanType   string `json:"scan_type"`   // full, custom, quick
	Target     string `json:"target"`      // CIDR or "auto"
	StartIP    string `json:"start_ip"`    // Custom range start
	EndIP      string `json:"end_ip"`      // Custom range end
	Ports      string `json:"ports"`       // Comma-separated ports
	AutoImport bool   `json:"auto_import"` // Automatically add to registry
	JobID      string `json:"job_id"`
	UserID     string `json:"user_id"`
}

type DiscoveredDevice struct {
	IPAddress   string  `json:"ip_address"`
	MACAddress  *string `json:"mac_address,omitempty"`
	Hostname    *string `json:"hostname,omitempty"`
	Vendor      *string `json:"vendor,omitempty"`
	OpenPorts   []int   `json:"open_ports"`
	DeviceType  string  `json:"device_type"`
	IsReachable bool    `json:"is_reachable"`
	RTSPFound   bool    `json:"rtsp_found"`
	ONVIFFound  bool    `json:"onvif_found"`
	WebFound    bool    `json:"web_found"`
	WebTitle    *string `json:"web_title,omitempty"`
	Confidence  float32 `json:"confidence"`
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(cfg.LogLevel)
	log.Info().Str("port", cfg.ScannerPort).Msg("Starting CamGuid Scanner service")

	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer pool.Close()

	// Redis for pub/sub scan progress events
	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse Redis URL")
	}
	rdb := redis.NewClient(opts)
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}

	scanner := &Scanner{pool: pool, rdb: rdb, cfg: cfg}

	app := fiber.New(fiber.Config{AppName: "CamGuid Scanner"})
	app.Use(cors.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "camguid-scanner"})
	})

	// All scan endpoints require a valid JWT (admin/operator) — minted by the
	// API and forwarded to this service on port 8081 by nginx.
	perm := middleware.AuthMiddleware(cfg.JWTSecret)
	role := middleware.RequireRole("admin", "operator")

	// Abuse throttling: limited concurrent scans + minimum interval between
	// scans for the same user (enforced via Redis).
	maxConcurrent := cfg.ScanMaxConcurrent
	minInterval := time.Duration(cfg.ScanMinInterval) * time.Second
	throttle := func(c *fiber.Ctx) error {
		var running int
		scanner.activeJobs.Range(func(_, _ any) bool {
			running++
			return true
		})
		if running >= maxConcurrent {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Maximum concurrent scans already running",
			})
		}
		if minInterval > 0 {
			userID := middleware.GetUserID(c)
			key := "scan:last:" + userID.String()
			ok, err := rdb.SetNX(ctx, key, time.Now().Unix(), minInterval).Result()
			if err == nil && !ok {
				return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
					"error": fmt.Sprintf("Please wait %d seconds between scans", int(minInterval.Seconds())),
				})
			}
		}
		return c.Next()
	}

	scanGroup := app.Group("/", perm, role)

	scanGroup.Post("/scan", throttle, func(c *fiber.Ctx) error {
		var req ScanRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
		}
		if req.ScanType == "" {
			req.ScanType = "full"
		}

		jobID := req.JobID
		if jobID == "" {
			jobID = uuid.New().String()
		}

		// Start scan in background with cancellation support
		jobCtx, cancel := context.WithCancel(context.Background())
		scanner.activeJobs.Store(jobID, cancel)

		go func() {
			defer scanner.activeJobs.Delete(jobID)
			scanner.RunScan(jobCtx, jobID, req)
		}()

		return c.JSON(fiber.Map{
			"job_id":  jobID,
			"status":  "started",
			"message": "Scan started in background",
		})
	})

	// Cancel a running scan
	scanGroup.Post("/scan/:id/cancel", func(c *fiber.Ctx) error {
		jobID := c.Params("id")
		if cancel, ok := scanner.activeJobs.Load(jobID); ok {
			cancel.(context.CancelFunc)()
			scanner.activeJobs.Delete(jobID)

			// Mark the job as cancelled in the DB
			pool.Exec(context.Background(),
				`UPDATE scan_jobs SET status = 'cancelled', completed_at = now() WHERE id = $1 AND status = 'running'`,
				jobID)

			return c.JSON(fiber.Map{"status": "cancelled", "job_id": jobID})
		}
		return c.Status(404).JSON(fiber.Map{"error": "No running scan with that ID"})
	})

	scanGroup.Get("/scan", func(c *fiber.Ctx) error {
		rows, err := pool.Query(ctx,
			`SELECT id, status, scan_type, target, device_count, started_at, completed_at, created_at
			 FROM scan_jobs ORDER BY created_at DESC LIMIT 50`)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to query scans"})
		}
		defer rows.Close()

		scans := make([]fiber.Map, 0)
		for rows.Next() {
			var id, status, scanType, target string
			var deviceCount int
			var startedAt, completedAt, createdAt *time.Time
			if err := rows.Scan(&id, &status, &scanType, &target, &deviceCount, &startedAt, &completedAt, &createdAt); err == nil {
				scans = append(scans, fiber.Map{
					"id": id, "status": status, "scan_type": scanType, "target": target,
					"device_count": deviceCount, "started_at": startedAt, "completed_at": completedAt, "created_at": createdAt,
				})
			}
		}
		return c.JSON(fiber.Map{"data": scans})
	})

	scanGroup.Get("/scan/:id/status", func(c *fiber.Ctx) error {
		jobID := c.Params("id")
		var status, scanType, target string
		var deviceCount int
		var startedAt, completedAt *time.Time

		err := pool.QueryRow(ctx,
			`SELECT status, scan_type, target, device_count, started_at, completed_at
			 FROM scan_jobs WHERE id = $1`, jobID,
		).Scan(&status, &scanType, &target, &deviceCount, &startedAt, &completedAt)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Scan job not found"})
		}

		// Also fetch discovered devices
		rows, err := pool.Query(ctx,
			`SELECT ip_address, mac_address, hostname, vendor, device_type, open_ports, rtsp_found, confidence
			 FROM discovered_devices WHERE scan_job_id = $1`, jobID)
		devices := make([]fiber.Map, 0)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var ip string
				var mac, host, vendor *string
				var devType string
				var ports []int
				var rtsp bool
				var conf float32
				if err := rows.Scan(&ip, &mac, &host, &vendor, &devType, &ports, &rtsp, &conf); err == nil {
					devices = append(devices, fiber.Map{
						"ip_address": ip, "mac_address": mac, "hostname": host,
						"vendor": vendor, "device_type": devType, "open_ports": ports,
						"rtsp_found": rtsp, "confidence": conf,
					})
				}
			}
		}

		return c.JSON(fiber.Map{
			"id":                 jobID,
			"status":             status,
			"scan_type":          scanType,
			"target":             target,
			"device_count":       deviceCount,
			"started_at":         startedAt,
			"completed_at":       completedAt,
			"discovered_devices": devices,
		})
	})

	go func() {
		if err := app.Listen(":" + cfg.ScannerPort); err != nil {
			log.Fatal().Err(err).Msg("Scanner server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down scanner service...")

	// Cancel all active scan jobs
	scanner.activeJobs.Range(func(key, value any) bool {
		value.(context.CancelFunc)()
		return true
	})

	app.Shutdown()
}

type Scanner struct {
	pool       *pgxpool.Pool
	rdb        *redis.Client
	cfg        *config.Config
	activeJobs sync.Map // map[string]context.CancelFunc
}

func (s *Scanner) RunScan(ctx context.Context, jobID string, req ScanRequest) {
	log := logger.New(s.cfg.LogLevel)
	log.Info().Str("job_id", jobID).Str("type", req.ScanType).Msg("Starting scan")

	var targetStr = req.Target
	if req.StartIP != "" && req.EndIP != "" {
		targetStr = req.StartIP + "-" + req.EndIP
	} else if targetStr == "" || targetStr == "auto" {
		targetStr = detectLocalSubnet()
	}

	// Insert job into database
	_, err := s.pool.Exec(ctx,
		`INSERT INTO scan_jobs (id, status, scan_type, target, started_at)
		 VALUES ($1, 'running', $2, $3, now())
		 ON CONFLICT (id) DO UPDATE SET status = 'running', started_at = now()`,
		jobID, req.ScanType, targetStr)

	if err != nil {
		log.Error().Err(err).Msg("Failed to insert/update scan job")
	}

	// Helper to mark scan as failed on errors
	markFailed := func(reason string) {
		s.pool.Exec(context.Background(),
			`UPDATE scan_jobs SET status = 'failed', completed_at = now(), error = $1 WHERE id = $2`,
			reason, jobID)
		evtData, _ := json.Marshal(fiber.Map{"event": "error", "data": fiber.Map{"message": reason}})
		s.rdb.Publish(context.Background(), "scan:"+jobID, string(evtData))
		log.Error().Str("job_id", jobID).Msg(reason)
	}

	// Check cancellation
	select {
	case <-ctx.Done():
		log.Info().Str("job_id", jobID).Msg("Scan job cancelled before start")
		return
	default:
	}

	// Determine target IPs
	var allIPs []string
	if req.StartIP != "" && req.EndIP != "" {
		allIPs = generateIPRange(req.StartIP, req.EndIP)
	} else {
		target := req.Target
		if target == "" || target == "auto" {
			target = detectLocalSubnet()
		}
		allIPs = generateCIDRIPs(target)
	}

	if len(allIPs) == 0 {
		markFailed("No valid IPs to scan for target: " + targetStr)
		return
	}

	// Determine ports to scan
	commonPorts := []int{22, 53, 80, 161, 443, 554, 3389, 5000, 8080, 8443, 8554, 34567, 37777}
	if req.Ports != "" {
		commonPorts = parsePorts(req.Ports)
	}

	// Publish progress: host discovery starting
	s.publishProgress(ctx, jobID, 0, len(allIPs))

	// Phase 1: Discover live hosts via TCP ping (port 80/554/443)
	hosts := s.discoverHosts(ctx, allIPs, jobID)

	// Check cancellation after discovery
	select {
	case <-ctx.Done():
		s.pool.Exec(context.Background(),
			`UPDATE scan_jobs SET status = 'cancelled', completed_at = now() WHERE id = $1`, jobID)
		log.Info().Str("job_id", jobID).Msg("Scan cancelled during host discovery")
		return
	default:
	}

	// Phase 2: Port scan and fingerprint each host
	var devices []DiscoveredDevice
	var mu sync.Mutex
	var wg sync.WaitGroup
	var scannedCount int64

	concurrency := s.cfg.ScannerConcurrency
	if concurrency <= 0 {
		concurrency = 50
	}
	sem := make(chan struct{}, concurrency)

	for _, host := range hosts {
		// Check cancellation before each host
		select {
		case <-ctx.Done():
			break
		default:
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()

			// Check cancellation in goroutine
			select {
			case <-ctx.Done():
				return
			default:
			}

			dev := s.scanHost(ctx, ip, commonPorts)
			mu.Lock()
			devices = append(devices, dev)
			scannedCount++
			count := scannedCount
			mu.Unlock()

			// Publish progress event
			evtData, _ := json.Marshal(fiber.Map{
				"event": "device_found",
				"data":  dev,
			})
			s.rdb.Publish(ctx, "scan:"+jobID, string(evtData))
			s.publishProgress(ctx, jobID, int(count), len(hosts))
		}(host)
	}
	wg.Wait()

	// Check cancellation after scanning
	select {
	case <-ctx.Done():
		s.pool.Exec(context.Background(),
			`UPDATE scan_jobs SET status = 'cancelled', completed_at = now(), device_count = $1 WHERE id = $2`,
			len(devices), jobID)
		log.Info().Str("job_id", jobID).Msg("Scan cancelled during port scanning")
		return
	default:
	}

	// Phase 3: Save discovered devices to database
	for _, dev := range devices {
		if err := s.saveDiscoveredDevice(ctx, jobID, dev); err != nil {
			log.Warn().Err(err).Str("ip", dev.IPAddress).Msg("Failed to save discovered device")
		}

		if req.AutoImport && (dev.DeviceType == "camera" || dev.DeviceType == "nvr") {
			name := dev.IPAddress
			if dev.Hostname != nil && *dev.Hostname != "" {
				name = *dev.Hostname
			} else if dev.Vendor != nil && *dev.Vendor != "" {
				name = *dev.Vendor + " Device"
			}

			_, err := s.pool.Exec(ctx,
				`INSERT INTO devices (id, name, device_type, ip_address, manufacturer)
				 SELECT $1, $2, $3, $4::inet, $5
				 WHERE NOT EXISTS (SELECT 1 FROM devices WHERE ip_address = $4::inet)`,
				uuid.New(), name, dev.DeviceType, dev.IPAddress, dev.Vendor)
			if err != nil {
				log.Error().Err(err).Str("ip", dev.IPAddress).Msg("Auto-import failed")
			}
		}
	}

	// Update job as completed
	_, err = s.pool.Exec(ctx,
		`UPDATE scan_jobs SET status = 'completed', completed_at = now(), device_count = $1 WHERE id = $2`,
		len(devices), jobID)
	if err != nil {
		log.Error().Err(err).Str("job_id", jobID).Msg("Failed to mark scan as completed")
	}

	// Publish completion event
	evtData, _ := json.Marshal(fiber.Map{
		"event": "completed",
		"data": fiber.Map{
			"total_devices": len(devices),
		},
	})
	s.rdb.Publish(ctx, "scan:"+jobID, string(evtData))

	log.Info().Int("devices", len(devices)).Str("job_id", jobID).Msg("Scan completed")
}

func (s *Scanner) publishProgress(ctx context.Context, jobID string, scanned, total int) {
	if total == 0 {
		return
	}
	percent := float64(scanned) / float64(total) * 100
	evtData, _ := json.Marshal(fiber.Map{
		"event": "progress",
		"data": fiber.Map{
			"scanned": scanned,
			"total":   total,
			"percent": percent,
		},
	})
	s.rdb.Publish(ctx, "scan:"+jobID, string(evtData))
}

func (s *Scanner) discoverHosts(ctx context.Context, allIPs []string, jobID string) []string {
	var hosts []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Use worker pool for host discovery
	workerCount := s.cfg.ScannerConcurrency
	if workerCount <= 0 {
		workerCount = 50
	}
	if workerCount > len(allIPs) {
		workerCount = len(allIPs)
	}

	ipChan := make(chan string, len(allIPs))
	for _, ip := range allIPs {
		ipChan <- ip
	}
	close(ipChan)

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case ip, ok := <-ipChan:
					if !ok {
						return
					}
					if isHostAlive(ip) {
						mu.Lock()
						hosts = append(hosts, ip)
						mu.Unlock()
					}
				}
			}
		}()
	}
	wg.Wait()

	return hosts
}

func (s *Scanner) scanHost(ctx context.Context, ip string, portsToScan []int) DiscoveredDevice {
	dev := DiscoveredDevice{
		IPAddress:   ip,
		IsReachable: true,
		DeviceType:  "unknown",
		OpenPorts:   make([]int, 0),
		Confidence:  0.0,
	}

	// Scan provided ports
	for _, port := range portsToScan {
		// Check context cancellation per-port
		select {
		case <-ctx.Done():
			return dev
		default:
		}

		addr := net.JoinHostPort(ip, strconv.Itoa(port))
		conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
		if err == nil {
			conn.Close()
			dev.OpenPorts = append(dev.OpenPorts, port)
		}
	}

	// Check specific services
	for _, p := range dev.OpenPorts {
		if p == 554 || p == 8554 {
			dev.RTSPFound = true
		}
		if p == 80 || p == 443 || p == 8080 {
			dev.WebFound = true
		}
	}

	// DNS reverse lookup
	names, err := net.LookupAddr(ip)
	if err == nil && len(names) > 0 {
		hostname := strings.TrimSuffix(names[0], ".")
		dev.Hostname = &hostname
	}

	// Classify device
	dev.DeviceType, dev.Confidence = classifyDevice(dev)

	return dev
}

func classifyDevice(d DiscoveredDevice) (string, float32) {
	scores := map[string]float32{
		"camera":  0,
		"nvr":     0,
		"switch":  0,
		"router":  0,
		"pc":      0,
		"server":  0,
		"unknown": 0.1,
	}

	for _, p := range d.OpenPorts {
		switch p {
		case 554, 8554:
			scores["camera"] += 0.4
			scores["nvr"] += 0.3
		case 37777:
			scores["nvr"] += 0.5
		case 34567:
			scores["nvr"] += 0.5
		case 22:
			scores["pc"] += 0.2
			scores["server"] += 0.3
		case 53:
			scores["router"] += 0.5
		case 161:
			scores["switch"] += 0.3
			scores["router"] += 0.2
		case 3389:
			scores["pc"] += 0.4
		case 80, 443:
			// common to many devices — minimal scoring
		case 8080:
			scores["camera"] += 0.1
			scores["nvr"] += 0.1
		}
	}

	if d.RTSPFound {
		scores["camera"] += 0.3
	}
	if d.ONVIFFound {
		scores["camera"] += 0.4
		scores["nvr"] += 0.2
	}

	// Find highest scoring type
	bestType := "unknown"
	bestScore := float32(0)
	for t, s := range scores {
		if s > bestScore {
			bestType = t
			bestScore = s
		}
	}

	return bestType, bestScore
}

func (s *Scanner) saveDiscoveredDevice(ctx context.Context, jobID string, dev DiscoveredDevice) error {
	fingerprint, _ := json.Marshal(dev)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO discovered_devices
			(id, scan_job_id, ip_address, mac_address, hostname, vendor, open_ports,
			 device_type, is_reachable, rtsp_found, onvif_found, web_found, web_title, fingerprint, confidence)
		 VALUES ($1, $2, $3::inet, $4::macaddr, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		uuid.New(), jobID, dev.IPAddress, dev.MACAddress, dev.Hostname, dev.Vendor,
		dev.OpenPorts, dev.DeviceType, dev.IsReachable, dev.RTSPFound, dev.ONVIFFound,
		dev.WebFound, dev.WebTitle, string(fingerprint), dev.Confidence,
	)
	return err
}

func isHostAlive(ip string) bool {
	ports := []int{80, 554, 443, 8080, 22}
	for _, port := range ports {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
	}
	return false
}

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func detectLocalSubnet() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "192.168.1.0/24"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			ip := ipnet.IP.To4()
			mask := ipnet.Mask
			network := net.IP(make([]byte, 4))
			for i := range ip {
				network[i] = ip[i] & mask[i]
			}
			ones, _ := mask.Size()
			return fmt.Sprintf("%s/%d", network.String(), ones)
		}
	}
	return "192.168.1.0/24"
}

func generateCIDRIPs(cidr string) []string {
	var allIPs []string
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return allIPs
	}

	// Copy the starting IP to avoid modifying the original
	start := ip.Mask(ipnet.Mask)
	cur := make(net.IP, len(start))
	copy(cur, start)

	for ipnet.Contains(cur) {
		// Copy each IP to avoid mutation
		temp := make(net.IP, len(cur))
		copy(temp, cur)
		allIPs = append(allIPs, temp.String())
		incrementIP(cur)
	}

	// Exclude network and broadcast addresses for typical subnets
	if len(allIPs) > 2 {
		return allIPs[1 : len(allIPs)-1]
	}
	return allIPs
}

func generateIPRange(startIP, endIP string) []string {
	var ips []string
	start := net.ParseIP(startIP)
	end := net.ParseIP(endIP)
	if start == nil || end == nil {
		return ips
	}
	start = start.To4()
	end = end.To4()
	if start == nil || end == nil {
		return ips
	}

	// Work on a copy to avoid corrupting the parsed IP
	cur := make(net.IP, len(start))
	copy(cur, start)

	for bytes.Compare(cur, end) <= 0 {
		temp := make(net.IP, len(cur))
		copy(temp, cur)
		ips = append(ips, temp.String())
		incrementIP(cur)
	}
	return ips
}

func parsePorts(portStr string) []int {
	var ports []int
	parts := strings.Split(portStr, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if port, err := strconv.Atoi(p); err == nil && port > 0 && port <= 65535 {
			ports = append(ports, port)
		}
	}
	if len(ports) == 0 {
		return []int{80, 554, 443, 8080} // fallback
	}
	return ports
}
