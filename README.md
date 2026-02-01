# Uptime Robot Operator

[![Build](https://github.com/joelp172/uptime-robot-operator/actions/workflows/build.yml/badge.svg)](https://github.com/joelp172/uptime-robot-operator/actions/workflows/build.yml)

Kubernetes operator to manage Uptime Robot monitors using the [UptimeRobot API v3](https://uptimerobot.com/api/v3/).

## Description

This operator allows a Kubernetes cluster to create, edit, and delete monitors in Uptime Robot. Monitors managed by this tool will also be reconciled regularly to prevent configuration drift.

### Supported Monitor Types

- **HTTPS** - Standard HTTP/HTTPS endpoint monitoring
- **Keyword** - Monitor for specific keywords in page content
- **Ping** - ICMP ping monitoring
- **Port** - TCP port monitoring
- **Heartbeat** - Expects periodic pings from your services/jobs
- **DNS** - DNS record monitoring

## Getting Started

### Prerequisites

- Go version v1.24.0+
- Docker version 17.03+
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster
- UptimeRobot API key (get from [UptimeRobot Integrations](https://dashboard.uptimerobot.com/integrations))

### Configuration

#### API Key Setup

Create a Kubernetes Secret containing your UptimeRobot API key:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: uptimerobot-api-key
  namespace: uptime-robot-system
type: Opaque
stringData:
  api-key: "your-api-key-here"
```

Create an Account resource referencing the secret:

```yaml
apiVersion: uptimerobot.com/v1
kind: Account
metadata:
  name: default
spec:
  isDefault: true
  apiKeySecretRef:
    name: uptimerobot-api-key
    key: api-key
```

### Monitor Examples

#### Basic HTTPS Monitor

```yaml
apiVersion: uptimerobot.com/v1
kind: Monitor
metadata:
  name: example-https
spec:
  monitor:
    name: My Website
    url: https://example.com
    type: HTTPS
    interval: 5m
```

#### Keyword Monitor

```yaml
apiVersion: uptimerobot.com/v1
kind: Monitor
metadata:
  name: example-keyword
spec:
  monitor:
    name: Keyword Check
    url: https://example.com
    type: Keyword
    interval: 5m
    keyword:
      type: Exists
      value: "Welcome"
      caseSensitive: false
```

#### DNS Monitor

```yaml
apiVersion: uptimerobot.com/v1
kind: Monitor
metadata:
  name: example-dns
spec:
  monitor:
    name: DNS Check
    url: example.com
    type: DNS
    interval: 5m
    dns:
      recordType: A
      value: "93.184.216.34"
```

#### Heartbeat Monitor

```yaml
apiVersion: uptimerobot.com/v1
kind: Monitor
metadata:
  name: example-heartbeat
spec:
  monitor:
    name: Cron Job Heartbeat
    url: https://heartbeat.uptimerobot.com/xxx
    type: Heartbeat
    interval: 5m
    heartbeat:
      interval: 5m
```

### To Deploy on the Cluster

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/uptime-robot-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don't work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/uptime-robot-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the [samples (examples)](config/samples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

> **NOTE**: Ensure that the samples have default values to test it out.

### To Uninstall

**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs (CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## API v3 Migration

This operator uses the UptimeRobot API v3, which introduced several improvements over v2:

- RESTful endpoints with standard HTTP methods
- Bearer token authentication via the Authorization header
- JSON request/response format
- Cursor-based pagination
- New monitor types (DNS, Heartbeat)

For more details, see the [UptimeRobot API v3 documentation](https://uptimerobot.com/api/v3/).

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/uptime-robot-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/uptime-robot-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v1-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
