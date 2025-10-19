# factctl Quick Start Guide

This guide will help you get started with factctl in just a few minutes.

## Prerequisites

- Go 1.24.5 or later
- Factorio installation (for running instances)

## Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/WhyIsSandwich/factctl.git
   cd factctl
   ```

2. **Build the tool:**
   ```bash
   go build -o factctl cmd/factctl/main.go
   ```

3. **Install to your PATH:**
   ```bash
   sudo cp factctl /usr/local/bin/
   ```

## Your First Instance

### 1. Create a Basic Instance

```bash
# Create a simple instance
factctl up my-first-server
```

This creates a basic Factorio instance with default settings.

### 2. Run Your Instance

```bash
# Launch the instance
factctl run my-first-server
```

### 3. Monitor Logs

In another terminal, you can watch the logs:

```bash
# Stream live logs
factctl logs my-first-server
```

### 4. Clean Up

When you're done:

```bash
# Remove the instance
factctl down my-first-server
```

## Using Configuration Files

### 1. Create a Configuration File

Create a file called `my-server.jsonc`:

```jsonc
{
  "name": "my-server",
  "version": "1.1",
  "headless": true,
  "port": 34197,
  "mods": {
    "enabled": ["base"],
    "sources": {}
  },
  "server": {
    "name": "My Factorio Server",
    "max_players": 4,
    "public": false,
    "auto_save": true,
    "auto_save_interval": 10
  }
}
```

### 2. Create Instance with Configuration

```bash
factctl up my-server --config ./my-server.jsonc
```

### 3. Run the Instance

```bash
factctl run my-server
```

## Adding Mods

### 1. Update Configuration

Edit your configuration file to include mods:

```jsonc
{
  "name": "modded-server",
  "version": "1.1",
  "headless": true,
  "mods": {
    "enabled": ["base", "bobmods"],
    "sources": {
      "bobmods": "portal:bobmods"
    }
  }
}
```

### 2. Update Instance

```bash
factctl up modded-server --config ./my-server.jsonc
```

The tool will automatically download and install the mods.

## Common Commands

| Command | Description |
|---------|-------------|
| `factctl up <name>` | Create/update instance |
| `factctl run <name>` | Launch instance |
| `factctl logs <name>` | View logs |
| `factctl down <name>` | Remove instance |
| `factctl down <name> --backup` | Remove with backup |

## Next Steps

- Check out the [full documentation](README.md) for advanced features
- Look at the [example configurations](examples/) for different setups
- Explore mod sources: Portal, GitHub, and Git repositories

## Troubleshooting

**Instance not found:**
```bash
# List instances
ls ~/.config/factctl/instances/

# Recreate instance
factctl up my-server
```

**Factorio not found:**
- Make sure Factorio is installed and in your PATH
- The tool will show helpful error messages

**Permission errors:**
```bash
# Fix permissions
chmod -R 755 ~/.config/factctl/
```

## Getting Help

- Check the [README.md](README.md) for detailed documentation
- Look at example configurations in the `examples/` directory
- Check the worklog in [WORKLOG.md](WORKLOG.md) for development status
