# Gsync

A CLI tool that reports on merge requests created by a set of authors across one or more GitLab (or GitHub) repositories within a configurable lookback window.

> **Note:** GitLab and GitHub are supported.

<img width="1249" height="668" alt="image" src="https://github.com/user-attachments/assets/7ef3e671-a58c-4497-b437-0ae55cd73b7e" />

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

### `gsync menu`

Launch an interactive terminal menu to browse pull requests without writing a report file:

```bash
gsync menu
```

The menu walks through three screens:

1. **Form** — set a date range, state filter, and authors. Authors are shown as a list; type a name and press `Enter` to add, press `d` or `Delete` to remove the selected one. Values are pre-filled from your config.
2. **Loading** — fetches results from all configured providers.
3. **Results** — scrollable list of PRs grouped by author. Press `Enter` on any PR to open it in your browser, and `q`/`Esc` to return to the form.

---

### `gsync report`

Generate a merge request report:

```bash
gsync report
```

Flags override the values set in your config:

| Flag | Description |
|------|-------------|
| `--format` | Output format: `text`, `json`, or `yaml` |
| `--out` | Directory to write the report file |
| `--lookback` | Hours to look back for MRs (overrides config) |
| `--authors` | Comma-separated list of authors to filter by (overrides config) |
| `--since` | Start date `YYYY-MM-DD` (overrides `--lookback`) |
| `--until` | End date `YYYY-MM-DD` (default: no upper bound) |
| `--state` | MR state: `opened`, `closed`, `merged`, or `all` (overrides config) |

Examples:

```bash
gsync report --format json
gsync report --out ~/reports
gsync report --lookback 48
gsync report --authors alice,bob
gsync report --authors alice --authors bob
gsync report --format text --out ~/reports
gsync report --since 2024-01-01 --until 2024-01-31
gsync report --since 2024-01-01
gsync report --state merged
gsync report --state all --since 2024-03-01
```

The `--config` flag is global and can be passed before any subcommand:

```bash
gsync --config /path/to/config.yaml report
```

## Config Subcommand

Manage the config file from the CLI without editing YAML by hand.

```bash
gsync config show                                  # print current config as YAML
gsync config set format text                       # set top-level field
gsync config set out ~/reports
gsync config add author alice                      # add to first provider
gsync config add author alice --provider github    # add to specific provider
gsync config remove author alice
gsync config add repo org/newrepo --provider github
gsync config remove repo org/newrepo --provider github
```

Supported fields for `gsync config set`: `format`, `out`.

## Cron

To run automatically each morning:

```
0 9 * * * gsync report >> /tmp/mr-report.log 2>&1
```
