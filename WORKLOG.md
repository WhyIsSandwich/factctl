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
- [ ] Define instance configuration structure
  - [ ] Create configuration schema
  - [ ] Add validation logic
  - [ ] Write configuration tests
- [ ] Implement instance creation (`up` command)
  - [ ] Directory structure creation
  - [ ] Configuration file generation
  - [ ] Mod directory setup
  - [ ] Save game management
- [ ] Implement instance removal (`down` command)
  - [ ] Safe cleanup procedures
  - [ ] Backup handling
- [ ] Implement instance launching (`run` command)
  - [ ] Runtime environment setup
  - [ ] Command-line argument handling
  - [ ] Process management
- [ ] Add log streaming functionality
  - [ ] Log file handling
  - [ ] Real-time streaming
  - [ ] Log rotation support

### 2. Mod Management System
- [ ] Design dependency resolution system
  - [ ] Version constraint parsing
  - [ ] Dependency graph building
  - [ ] Conflict resolution
- [ ] Implement mod installation workflow
  - [ ] Download management
  - [ ] Installation verification
  - [ ] Update checking
- [ ] Add mod configuration handling
  - [ ] Settings file management
  - [ ] Default configuration
  - [ ] User overrides

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
(No tasks currently in progress)

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