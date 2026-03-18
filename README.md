# cloudshell-term

Interactive AWS CloudShell terminal from your CLI. No AWS Console required.

```
$ cloudshell-term us-west-2
Connecting to CloudShell in us-west-2...
Connected.

cloudshell-user@ip-10-132-74-210 ~ $
```

## What it does

Opens an interactive shell session to an AWS CloudShell environment using the internal CloudShell API (reverse engineered — no public API exists). Handles environment creation, startup from suspended state, session management, and heartbeats to keep the environment alive.

## Install

```bash
go install github.com/dan-v/cloudshell-term@latest
```

Or build from source:

```bash
go build -o cloudshell-term .
```

## Usage

```bash
# Connect to default region (AWS_DEFAULT_REGION or us-east-1)
cloudshell-term

# Connect to a specific region
cloudshell-term eu-west-1
```

## Requirements

- AWS credentials configured (`~/.aws/credentials` or environment variables)
- [`session-manager-plugin`](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html) installed
