# Gsync

A CLI tool that reports on merge requests created by a set of authors across one or more GitLab repositories within a configurable lookback window.

> **Note:** GitLab and GitHub are supported.

## Install

Build and install the binary to `/usr/local/bin`:

```bash
./install.sh
```

> You may need `sudo ./install.sh` if `/usr/local/bin` requires elevated permissions.

## Config

The config lives at `~/.gsync/config.yaml`. Create the directory and copy the example:

```bash
mkdir -p ~/.gsync
cp config.example.yaml ~/.gsync/config.yaml
```

Then fill in your details:

```yaml
format: "text"         # text | json | yaml
output_dir: "./reports"  # omit to print to stdout
providers:
  - name: gitlab
    token: "glpat-xxxxxxxxxxxx"
    base_url: "https://gitlab.com"
    lookback_hours: 96
    state: "all"       # opened | closed | merged | all
    authors:
      - alice
      - bob
    repos:
      - group/repo
      - group/*        # wildcard expands all projects under a group

  - name: github
    token: "ghp-xxxxxxxxxxxx"
    base_url: "https://github.com"   # omit or set to GitHub Enterprise URL
    lookback_hours: 96
    state: "all"       # opened | closed | merged | all
    authors:
      - alice
      - bob
    repos:
      - org/repo
      - org/*          # wildcard expands all repos under an org
```

## Usage

```bash
gsync
```

Flags override the values set in your config:

| Flag | Description |
|------|-------------|
| `--format` | Output format: `text`, `json`, or `yaml` |
| `--out` | Directory to write the report file |
| `--lookback` | Hours to look back for MRs (overrides config) |
| `--authors` | Comma-separated list of authors to filter by (overrides config) |
| `--config` | Path to config file (default: `~/.gsync/config.yaml`) |

Examples:

```bash
gsync --format json
gsync --out ~/reports
gsync --lookback 48
gsync --authors alice,bob
gsync --authors alice --authors bob
gsync --format text --out ~/reports
gsync --config /path/to/config.yaml
```

## Cron

To run automatically each morning:

```
0 9 * * * gsync >> /tmp/mr-report.log 2>&1
```
