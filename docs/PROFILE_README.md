# Stephan Loesevitz

Linux Systems Engineer building safer, more observable hosting infrastructure.
I work at the boundary between Linux operations and software engineering: taking
messy, failure-prone server workflows and turning them into repeatable,
auditable automation.

My focus is practical reliability—least privilege, safe migrations, recovery
paths, useful health signals, and documentation that an operator can follow at
03:00. I work across Red Hat and Debian-based systems, cPanel/PHP hosting,
databases, Kubernetes, and infrastructure-as-code, using Go, Python, Bash, and
Ansible.

## Selected work

| Project | What it demonstrates |
| --- | --- |
| [StePanel](https://github.com/itchyitchy123/StePanel) | Go control plane for LAMP hosting and cPanel migrations, with validated restores, durable jobs, audit events, backups, and reproducible releases. Start with the [architecture](https://github.com/itchyitchy123/StePanel/blob/main/docs/ARCHITECTURE.md) or [demo walkthrough](https://github.com/itchyitchy123/StePanel/blob/main/docs/DEMO.md). |
| [php-fpm_auto-optimize](https://github.com/itchyitchy123/php-fpm_auto-optimize) | Workload-based PHP-FPM capacity recommendations that connect measurement to safer resource planning. |
| [install_fail2ban](https://github.com/itchyitchy123/install_fail2ban) | Validated, repeatable security deployment for Linux hosting services. |
| [wpress_extract_plugin_cpanel](https://github.com/itchyitchy123/wpress_extract_plugin_cpanel) | WordPress migration tooling for real cPanel archive workflows. |

## How I approach infrastructure

- Define the trust boundary before writing the automation.
- Make destructive operations explicit, bounded, and recoverable.
- Prefer observable failure over silent success.
- Test the recovery path, not only the happy path.
- Leave behind clear runbooks, interfaces, and operating limits.

## Currently building

I am developing StePanel as a focused reference implementation for safe hosting
operations. The project is intentionally honest about its limits: it is a
single-host operator control plane, not yet a complete multi-tenant platform.

## Connect

[LinkedIn](https://www.linkedin.com/in/stephan-loesevitz-85646b225/) ·
[cyberducttape.com](https://cyberducttape.com)

Open to infrastructure engineering, Linux hosting, platform automation, and
reliability conversations.
