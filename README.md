# factctl

A runtime orchestrator for Factorio game instances, managing mods, runtimes, and server configurations.

## Overview

factctl is a command-line tool designed to simplify the management of Factorio game instances. It provides a unified interface for creating, configuring, and running Factorio instances with different mod sets, versions, and configurations.

## Features

- **Instance Management**: Create, update, and remove Factorio instances
- **Mod Management**: Install and manage mods from multiple sources (Portal, GitHub, Git)
- **Runtime Management**: Handle different Factorio versions and runtime environments
- **Log Streaming**: Real-time log monitoring and historical log access
- **Configuration Management**: JSON/JSONC configuration files with validation
- **Cross-Platform**: Works on Windows, macOS, and Linux

## Installation

### Prerequisites

- Go 1.24.5 or later
- Factorio installation (install via Steam, direct download, or package manager)

### Building from Source

```bash
git clone https://github.com/WhyIsSandwich/factctl.git
cd factctl
go build -o factctl cmd/factctl/main.go
```

### Installation

```bash
# Install to your PATH
sudo cp factctl /usr/local/bin/

# Or install to a local directory
mkdir -p ~/bin
cp factctl ~/bin/
export PATH="$HOME/bin:$PATH"
```

## Quick Start

### 1. Create Your First Instance

```bash
# Create a basic instance
factctl up my-server

# Create a headless server instance
factctl up my-server --headless

# Create an instance with a configuration file
factctl up my-server --config ./server-config.jsonc
```

### 2. Run Your Instance

```bash
# Launch the instance
factctl run my-server

# Launch in headless mode
factctl run my-server --headless
```

### 3. Monitor Logs

```bash
# Stream live logs
factctl logs my-server

# Show recent logs without following
factctl logs my-server --no-follow
```

### 4. Clean Up

```bash
# Remove instance
factctl down my-server

# Remove with backup
factctl down my-server --backup
```

## Configuration

### Instance Configuration

Instance configurations are stored as JSON/JSONC files. Here's an example:

```jsonc
{
  "name": "my-server",
  "version": "1.1",
  "headless": true,
  "port": 34197,
  "save_file": "my-world.zip",
  "mods": {
    "enabled": ["base", "bobmods", "angelsmods"],
    "sources": {
      "bobmods": "portal:bobmods",
      "angelsmods": "github:Angel-666/AngelMods"
    }
  },
  "server": {
    "name": "My Factorio Server",
    "max_players": 8,
    "public": false,
    "password": "secret123",
    "admins": ["admin1", "admin2"],
    "auto_save": true,
    "auto_save_interval": 10
  }
}
```

### Mod Sources

factctl supports multiple mod sources:

- **Portal**: `portal:modname` - Download from Factorio mod portal
- **GitHub**: `github:user/repo` - Clone and build from GitHub repository
- **Git**: `git:https://example.com/repo.git` - Clone from any Git repository

### Directory Structure

```
~/.config/factctl/          # Base directory (Linux)
├── instances/              # Instance directories
│   └── my-server/
│       ├── config/
│       │   ├── instance.json
│       │   ├── mod-list.json
│       │   └── server-settings.json
│       ├── mods/           # Installed mods
│       ├── saves/          # Save files
│       └── factorio.log    # Instance logs
├── runtimes/              # Factorio installations
└── backups/               # Instance backups
```

## Commands

### `factctl up <instance-name> [options]`

Create or update an instance.

**Options:**
- `--config <path>`: Path to configuration file
- `--headless`: Run in headless mode
- `--base-dir <path>`: Override base directory

**Examples:**
```bash
factctl up my-server
factctl up my-server --config ./config.jsonc
factctl up my-server --headless
```

### `factctl down <instance-name> [options]`

Remove an instance.

**Options:**
- `--backup`: Create backup before removal

**Examples:**
```bash
factctl down my-server
factctl down my-server --backup
```

### `factctl run <instance-name> [options]`

Launch a Factorio instance.

**Options:**
- `--headless`: Override headless mode
- `--base-dir <path>`: Override base directory

**Examples:**
```bash
factctl run my-server
factctl run my-server --headless
```

### `factctl logs <instance-name> [options]`

Stream or view instance logs.

**Options:**
- `--no-follow`: Show recent logs without following

**Examples:**
```bash
factctl logs my-server
factctl logs my-server --no-follow
```

## Advanced Usage

### Multiple Instances

You can manage multiple instances simultaneously:

```bash
# Create different instances
factctl up vanilla-server
factctl up modded-server --config ./modded-config.jsonc
factctl up creative-server --config ./creative-config.jsonc

# Run different instances
factctl run vanilla-server &
factctl run modded-server &
```

### Mod Development

For mod development, you can use Git sources:

```jsonc
{
  "name": "dev-server",
  "version": "1.1",
  "mods": {
    "enabled": ["base", "my-mod"],
    "sources": {
      "my-mod": "git:https://github.com/user/my-mod.git"
    }
  }
}
```

### Server Management

For dedicated servers:

```jsonc
{
  "name": "production-server",
  "version": "1.1",
  "headless": true,
  "port": 34197,
  "server": {
    "name": "Production Server",
    "max_players": 20,
    "public": true,
    "password": "secure-password",
    "admins": ["admin1", "admin2"],
    "auto_save": true,
    "auto_save_interval": 5
  }
}
```

## Troubleshooting

### Common Issues

**Instance not found:**
```bash
# Check if instance exists
ls ~/.config/factctl/instances/

# Recreate instance
factctl up my-server
```

**Factorio not found:**
```bash
# Check if Factorio is installed
which factorio

# Install Factorio via your preferred method:
# - Steam: Purchase and install through Steam
# - Direct: Download from factorio.com
# - Package manager: Use your system's package manager
```

**Permission errors:**
```bash
# Check directory permissions
ls -la ~/.config/factctl/

# Fix permissions if needed
chmod -R 755 ~/.config/factctl/
```

### Log Files

Instance logs are stored in the instance directory:
- `factorio.log` - Current log file
- `factorio.log.1`, `factorio.log.2`, etc. - Rotated log files

### Backup and Restore

Backups are created in the `backups/` directory:
```bash
# List available backups
ls ~/.config/factctl/backups/

# Restore from backup (manual process)
tar -xzf ~/.config/factctl/backups/my-server-20240101-120000.tar.gz
```

## Development

### Building

```bash
# Build for current platform
go build -o factctl cmd/factctl/main.go

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o factctl-linux cmd/factctl/main.go
```

### Testing

```bash
# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...
```

### Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

## Roadmap

- [ ] Web interface for instance management
- [ ] Plugin system for custom mod sources
- [ ] Docker support for containerized instances
- [ ] Multi-user support with permissions
- [ ] Instance templates and presets
- [ ] Automated backup scheduling
- [ ] Performance monitoring and metrics
- [ ] Real mod download integration (Portal API)
- [ ] Advanced mod dependency resolution
