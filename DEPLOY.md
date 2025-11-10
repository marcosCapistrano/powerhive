# PowerHive Production Deployment Guide

Complete guide for deploying PowerHive to manage ~1000 ASIC mining machines in a datacenter environment.

## Table of Contents
- [Server Requirements](#server-requirements)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Deployment Options](#deployment-options)
- [Configuration](#configuration)
- [Data Persistence](#data-persistence)
- [Monitoring & Logs](#monitoring--logs)
- [Maintenance](#maintenance)
- [Troubleshooting](#troubleshooting)
- [Security Considerations](#security-considerations)

---

## Server Requirements

### Recommended Specifications (1000 Machines)
| Resource | Specification |
|----------|---------------|
| **OS** | Ubuntu Server 22.04 LTS or 24.04 LTS |
| **CPU** | 4 cores @ 2.5+ GHz (x86_64) |
| **RAM** | 2 GB |
| **Storage** | 200 GB SSD |
| **Network** | 100 Mbps sustained, 1 Gbps burst |

### Minimum Specifications (Budget Deployment)
| Resource | Specification |
|----------|---------------|
| **OS** | Ubuntu Server 22.04 LTS |
| **CPU** | 2 cores @ 2.0+ GHz |
| **RAM** | 1 GB |
| **Storage** | 100 GB SSD |
| **Network** | 50 Mbps sustained |

### Storage Growth Expectations
With optimized polling intervals (status: 60s, telemetry: 3600s):
- **Daily growth:** ~1.15 GB/day
- **Monthly growth:** ~34 GB/month
- **Annual growth:** ~413 GB/year
- **Recommended retention:** 30 days (~35 GB working set)

---

## Prerequisites

### 1. Install Docker
```bash
# Update package index
sudo apt-get update

# Install dependencies
sudo apt-get install -y ca-certificates curl gnupg lsb-release

# Add Docker's official GPG key
sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg

# Set up Docker repository
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

# Install Docker Engine
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

# Verify installation
sudo docker --version
sudo docker compose version
```

### 2. Configure Docker (Optional but Recommended)
```bash
# Add current user to docker group (avoid using sudo)
sudo usermod -aG docker $USER

# Log out and back in for group changes to take effect
# Or run: newgrp docker

# Enable Docker to start on boot
sudo systemctl enable docker
sudo systemctl start docker
```

### 3. Verify Network Access
Ensure the server can reach your miner network subnets:
```bash
# Test connectivity to a known miner IP
ping -c 3 192.168.1.100

# Verify port accessibility
nc -zv 192.168.1.100 80
```

---

## Quick Start

### Option 1: Deploy Directly from GitHub (Easiest for Server Admins)

**No repository cloning needed! Docker Compose will pull and build directly from GitHub.**

1. **Download just the docker-compose.yml file:**
   ```bash
   # Create deployment directory
   mkdir -p /opt/powerhive
   cd /opt/powerhive

   # Download docker-compose.yml
   wget https://raw.githubusercontent.com/YOUR_USERNAME/powerhive/main/docker-compose.yml

   # Or if you received it via email/file transfer
   # Just copy it to /opt/powerhive/docker-compose.yml
   ```

2. **Start the service:**
   ```bash
   docker compose up -d
   ```

   Docker will automatically:
   - Clone the repository from GitHub
   - Build the image using the Dockerfile
   - Start the container with all configured settings

3. **Verify it's running:**
   ```bash
   docker compose ps
   docker compose logs -f
   ```

4. **Access the dashboard:**
   Open browser to: `http://<server-ip>:8080`

**To update to latest version:**
```bash
cd /opt/powerhive
docker compose down
docker compose build --no-cache
docker compose up -d
```

**To use a specific version/branch:**
Edit the `build.context` line in docker-compose.yml:
```yaml
context: https://github.com/YOUR_USERNAME/powerhive.git#v1.0.0  # Use a tag
context: https://github.com/YOUR_USERNAME/powerhive.git#develop  # Use a branch
```

### Option 2: Clone Repository Locally (For Development)

1. **Clone the project to the server:**
   ```bash
   # If using git
   git clone <your-repo-url>
   cd powerhive

   # Or transfer files via scp/rsync
   scp -r powerhive/ user@server:/opt/powerhive/
   ssh user@server
   cd /opt/powerhive
   ```

2. **Configure network subnets:**
   Edit `config.json` to match your datacenter network:
   ```bash
   nano config.json
   ```
   Update the `subnets` array:
   ```json
   "subnets": [
       "192.168.1.0/24",
       "192.168.2.0/24",
       "192.168.3.0/24",
       "192.168.4.0/24"
   ]
   ```

3. **Build and start the service:**
   ```bash
   docker compose up -d
   ```

4. **Verify it's running:**
   ```bash
   docker compose ps
   docker compose logs -f
   ```

5. **Access the dashboard:**
   Open browser to: `http://<server-ip>:8080`

### Option 3: Using Docker CLI Directly

1. **Build the image:**
   ```bash
   docker build -t powerhive:latest .
   ```

2. **Create a data volume:**
   ```bash
   docker volume create powerhive-data
   ```

3. **Run the container:**
   ```bash
   docker run -d \
     --name powerhive \
     --restart unless-stopped \
     -p 8080:8080 \
     -v powerhive-data:/app/data \
     -e TZ=UTC \
     --memory=2g \
     --cpus=4 \
     powerhive:latest
   ```

4. **Check status:**
   ```bash
   docker ps
   docker logs -f powerhive
   ```

---

## Deployment Options

### Deployment 1: Standard Bridge Network
Default configuration. Container runs on Docker bridge network with port forwarding.

**Pros:**
- Isolated from host network
- Standard Docker networking
- Easy firewall management

**Cons:**
- May have issues discovering miners on certain network configurations
- Requires port forwarding

**Use when:** Miners are accessible via routing/NAT or server has multiple NICs.

### Deployment 2: Host Network Mode
Container shares host's network stack directly.

**Enable in `docker-compose.yml`:**
```yaml
services:
  powerhive:
    network_mode: host
    # Remove ports section when using host mode
```

**Or with Docker CLI:**
```bash
docker run -d \
  --name powerhive \
  --network host \
  --restart unless-stopped \
  -v powerhive-data:/app/data \
  --memory=2g \
  --cpus=4 \
  powerhive:latest
```

**Pros:**
- Direct access to all network interfaces
- Better performance for network-intensive operations
- Easier subnet scanning

**Cons:**
- Less isolation
- Port 8080 directly exposed on host

**Use when:** Miners are on the same LAN as the server.

---

## Configuration

### Editing config.json

#### Network Configuration
```json
{
  "network": {
    "subnets": [
      "192.168.1.0/24",
      "10.0.0.0/16"
    ],
    "light_scan_timeout_ms": 300,
    "miner_probe_timeout_ms": 1500
  }
}
```
- **subnets**: Array of CIDR ranges to scan for miners
- **light_scan_timeout_ms**: Timeout for initial TCP connection (default: 300ms)
- **miner_probe_timeout_ms**: Timeout for API probe requests (default: 1500ms)

#### Polling Intervals
```json
{
  "intervals": {
    "discovery_seconds": 30,
    "status_seconds": 60,
    "telemetry_seconds": 3600,
    "plant_seconds": 30,
    "balancer_seconds": 60
  }
}
```
- **discovery_seconds**: How often to scan for new miners (default: 30)
- **status_seconds**: Status polling frequency (default: 60 = 1 minute)
- **telemetry_seconds**: Detailed telemetry frequency (default: 3600 = 1 hour)
- **balancer_seconds**: Power balancing cycle (default: 60)

**Note:** Current settings optimized for 1000 machines. Don't reduce intervals without increasing worker counts in code.

#### Plant API Configuration
```json
{
  "plant": {
    "api_endpoint": "https://your-energy-api.example.com/data/latest",
    "api_key": "your-api-key-here"
  }
}
```

### Applying Configuration Changes

**Option A: Restart container (if using volume mount override)**
```bash
docker compose restart
```

**Option B: Rebuild and redeploy (if config baked into image)**
```bash
docker compose down
docker compose up -d --build
```

---

## Data Persistence

### Volume Management

The SQLite database is stored in `/app/data` within the container, mapped to a Docker volume.

#### Check volume location:
```bash
docker volume inspect powerhive-data
```

#### Backup database:
```bash
# Copy database from volume to host
docker run --rm \
  -v powerhive-data:/app/data \
  -v $(pwd):/backup \
  ubuntu tar czf /backup/powerhive-backup-$(date +%Y%m%d).tar.gz /app/data
```

#### Restore database:
```bash
# Stop the application
docker compose down

# Extract backup to volume
docker run --rm \
  -v powerhive-data:/app/data \
  -v $(pwd):/backup \
  ubuntu bash -c "cd / && tar xzf /backup/powerhive-backup-20250107.tar.gz"

# Restart application
docker compose up -d
```

### Using Host Path for Data (Alternative)

Edit `docker-compose.yml`:
```yaml
volumes:
  powerhive-data:
    driver: local
    driver_opts:
      type: none
      o: bind
      device: /mnt/data/powerhive  # Your desired host path
```

Then create the directory and deploy:
```bash
sudo mkdir -p /mnt/data/powerhive
sudo chown 10001:10001 /mnt/data/powerhive
docker compose up -d
```

---

## Monitoring & Logs

### View Container Logs
```bash
# Follow logs in real-time
docker compose logs -f

# View last 100 lines
docker compose logs --tail=100

# View logs for specific time range
docker compose logs --since 30m
```

### Check Resource Usage
```bash
# Container stats
docker stats powerhive

# Detailed inspection
docker inspect powerhive
```

### Health Check Status
```bash
# Check health status
docker compose ps

# Inspect health check details
docker inspect powerhive --format='{{json .State.Health}}' | jq
```

### Dashboard Access
- **URL:** `http://<server-ip>:8080`
- **No authentication** (consider adding reverse proxy with auth for production)

### Key Metrics to Monitor

1. **Poll Cycle Completion**
   - Check logs for cycle duration messages
   - Status poller should complete in <30s
   - Telemetry poller should complete in <60s

2. **Database Size**
   ```bash
   docker exec powerhive du -sh /app/data/powerhive.db
   ```

3. **Memory Usage**
   ```bash
   docker stats powerhive --no-stream
   ```
   Should stay under 500 MB under normal load.

4. **Miner Discovery**
   - Check dashboard for miner count
   - Review logs for discovery errors

---

## Maintenance

### Updating PowerHive

1. **Pull latest code/image:**
   ```bash
   cd /opt/powerhive
   git pull origin main
   # Or transfer new files
   ```

2. **Rebuild and restart:**
   ```bash
   docker compose down
   docker compose up -d --build
   ```

3. **Verify no issues:**
   ```bash
   docker compose logs -f
   ```

### Database Maintenance

#### Check database size:
```bash
docker exec powerhive sqlite3 /app/data/powerhive.db "SELECT page_count * page_size as size FROM pragma_page_count(), pragma_page_size();" | numfmt --to=iec-i
```

#### Vacuum database (reclaim space):
```bash
docker exec powerhive sqlite3 /app/data/powerhive.db "VACUUM;"
```

### Implementing Data Retention (Future)

When database grows too large (>100 GB), implement retention:

```sql
-- Delete chip telemetry older than 7 days
DELETE FROM chain_chips
WHERE snapshot_id IN (
  SELECT id FROM chain_snapshots
  WHERE recorded_at < datetime('now', '-7 days')
);

-- Delete old chain snapshots older than 30 days
DELETE FROM chain_snapshots
WHERE recorded_at < datetime('now', '-30 days');

-- Delete old status snapshots older than 30 days
DELETE FROM statuses
WHERE recorded_at < datetime('now', '-30 days');

-- Reclaim space
VACUUM;
```

Consider running monthly via cron:
```bash
# Create retention script
cat > /opt/powerhive/cleanup.sh <<'EOF'
#!/bin/bash
docker exec powerhive sqlite3 /app/data/powerhive.db <<SQL
DELETE FROM chain_chips WHERE snapshot_id IN (
  SELECT id FROM chain_snapshots WHERE recorded_at < datetime('now', '-7 days')
);
DELETE FROM chain_snapshots WHERE recorded_at < datetime('now', '-30 days');
DELETE FROM statuses WHERE recorded_at < datetime('now', '-30 days');
VACUUM;
SQL
EOF

chmod +x /opt/powerhive/cleanup.sh

# Add to crontab (run monthly on 1st at 2 AM)
(crontab -l 2>/dev/null; echo "0 2 1 * * /opt/powerhive/cleanup.sh >> /var/log/powerhive-cleanup.log 2>&1") | crontab -
```

---

## Troubleshooting

### Container Won't Start

**Check logs:**
```bash
docker compose logs
docker logs powerhive
```

**Common issues:**
1. **Port 8080 already in use:**
   ```bash
   sudo lsof -i :8080
   # Kill conflicting process or change port in docker-compose.yml
   ```

2. **Permission issues with volume:**
   ```bash
   docker volume rm powerhive-data
   docker compose up -d
   ```

### No Miners Discovered

**Possible causes:**

1. **Network configuration:**
   - Verify subnets in `config.json` match your network
   - Try host network mode: `network_mode: host`

2. **Firewall blocking:**
   ```bash
   # Test from container
   docker exec powerhive wget -O- --timeout=2 http://192.168.1.100
   ```

3. **Miners not responsive:**
   - Verify miners are powered on
   - Check miner firmware is running HTTP API

### High Memory Usage

**Symptoms:** Container uses >1.5 GB RAM

**Solutions:**
1. Check for database corruption:
   ```bash
   docker exec powerhive sqlite3 /app/data/powerhive.db "PRAGMA integrity_check;"
   ```

2. Reduce polling frequency in `config.json`

3. Restart container:
   ```bash
   docker compose restart
   ```

### Slow Poll Cycles

**Check logs for cycle duration:**
```bash
docker compose logs | grep "completed in"
```

**If status poller takes >50s:**
- Increase worker count in `internal/app/status_poller.go` (currently: 100)
- Rebuild: `docker compose up -d --build`

**If telemetry poller takes >300s:**
- Increase worker count in `internal/app/telemetry_poller.go` (currently: 30)
- Or increase `telemetry_seconds` interval

### Dashboard Not Loading

1. **Check container is running:**
   ```bash
   docker compose ps
   ```

2. **Check health status:**
   ```bash
   docker inspect powerhive | grep -A5 Health
   ```

3. **Test locally from server:**
   ```bash
   curl http://localhost:8080
   ```

4. **Check firewall:**
   ```bash
   sudo ufw status
   sudo ufw allow 8080/tcp
   ```

### Database Locked Errors

SQLite single-writer limitation. Symptoms in logs:
```
database is locked
```

**Solutions:**
1. Increase timeouts in code (requires rebuild)
2. Migrate to PostgreSQL for >1000 machines
3. Reduce concurrent operations (increase intervals)

---

## Security Considerations

### Network Security

1. **Restrict dashboard access:**
   - Use firewall rules to limit access to port 8080
   - Consider reverse proxy with authentication (nginx + basic auth)
   ```bash
   sudo ufw allow from 10.0.0.0/8 to any port 8080
   sudo ufw deny 8080
   ```

2. **Use HTTPS:**
   - Place nginx/traefik in front with SSL certificates
   - Let's Encrypt for automatic certificate management

### Container Security

1. **Run as non-root:** ✅ Already configured (user: appuser, UID 10001)

2. **Read-only filesystem (optional):**
   Add to `docker-compose.yml`:
   ```yaml
   read_only: true
   tmpfs:
     - /tmp
   ```

3. **Limit capabilities:**
   ```yaml
   cap_drop:
     - ALL
   cap_add:
     - NET_BIND_SERVICE
   ```

### API Key Security

- Miner API keys stored in database (not logged)
- Plant API key in config.json (ensure file permissions: `chmod 600 config.json`)
- Consider using Docker secrets for sensitive values

### Backup Security

Encrypt backups if storing offsite:
```bash
# Create encrypted backup
docker run --rm \
  -v powerhive-data:/app/data \
  -v $(pwd):/backup \
  ubuntu tar czf - /app/data | \
  openssl enc -aes-256-cbc -salt -out powerhive-backup-$(date +%Y%m%d).tar.gz.enc
```

---

## Production Checklist

Before deploying to production:

- [ ] Server meets minimum requirements (2 GB RAM, 4 cores, 200 GB storage)
- [ ] Ubuntu 22.04 LTS or newer installed
- [ ] Docker and Docker Compose installed
- [ ] Network subnets configured correctly in `config.json`
- [ ] Plant API endpoint and key configured (if using power balancing)
- [ ] Firewall rules configured for port 8080 access
- [ ] Data volume backup strategy implemented
- [ ] Monitoring/alerting configured (optional but recommended)
- [ ] Database retention script scheduled (for long-term deployments)
- [ ] Reverse proxy with HTTPS configured (recommended)
- [ ] Container set to restart automatically (`restart: unless-stopped`)
- [ ] Initial test run completed successfully
- [ ] Documentation shared with operations team

---

## Getting Help

**Project maintainer:** [Your contact info]

**Logs to include when reporting issues:**
```bash
docker compose logs --tail=500 > powerhive-logs.txt
docker inspect powerhive > powerhive-inspect.txt
cat config.json > config-sanitized.json  # Remove sensitive API keys first!
```

**Useful diagnostic commands:**
```bash
# System info
uname -a
docker version
docker compose version

# Container details
docker stats powerhive --no-stream
docker exec powerhive ps aux

# Database stats
docker exec powerhive sqlite3 /app/data/powerhive.db "SELECT name, COUNT(*) FROM sqlite_master m LEFT JOIN pragma_table_info(m.name) WHERE m.type='table' GROUP BY m.name;"
```

---

## Appendix: Scaling Beyond 1000 Machines

| Fleet Size | CPU Cores | RAM | Storage (30-day) | Status Workers | Telemetry Workers |
|------------|-----------|-----|------------------|----------------|-------------------|
| 500        | 2         | 1 GB | 100 GB          | 50             | 15                |
| 1000       | 4         | 2 GB | 200 GB          | 100            | 30                |
| 2000       | 8         | 4 GB | 400 GB          | 200            | 60                |
| 5000       | 16        | 8 GB | 1 TB            | 500            | 150               |

**Beyond 2000 machines:** Consider PostgreSQL instead of SQLite and horizontal scaling with multiple PowerHive instances.

---

*Last updated: 2025-01-07*
