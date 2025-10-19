# factctl Project Worklog

## Project Overview
factctl is a runtime orchestrator for Factorio game instances, managing mods, runtimes, and server configurations.

## Current Status
- [x] Source Resolution System (Complete)
- [x] Authentication System (Complete)
- [x] JSONC Parser (Complete)
- [ ] Instance Management System (Not Started)
- [ ] Mod Management System (Not Started)
- [ ] Runtime Management System (Not Started)
- [ ] CLI Implementation (Basic Structure Only)
- [ ] Documentation (Minimal)

## Priority Tasks

### 1. Instance Management System
- [x] Define instance configuration structure
  - [x] Create configuration schema
  - [x] Add validation logic
  - [x] Write configuration tests
- [x] Implement instance creation (`up` command)
  - [x] Directory structure creation
  - [x] Configuration file generation
  - [x] Mod directory setup
  - [x] Save game management
- [x] Implement instance removal (`down` command)
  - [x] Safe cleanup procedures
  - [x] Backup handling
  - [x] Backup restoration
- [x] Implement instance launching (`run` command)
  - [x] Runtime environment setup
  - [x] Command-line argument handling
  - [x] Process management
  - [x] Graceful shutdown handling
- [x] Add log streaming functionality
  - [x] Log file handling
  - [x] Real-time streaming
  - [x] Log rotation support
  - [x] Log parsing and filtering
  - [x] Historical log access

### 2. Mod Management System
- [x] Design dependency resolution system
  - [x] Version constraint parsing
  - [x] Dependency graph building
  - [x] Version compatibility checking
- [x] Implement mod installation workflow
  - [x] Download management
  - [x] Installation verification
  - [x] Dependency handling
- [x] Add mod configuration handling
  - [x] Mod list management
  - [x] Enable/disable functionality
  - [x] Mod info extraction

### 3. Runtime Management System
- [ ] Add Factorio version management
  - [ ] Version listing
  - [ ] Download handling
  - [ ] Installation verification
- [ ] Implement runtime environment setup
  - [ ] Directory structure
  - [ ] Environment variables
  - [ ] Resource management
- [ ] Add save game management
  - [ ] Save file handling
  - [ ] Backup system
  - [ ] Restore functionality

### 4. CLI Implementation
- [ ] Complete command handlers
  - [ ] Argument parsing
  - [ ] Validation
  - [ ] Help text
- [ ] Add progress reporting
  - [ ] Download progress
  - [ ] Installation status
  - [ ] Operation completion
- [ ] Implement error handling
  - [ ] User-friendly messages
  - [ ] Debug logging
  - [ ] Recovery procedures

### 5. Documentation
- [ ] Write comprehensive README
  - [ ] Installation instructions
  - [ ] Basic usage guide
  - [ ] Configuration examples
- [ ] Add API documentation
  - [ ] Source resolution
  - [ ] Authentication
  - [ ] Instance management
- [ ] Create user guides
  - [ ] Getting started
  - [ ] Advanced usage
  - [ ] Troubleshooting
- [ ] Add configuration examples
  - [ ] Basic setups
  - [ ] Advanced scenarios
  - [ ] Multi-mod configurations

## In Progress
2. Mod Management System
   - Implementing mod installation
   - Dependency resolution
   - Version constraint handling
   - Mod configuration management

## Completed Tasks
1. [x] Source Resolution System
   - [x] Multiple source type support (portal, GitHub, Git, etc.)
   - [x] Source parsing and validation
   - [x] Integration tests
   - [x] Multi-mod repository support

2. [x] Authentication System
   - [x] Credential management
   - [x] Secure storage
   - [x] Unit tests

3. [x] JSONC Parser
   - [x] Comment stripping
   - [x] JSON validation
   - [x] Unit tests

## Notes
- Project is using Go 1.24.5
- Licensed under Apache License 2.0
- Currently focused on command-line interface
- Planning to add server management features in future