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

Supports both standard CloudShell environments (with persistent ~1GB storage) and VPC environments (ephemeral, but with access to private VPC resources).

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

# Connect to a VPC environment (access private resources)
cloudshell-term --vpc vpc-abc123 --subnet subnet-def456 --sg sg-ghi789 us-east-1

# Multiple security groups
cloudshell-term --vpc vpc-abc --subnet subnet-def --sg sg-one --sg sg-two us-east-1
```

### VPC Environments

VPC environments run inside your VPC and can access private resources like RDS databases, ElastiCache clusters, and internal services. Note that VPC environments have ephemeral storage — data is lost when the session ends.

## Requirements

- AWS credentials configured (`~/.aws/credentials` or environment variables)
- [`session-manager-plugin`](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html) installed
- For VPC environments: appropriate IAM permissions and VPC/subnet/security group configuration
