# git-synced

A CLI tool that reports on merge requests created by a set of authors across one or more GitLab repositories within a configurable lookback window.

> **Note:** Only GitLab is supported currently.

## Install

Build and install the binary to `/usr/local/bin`:

```bash
./install.sh
```

> You may need `sudo ./install.sh` if `/usr/local/bin` requires elevated permissions.

## Config

The config lives at `~/.git-synced/config.yaml`. Create the directory and copy the example:

```bash
mkdir -p ~/.git-synced
cp config.example.yaml ~/.git-synced/config.yaml
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
```

## Usage

```bash
git-synced
```

Override the config path if needed:

```bash
git-synced --config /path/to/config.yaml
```

## Cron

To run automatically each morning:

```
0 9 * * * git-synced >> /tmp/mr-report.log 2>&1
```
