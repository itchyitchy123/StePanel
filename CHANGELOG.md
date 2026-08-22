# Changelog

All notable changes to StePanel are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- LAMP installation for Debian/Ubuntu and RHEL-family systems.
- systemd service deployment with a dedicated `stepanel` user.
- cpmove archive validation, staging, website restoration, and opt-in SQL restoration.
- health endpoint and migration-center dashboard workflow.
- initial project documentation, CI, and contributor policies.

### Known limitations

- Authentication, authorization, and TLS termination are not included yet.
- The installer expects a pre-built `stepanel` binary.
- Database restoration requires the local `mysql` client and root/socket access.
