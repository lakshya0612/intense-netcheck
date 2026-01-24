# Intenseye Network Diagnostics CLI

A command-line tool to test network connectivity from your Sentinel device or customer network to all Intenseye Cloud endpoints. This tool is part of the Intenseye Deployment Readiness toolkit.

## Features

- **Deployment-aware testing** - Select Sentinel, Cloud (Iris), or Hybrid On-Premise deployment
- **TCP/HTTPS/UDP connectivity tests** - Tests all required endpoints including non-HTTP protocols
- **ICMP ping tests** - Measures latency and packet loss
- **TLS certificate validation** - Checks cert expiry
- **Traceroute** - Diagnose failed connections
- **Region filtering** - Test only relevant endpoints
- **Firewall rules export** - Generate rules for iptables, Windows Firewall, or plain text
- **JSON output** - For automation and integration

## Installation

### Download Pre-built Binaries

Download the appropriate binary for your platform:

- **Linux (amd64)**: `intenseye-netcheck-linux-amd64`
- **Linux (arm64)**: `intenseye-netcheck-linux-arm64`
- **macOS (amd64)**: `intenseye-netcheck-darwin-amd64`
- **macOS (arm64)**: `intenseye-netcheck-darwin-arm64`
- **Windows**: `intenseye-netcheck-windows-amd64.exe`

### Build from Source

```bash
# Requires Go 1.21+
cd cli
go build -o intenseye-netcheck .
```

## Usage

```bash
# Run tests for Sentinel deployment (default)
./intenseye-netcheck

# Run tests for specific deployment type
./intenseye-netcheck --deployment sentinel
./intenseye-netcheck --deployment cloud
./intenseye-netcheck --deployment hybrid

# List available deployment types
./intenseye-netcheck --list-deployments

# Filter by region
./intenseye-netcheck --deployment hybrid --region americas
./intenseye-netcheck --deployment sentinel --region emea

# Export firewall rules
./intenseye-netcheck --deployment hybrid --export-firewall iptables
./intenseye-netcheck --deployment hybrid --export-firewall windows
./intenseye-netcheck --deployment hybrid --export-firewall text

# Skip ping tests (faster)
./intenseye-netcheck --ping=false

# Run traceroute for failed endpoints
./intenseye-netcheck --traceroute

# Output as JSON (for automation)
./intenseye-netcheck --json

# Custom timeout
./intenseye-netcheck --timeout 15s

# Adjust concurrency
./intenseye-netcheck --concurrent 10
```

## Options

| Flag | Default | Description |
|------|---------|-------------|
| `--deployment` | `sentinel` | Deployment type: `sentinel`, `cloud`, `hybrid` |
| `--region` | `all` | Filter endpoints: `all`, `americas`, `emea`, `apac` |
| `--timeout` | `10s` | Connection timeout per endpoint |
| `--ping` | `true` | Run ICMP ping tests |
| `--traceroute` | `false` | Run traceroute for failed endpoints |
| `--json` | `false` | Output results as JSON |
| `--concurrent` | `5` | Number of concurrent tests |
| `--export-firewall` | | Export firewall rules: `iptables`, `windows`, `text` |
| `--list-deployments` | `false` | List available deployment types |

## Example Output

```
  ╦┌┐┌┌┬┐┌─┐┌┐┌┌─┐┌─┐┬ ┬┌─┐
  ║│││ │ ├┤ │││└─┐├┤ └┬┘├┤ 
  ╩┘└┘ ┴ └─┘┘└┘└─┘└─┘ ┴ └─┘
  Network Diagnostics Tool v1.0

Running tests for 24 endpoints (region: all)

=== Core API ===
  ✓ PASS Intenseye API              api.intenseye.com:443  45ms | ping: 32ms
  ✓ PASS Temporal Workflow          temporal.intenseye.com:443  52ms | ping: 35ms
  ✓ PASS Web Dashboard              dashboard.intenseye.com:443  48ms | ping: 33ms

=== Telemetry ===
  ✓ PASS Pulsar Event Transport     pulsar.intenseye.com:6651  67ms | ping: 45ms
  ✓ PASS NATS Messaging             nats.intenseye.com:443  42ms | ping: 30ms

=== Video ===
  ✓ PASS Video EU01                 eu01live.intenseye.com:32401  55ms | ping: 38ms
  ✗ FAIL Video EU02                 eu02live.intenseye.com:32402  timeout

=== SUMMARY ===
  Total:  24
  Passed: 23
  Failed: 1

Note: Run with --traceroute flag to diagnose failed connections
```

## Endpoints Tested

| Category | Endpoints |
|----------|-----------|
| Core API | api.intenseye.com, temporal.intenseye.com, dashboard.intenseye.com |
| Video | Regional video ingest servers (us01/02, eu01/02/03, as01/02) |
| Telemetry | pulsar.intenseye.com:6651, nats.intenseye.com |
| Updates | storage.googleapis.com, gcr.io, S3 buckets |
| Device Management | Balena Cloud services |
| Auth | auth0.com, LaunchDarkly |

## Running on Sentinel

SSH into your Sentinel device and run:

```bash
# Download (Linux ARM64 for Sentinel Hub)
curl -LO https://github.com/intenseye/netcheck/releases/latest/download/intenseye-netcheck-linux-arm64
chmod +x intenseye-netcheck-linux-arm64

# Run tests
./intenseye-netcheck-linux-arm64 --region emea
```

## Troubleshooting

### Permission Denied for Ping
ICMP ping requires elevated privileges on some systems:
```bash
sudo ./intenseye-netcheck
```

### Firewall Blocking Ports
If endpoints fail, check your firewall allows:
- TCP 443 (HTTPS)
- TCP 6651 (Pulsar)
- TCP 32401-32403 (Video ingest)

### DNS Resolution Failed
Ensure DNS is configured correctly:
```bash
nslookup api.intenseye.com
```
