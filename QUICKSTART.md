# PowerHive - Quick Deployment Guide

## Prerequisites
- Docker and Docker Compose installed on your server
- Server can reach your miner network

## Deploy in 3 Steps

### 1. Create deployment directory
```bash
mkdir -p /opt/powerhive && cd /opt/powerhive
```

### 2. Save the docker-compose.yml file
Create a file named `docker-compose.yml` with the contents provided, or download it:
```bash
wget https://raw.githubusercontent.com/YOUR_USERNAME/powerhive/main/docker-compose.yml
```

### 3. Start PowerHive
```bash
docker compose up -d
```

That's it! Docker will automatically:
- Pull the code from GitHub
- Build the application
- Start the service

## Access Dashboard
Open your browser to: `http://<server-ip>:8080`

## View Logs
```bash
docker compose logs -f
```

## Update to Latest Version
```bash
cd /opt/powerhive
docker compose down
docker compose build --no-cache
docker compose up -d
```

## Stop Service
```bash
docker compose down
```

## Troubleshooting

**Container won't start?**
```bash
docker compose logs
```

**Port 8080 already in use?**
```bash
sudo lsof -i :8080
```

**Need to customize network subnets?**

The default configuration scans `192.168.1.0/24`. To change this, you'll need to mount a custom `config.json` file. See full DEPLOY.md for details.

---

For complete documentation, see [DEPLOY.md](./DEPLOY.md)
