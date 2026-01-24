package main

import (
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed endpoints.json
var embeddedEndpointsJSON []byte

type Endpoint struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Protocol    string `json:"protocol"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Region      string `json:"region"`
}

type DeploymentConfig struct {
	Label       string     `json:"label"`
	Description string     `json:"description"`
	Endpoints   []Endpoint `json:"endpoints"`
}

type EndpointsConfig struct {
	Version     string                      `json:"version"`
	Generated   string                      `json:"generated"`
	Deployments map[string]DeploymentConfig `json:"deployments"`
}

type TestResult struct {
	Endpoint    Endpoint
	Reachable   bool
	Latency     time.Duration
	Error       string
	Method      string
	TLSValid    bool
	TLSExpiry   time.Time
	PingLatency time.Duration
	PingLoss    float64
}

var endpointsConfig EndpointsConfig

func loadEndpoints() error {
	return json.Unmarshal(embeddedEndpointsJSON, &endpointsConfig)
}

func getEndpointsForDeployment(deployment string) []Endpoint {
	if config, ok := endpointsConfig.Deployments[deployment]; ok {
		return config.Endpoints
	}
	// Default to sentinel if deployment not found
	if config, ok := endpointsConfig.Deployments["sentinel"]; ok {
		return config.Endpoints
	}
	return nil
}

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

func main() {
	deployment := flag.String("deployment", "sentinel", "Deployment type: sentinel, cloud, hybrid")
	region := flag.String("region", "all", "Filter by region: all, americas, emea, apac")
	timeout := flag.Duration("timeout", 10*time.Second, "Connection timeout")
	ping := flag.Bool("ping", true, "Run ICMP ping tests")
	jsonOutput := flag.Bool("json", false, "Output results as JSON")
	traceroute := flag.Bool("traceroute", false, "Run traceroute to failed endpoints")
	concurrent := flag.Int("concurrent", 5, "Number of concurrent tests")
	exportFirewall := flag.String("export-firewall", "", "Export firewall rules: iptables, windows, text")
	listDeployments := flag.Bool("list-deployments", false, "List available deployment types")
	flag.Parse()

	// Load endpoints from embedded config
	if err := loadEndpoints(); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading endpoints: %v\n", err)
		os.Exit(1)
	}

	// List deployments and exit
	if *listDeployments {
		fmt.Println("Available deployment types:")
		for name, config := range endpointsConfig.Deployments {
			fmt.Printf("  %s - %s (%d endpoints)\n", name, config.Description, len(config.Endpoints))
		}
		return
	}

	// Get endpoints for deployment type
	endpoints := getEndpointsForDeployment(*deployment)
	if endpoints == nil {
		fmt.Fprintf(os.Stderr, "Unknown deployment type: %s\n", *deployment)
		fmt.Fprintf(os.Stderr, "Available types: sentinel, cloud, hybrid\n")
		os.Exit(1)
	}

	// Export firewall rules if requested
	if *exportFirewall != "" {
		exportFirewallRules(endpoints, *exportFirewall, *deployment)
		return
	}

	if !*jsonOutput {
		printBanner()
	}

	// Filter endpoints by region
	filtered := filterEndpoints(endpoints, *region)

	if !*jsonOutput {
		deploymentLabel := endpointsConfig.Deployments[*deployment].Label
		fmt.Printf("%sDeployment: %s%s\n", colorCyan, deploymentLabel, colorReset)
		fmt.Printf("%sRunning tests for %d endpoints (region: %s)%s\n\n", colorCyan, len(filtered), *region, colorReset)
	}

	// Run tests
	results := runTests(filtered, *timeout, *ping, *concurrent)

	// Output results
	if *jsonOutput {
		outputJSON(results)
	} else {
		outputTable(results)

		// Run traceroute for failed endpoints if requested
		if *traceroute {
			runTraceroutes(results)
		}

		printSummary(results)
	}
}

func printBanner() {
	fmt.Println()
	fmt.Printf("%s%s", colorBold, colorCyan)
	fmt.Println("  ╦┌┐┌┌┬┐┌─┐┌┐┌┌─┐┌─┐┬ ┬┌─┐")
	fmt.Println("  ║│││ │ ├┤ │││└─┐├┤ └┬┘├┤ ")
	fmt.Println("  ╩┘└┘ ┴ └─┘┘└┘└─┘└─┘ ┴ └─┘")
	fmt.Printf("%s", colorReset)
	fmt.Println("  Network Diagnostics Tool v1.0")
	fmt.Println()
}

func filterEndpoints(endpoints []Endpoint, region string) []Endpoint {
	if region == "all" {
		return endpoints
	}

	var filtered []Endpoint
	for _, e := range endpoints {
		if e.Region == "all" || e.Region == region {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

func exportFirewallRules(endpoints []Endpoint, format string, deployment string) {
	switch format {
	case "iptables":
		fmt.Println("# Intenseye Firewall Rules - iptables")
		fmt.Printf("# Deployment: %s\n", deployment)
		fmt.Println("# Run these commands as root or with sudo")
		fmt.Println()
		for _, ep := range endpoints {
			protocol := "tcp"
			if ep.Protocol == "udp" {
				protocol = "udp"
			}
			fmt.Printf("# %s\n", ep.Label)
			fmt.Printf("iptables -A OUTPUT -p %s -d %s --dport %d -j ACCEPT\n", protocol, ep.Host, ep.Port)
		}
		fmt.Println()
		fmt.Println("# Save rules (Debian/Ubuntu)")
		fmt.Println("# iptables-save > /etc/iptables/rules.v4")

	case "windows":
		fmt.Println("# Intenseye Firewall Rules - Windows PowerShell")
		fmt.Printf("# Deployment: %s\n", deployment)
		fmt.Println("# Run PowerShell as Administrator")
		fmt.Println()
		for _, ep := range endpoints {
			ruleName := strings.ReplaceAll(ep.Host, ".", "-")
			protocol := "TCP"
			if ep.Protocol == "udp" {
				protocol = "UDP"
			}
			fmt.Printf("# %s\n", ep.Label)
			fmt.Printf("New-NetFirewallRule -DisplayName \"Intenseye-%s-%d\" -Direction Outbound -Protocol %s -RemoteAddress %s -RemotePort %d -Action Allow\n\n",
				ruleName, ep.Port, protocol, ep.Host, ep.Port)
		}

	case "text":
		fmt.Println("Intenseye Required Endpoints")
		fmt.Printf("Deployment: %s\n", deployment)
		fmt.Println("===========================")
		fmt.Println()
		fmt.Println("The following outbound connections must be allowed:")
		fmt.Println()

		// Group by port
		byPort := make(map[int][]Endpoint)
		for _, ep := range endpoints {
			byPort[ep.Port] = append(byPort[ep.Port], ep)
		}

		ports := make([]int, 0, len(byPort))
		for port := range byPort {
			ports = append(ports, port)
		}
		sort.Ints(ports)

		for _, port := range ports {
			eps := byPort[port]
			protocol := "TCP"
			if eps[0].Protocol == "udp" {
				protocol = "UDP"
			}
			fmt.Printf("Port %d (%s):\n", port, protocol)
			for _, ep := range eps {
				fmt.Printf("  - %s\n", ep.Host)
			}
			fmt.Println()
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown firewall format: %s\n", format)
		fmt.Fprintf(os.Stderr, "Available formats: iptables, windows, text\n")
		os.Exit(1)
	}
}

func runTests(endpoints []Endpoint, timeout time.Duration, runPing bool, concurrent int) []TestResult {
	results := make([]TestResult, len(endpoints))
	sem := make(chan struct{}, concurrent)
	var wg sync.WaitGroup

	for i, endpoint := range endpoints {
		wg.Add(1)
		go func(idx int, ep Endpoint) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := testEndpoint(ep, timeout)

			if runPing {
				pingResult := runICMPPing(ep.Host)
				result.PingLatency = pingResult.latency
				result.PingLoss = pingResult.loss
			}

			results[idx] = result
		}(i, endpoint)
	}

	wg.Wait()
	return results
}

func testEndpoint(ep Endpoint, timeout time.Duration) TestResult {
	result := TestResult{Endpoint: ep}
	start := time.Now()

	switch ep.Protocol {
	case "tcp", "other":
		result = testTCP(ep, timeout)
	case "https", "wss":
		result = testHTTPS(ep, timeout)
	default:
		result = testTCP(ep, timeout)
	}

	result.Latency = time.Since(start)
	return result
}

func testTCP(ep Endpoint, timeout time.Duration) TestResult {
	result := TestResult{Endpoint: ep, Method: "TCP"}
	addr := fmt.Sprintf("%s:%d", ep.Host, ep.Port)

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		result.Reachable = false
		result.Error = err.Error()
		return result
	}
	defer conn.Close()

	result.Reachable = true

	// If it's a TLS port, check certificate
	if ep.Port == 443 || ep.Port == 6651 {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         ep.Host,
			InsecureSkipVerify: false,
		})
		if err := tlsConn.Handshake(); err == nil {
			result.TLSValid = true
			certs := tlsConn.ConnectionState().PeerCertificates
			if len(certs) > 0 {
				result.TLSExpiry = certs[0].NotAfter
			}
		}
	}

	return result
}

func testHTTPS(ep Endpoint, timeout time.Duration) TestResult {
	result := TestResult{Endpoint: ep, Method: "HTTPS"}

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
		},
	}

	url := fmt.Sprintf("https://%s:%d/", ep.Host, ep.Port)
	if ep.Protocol == "wss" {
		// For WebSocket, just test TCP/TLS connection
		return testTCP(ep, timeout)
	}

	req, _ := http.NewRequest("HEAD", url, nil)
	req.Header.Set("User-Agent", "Intenseye-NetCheck/1.0")

	resp, err := client.Do(req)
	if err != nil {
		// Try GET if HEAD fails
		req.Method = "GET"
		resp, err = client.Do(req)
		if err != nil {
			result.Reachable = false
			result.Error = simplifyError(err)
			return result
		}
	}
	defer resp.Body.Close()

	result.Reachable = true
	result.TLSValid = true

	// Get TLS certificate expiry
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		result.TLSExpiry = resp.TLS.PeerCertificates[0].NotAfter
	}

	return result
}

type pingResult struct {
	latency time.Duration
	loss    float64
}

func runICMPPing(host string) pingResult {
	result := pingResult{loss: 100}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("ping", "-n", "3", "-w", "2000", host)
	} else {
		cmd = exec.Command("ping", "-c", "3", "-W", "2", host)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)

	output, err := cmd.Output()
	if err != nil {
		return result
	}

	// Parse output to get latency and loss
	outputStr := string(output)

	// Parse packet loss
	if strings.Contains(outputStr, "0% packet loss") || strings.Contains(outputStr, "0.0% packet loss") || strings.Contains(outputStr, "(0% loss)") {
		result.loss = 0
	} else if strings.Contains(outputStr, "100% packet loss") || strings.Contains(outputStr, "100.0% packet loss") || strings.Contains(outputStr, "(100% loss)") {
		result.loss = 100
	} else {
		// Try to extract percentage
		for _, line := range strings.Split(outputStr, "\n") {
			if strings.Contains(line, "packet loss") || strings.Contains(line, "loss") {
				var loss float64
				fmt.Sscanf(line, "%f%% packet loss", &loss)
				if loss > 0 {
					result.loss = loss
				}
			}
		}
	}

	// Parse average latency
	for _, line := range strings.Split(outputStr, "\n") {
		if strings.Contains(line, "avg") || strings.Contains(line, "Average") {
			// macOS/Linux format: min/avg/max/stddev = 1.234/5.678/9.012/1.234 ms
			if idx := strings.Index(line, "="); idx != -1 {
				parts := strings.Split(strings.TrimSpace(line[idx+1:]), "/")
				if len(parts) >= 2 {
					var avg float64
					fmt.Sscanf(parts[1], "%f", &avg)
					result.latency = time.Duration(avg * float64(time.Millisecond))
				}
			}
			// Windows format: Average = 5ms
			if strings.Contains(line, "Average") {
				var avg int
				fmt.Sscanf(line, "Average = %dms", &avg)
				result.latency = time.Duration(avg) * time.Millisecond
			}
		}
	}

	return result
}

func runTraceroutes(results []TestResult) {
	fmt.Printf("\n%s%sRunning traceroute for failed endpoints...%s\n\n", colorBold, colorYellow, colorReset)

	for _, r := range results {
		if !r.Reachable {
			fmt.Printf("%sTraceroute to %s:%s\n", colorCyan, r.Endpoint.Host, colorReset)

			var cmd *exec.Cmd
			if runtime.GOOS == "windows" {
				cmd = exec.Command("tracert", "-d", "-h", "15", r.Endpoint.Host)
			} else {
				cmd = exec.Command("traceroute", "-n", "-m", "15", r.Endpoint.Host)
			}

			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()
			fmt.Println()
		}
	}
}

func outputTable(results []TestResult) {
	// Group by category
	categories := make(map[string][]TestResult)
	for _, r := range results {
		categories[r.Endpoint.Category] = append(categories[r.Endpoint.Category], r)
	}

	// Sort categories
	var catNames []string
	for cat := range categories {
		catNames = append(catNames, cat)
	}
	sort.Strings(catNames)

	for _, cat := range catNames {
		fmt.Printf("%s%s=== %s ===%s\n", colorBold, colorBlue, cat, colorReset)

		for _, r := range categories[cat] {
			status := fmt.Sprintf("%s✓ PASS%s", colorGreen, colorReset)
			if !r.Reachable {
				status = fmt.Sprintf("%s✗ FAIL%s", colorRed, colorReset)
			}

			latencyStr := fmt.Sprintf("%dms", r.Latency.Milliseconds())
			if r.Latency > 500*time.Millisecond {
				latencyStr = fmt.Sprintf("%s%dms%s", colorYellow, r.Latency.Milliseconds(), colorReset)
			}

			pingStr := ""
			if r.PingLatency > 0 {
				pingStr = fmt.Sprintf(" | ping: %dms", r.PingLatency.Milliseconds())
				if r.PingLoss > 0 {
					pingStr += fmt.Sprintf(" (%.0f%% loss)", r.PingLoss)
				}
			}

			fmt.Printf("  %s %-30s %s:%d  %s%s",
				status,
				r.Endpoint.Label,
				r.Endpoint.Host,
				r.Endpoint.Port,
				latencyStr,
				pingStr,
			)

			if !r.Reachable && r.Error != "" {
				fmt.Printf(" - %s%s%s", colorRed, r.Error, colorReset)
			}

			if r.TLSValid && !r.TLSExpiry.IsZero() {
				daysUntilExpiry := int(time.Until(r.TLSExpiry).Hours() / 24)
				if daysUntilExpiry < 30 {
					fmt.Printf(" %s[TLS expires in %d days]%s", colorYellow, daysUntilExpiry, colorReset)
				}
			}

			fmt.Println()
		}
		fmt.Println()
	}
}

func outputJSON(results []TestResult) {
	type jsonResult struct {
		ID          string  `json:"id"`
		Label       string  `json:"label"`
		Category    string  `json:"category"`
		Host        string  `json:"host"`
		Port        int     `json:"port"`
		Protocol    string  `json:"protocol"`
		Reachable   bool    `json:"reachable"`
		LatencyMs   int64   `json:"latency_ms"`
		Error       string  `json:"error,omitempty"`
		PingMs      int64   `json:"ping_ms,omitempty"`
		PingLoss    float64 `json:"ping_loss,omitempty"`
		TLSValid    bool    `json:"tls_valid,omitempty"`
		TLSExpiry   string  `json:"tls_expiry,omitempty"`
	}

	var jsonResults []jsonResult
	for _, r := range results {
		jr := jsonResult{
			ID:        r.Endpoint.ID,
			Label:     r.Endpoint.Label,
			Category:  r.Endpoint.Category,
			Host:      r.Endpoint.Host,
			Port:      r.Endpoint.Port,
			Protocol:  r.Endpoint.Protocol,
			Reachable: r.Reachable,
			LatencyMs: r.Latency.Milliseconds(),
			Error:     r.Error,
			PingMs:    r.PingLatency.Milliseconds(),
			PingLoss:  r.PingLoss,
			TLSValid:  r.TLSValid,
		}
		if !r.TLSExpiry.IsZero() {
			jr.TLSExpiry = r.TLSExpiry.Format(time.RFC3339)
		}
		jsonResults = append(jsonResults, jr)
	}

	output, _ := json.MarshalIndent(jsonResults, "", "  ")
	fmt.Println(string(output))
}

func printSummary(results []TestResult) {
	passed := 0
	failed := 0
	for _, r := range results {
		if r.Reachable {
			passed++
		} else {
			failed++
		}
	}

	fmt.Printf("%s%s=== SUMMARY ===%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("  Total:  %d\n", len(results))
	fmt.Printf("  %sPassed: %d%s\n", colorGreen, passed, colorReset)
	if failed > 0 {
		fmt.Printf("  %sFailed: %d%s\n", colorRed, failed, colorReset)
		fmt.Println()
		fmt.Printf("%sNote: Run with --traceroute flag to diagnose failed connections%s\n", colorYellow, colorReset)
	} else {
		fmt.Printf("  Failed: %d\n", failed)
		fmt.Println()
		fmt.Printf("%s✓ All endpoints are reachable from this network!%s\n", colorGreen, colorReset)
	}
	fmt.Println()
}

func simplifyError(err error) string {
	errStr := err.Error()
	if strings.Contains(errStr, "connection refused") {
		return "connection refused"
	}
	if strings.Contains(errStr, "no such host") {
		return "DNS resolution failed"
	}
	if strings.Contains(errStr, "i/o timeout") || strings.Contains(errStr, "deadline exceeded") {
		return "timeout"
	}
	if strings.Contains(errStr, "certificate") {
		return "TLS certificate error"
	}
	if strings.Contains(errStr, "connection reset") {
		return "connection reset"
	}
	// Truncate long errors
	if len(errStr) > 50 {
		return errStr[:50] + "..."
	}
	return errStr
}
