# Architecture

This document provides a visual overview of the Uptime Robot Operator architecture, including CRD relationships, controller interactions, and data flows.

## System Overview

```mermaid
graph TB
    subgraph "Kubernetes Cluster"
        subgraph "Custom Resources"
            Account[Account<br/>Cluster-scoped]
            Contact[Contact<br/>Cluster-scoped]
            Monitor[Monitor]
            MW[MaintenanceWindow]
            MG[MonitorGroup]
            SI[SlackIntegration]
        end
        
        subgraph "Controllers"
            AC[AccountController]
            CC[ContactController]
            MC[MonitorController]
            MWC[MaintenanceWindowController]
            MGC[MonitorGroupController]
            SIC[SlackIntegrationController]
            IC[IngressController]
        end
        
        subgraph "Kubernetes Resources"
            Secret[Secret<br/>API Keys]
            CM[ConfigMap]
            Events[Events]
            K8sIngress[Ingress<br/>networking.k8s.io]
        end
    end
    
    subgraph "External Services"
        URAPI[UptimeRobot API]
    end
    
    Account -->|references| Secret
    Monitor -->|references| Account
    Monitor -->|references| Contact
    MW -->|references| Account
    MW -->|references| Monitor
    Contact -->|references| Account
    
    AC -->|watches| Account
    CC -->|watches| Contact
    MC -->|watches| Monitor
    MWC -->|watches| MW
    MGC -->|watches| MG
    SIC -->|watches| SI
    IC -->|watches| K8sIngress
    
    AC -->|reconciles| URAPI
    CC -->|reconciles| URAPI
    MC -->|reconciles| URAPI
    MWC -->|reconciles| URAPI
    MGC -->|reconciles| URAPI
    SIC -->|reconciles| URAPI
    IC -->|creates| Monitor
    
    MC -->|publishes heartbeat URL| Secret
    MC -->|publishes heartbeat URL| CM
    
    AC -->|updates status| Account
    CC -->|updates status| Contact
    MC -->|updates status| Monitor
    MWC -->|updates status| MW
    MGC -->|updates status| MG
    SIC -->|updates status| SI

    IC -->|emits warning events| Events
```

## CRD Dependency Graph

```mermaid
graph LR
    subgraph "Core Resources"
        Account[Account<br/>Cluster-scoped<br/>Stores API Key]
    end
    
    subgraph "Alert Configuration"
        Contact[Contact<br/>Cluster-scoped<br/>References Account<br/>Uses existing contact ID]
    end
    
    subgraph "Monitoring"
        Monitor[Monitor<br/>References Account<br/>References Contacts<br/>Creates monitors in UptimeRobot]
        MG[MonitorGroup<br/>Logical grouping<br/>References Account]
    end
    
    subgraph "Maintenance"
        MW[MaintenanceWindow<br/>References Account<br/>References Monitors<br/>Schedules downtime]
    end
    
    subgraph "Integrations"
        SI[SlackIntegration<br/>References Account<br/>Webhook configuration]
    end
    
    subgraph "Kubernetes Native Resources"
        K8sIngress[Ingress<br/>networking.k8s.io<br/>Watched by IngressController]
    end
    
    Monitor --> Account
    Monitor -.optional.-> Contact
    Contact --> Account
    MW --> Account
    MW --> Monitor
    MG --> Account
    SI --> Account
    K8sIngress -.triggers creation of.-> Monitor
    MG -.references.-> Monitor
    
    style Account fill:#e1f5ff
    style Monitor fill:#fff4e1
    style Contact fill:#f0f0f0
    style MW fill:#ffe1f5
```

### Dependency Rules

1. **Account** and **Contact** are cluster-scoped; other managed CRDs are namespaced.
2. **Contact** reconciliation requires a resolvable Account/API key and either `spec.contact.id` or `spec.contact.name`.
3. **Monitor** reconciliation requires a resolvable Account/API key; referenced Contacts must resolve to a `status.id`.
4. **MaintenanceWindow** and **MonitorGroup** resolve monitor references in the same namespace; unresolved or not-ready monitors are skipped.
5. **IngressController** creates/updates/deletes `Monitor` resources only when ingress annotations (prefix `uptimerobot.com/`, configurable) include `enabled=true`.

## Reconciliation Flow

```mermaid
sequenceDiagram
    participant K8s as Kubernetes API
    participant Ctrl as Controller
    participant UR as UptimeRobot API
    
    K8s->>Ctrl: Resource created/updated
    Ctrl->>Ctrl: Get resource + resolve refs/credentials
    
    alt Resource is deleting and finalizer exists
        Ctrl->>Ctrl: Check prune + cleanup external resource
        Ctrl->>K8s: Remove finalizer
    else Normal reconcile
        Ctrl->>UR: Create/Update/Verify external state
        Ctrl->>K8s: Update status fields (Ready/ID/etc.)
        Ctrl->>K8s: Requeue after syncInterval (where applicable)
    end

    alt Reconcile returns error
        Ctrl->>Ctrl: controller-runtime retries with rate-limited backoff
    end
```

## Drift Detection

Drift handling is implemented as periodic reconciliation, and behavior varies by controller:
- **Monitor**, **MaintenanceWindow**, and **MonitorGroup** reconcile desired state to UptimeRobot on each sync interval.
- **SlackIntegration** lists integrations and recreates the Slack integration if missing or different from desired state.
- **MaintenanceWindow** and **MonitorGroup** explicitly recreate resources when backend IDs are not found.

### Drift Detection Frequency

Controlled by `syncInterval` fields (default `24h` where defined):
- Lower values = faster drift detection, more API calls
- Higher values = less API load, slower drift detection

## Finalizer and Deletion Flow (Monitor)

```mermaid
sequenceDiagram
    participant User
    participant K8s as Kubernetes API
    participant Ctrl as Controller
    participant UR as UptimeRobot API
    
    User->>K8s: kubectl delete monitor
    K8s->>K8s: Set deletionTimestamp
    K8s->>Ctrl: Reconcile (deletion)
    
    Ctrl->>Ctrl: Check finalizer present?
    
    alt Finalizer present
        Ctrl->>Ctrl: Check prune setting
        
        alt prune=true
            Ctrl->>K8s: List monitors in same namespace<br/>and account
            
            alt No other Monitor adopts/manages same ID
                Ctrl->>UR: Delete monitor
            else Another Monitor has adopt-id or same ready status.id
                Ctrl->>Ctrl: Skip deletion (adopted)
            end
        else prune=false
            Ctrl->>Ctrl: Skip deletion (prune disabled)
        end
        
        Ctrl->>K8s: Remove finalizer
    end
    
    K8s->>K8s: Delete resource from etcd
```

## Controller Watches

Each controller watches specific resources:

```mermaid
graph LR
    subgraph "AccountController"
        AC[Watches: Account<br/>Updates: Account.Status]
    end
    
    subgraph "ContactController"
        CC[Watches: Contact<br/>Reads: Account<br/>Updates: Contact.Status]
    end
    
    subgraph "MonitorController"
        MC[Watches: Monitor<br/>Reads: Account, Contact<br/>Creates: Secret/ConfigMap<br/>Updates: Monitor.Status]
    end
    
    subgraph "MaintenanceWindowController"
        MWC[Watches: MaintenanceWindow<br/>Reads: Account, Monitor<br/>Updates: MaintenanceWindow.Status]
    end

    subgraph "MonitorGroupController"
        MGC[Watches: MonitorGroup and Monitor<br/>Reads: Account, Monitor<br/>Updates: MonitorGroup.Status]
    end

    subgraph "SlackIntegrationController"
        SIC[Watches: SlackIntegration<br/>Reads: Account, Secret<br/>Updates: SlackIntegration.Status]
    end
    
    subgraph "IngressController"
        IC[Watches: Ingress<br/>networking.k8s.io<br/>Creates: Monitor CRD<br/>No Status Updates]
    end
```

## External API Interactions

### Authentication Flow

```mermaid
graph LR
    Account[Account Resource] -->|references| Secret[Secret with apiKey]
    Controller[Controller] -->|reads| Secret
    Controller -->|uses apiKey| URAPI[UptimeRobot API]
    
    URAPI -->|validates| Key{Valid Key?}
    Key -->|Yes| Success[API Operations]
    Key -->|No| Error[Reconcile error<br/>Account controller sets Ready=false]
```

### Rate Limiting

UptimeRobot API has rate limits:
- Controllers do not implement custom client-side throttling logic in this repo
- `syncInterval` controls reconciliation frequency
- Failed reconciliations are retried with controller-runtime rate-limited backoff
- Check controller logs for rate limit errors

## Component Interactions

### Heartbeat URL Publishing

For Heartbeat-type monitors, the controller publishes the generated URL:

```mermaid
graph TB
    Monitor[Monitor<br/>type: Heartbeat] -->|creates in UR| UR[UptimeRobot API]
    UR -->|returns| URL[Heartbeat URL]
    Controller[MonitorController] -->|publishes| Target
    
    Target -->|to Secret| Secret[Secret<br/>heartbeatURL key]
    Target -->|or ConfigMap| CM[ConfigMap<br/>heartbeatURL key]
    
    App[Application/CronJob] -->|reads| Target
    App -->|sends HTTP GET| URL
```

### Monitor Adoption

Adopting existing monitors preserves history and avoids duplicates:

```mermaid
graph TD
    Existing[Existing Monitor<br/>in UptimeRobot] -->|has ID| ID[Monitor ID: 123456789]
    
    User[User] -->|creates| K8sRes[Monitor Resource<br/>with annotation]
    K8sRes -->|uptimerobot.com/adopt-id: 123456789| Annotation
    
    Controller[MonitorController] -->|reads annotation| Annotation
    Controller -->|verifies| Existing
    Controller -->|updates| Existing
    Controller -->|stores ID| Status[Monitor.Status.ID]
    
    User2[User] -->|can remove annotation<br/>after adoption| K8sRes
    
    style Existing fill:#e1f5ff
    style K8sRes fill:#ffe1e1
```

## Webhook Configuration

The operator currently uses validating webhooks (no mutating/defaulting webhook):

```mermaid
graph LR
    K8s[Kubernetes API] -->|validates create/update| VWC[ValidatingWebhookConfiguration]
    VWC --> AccountWH[Account validating webhook]
    VWC --> ContactWH[Contact validating webhook]

    AccountWH -->|enforces| AccountRule[Only one default Account]
    ContactWH -->|enforces| ContactRule[Only one default Contact]

    CertMgr[cert-manager] --> Cert[TLS certificate]
    Cert --> WebhookSvc[webhook-service]
    WebhookSvc --> VWC
```

### Certificate Management

- Certificates are managed by cert-manager (dependency)
- Auto-renewal before expiration
- Webhook unavailable if certificate invalid
- See [Troubleshooting](troubleshooting.md#webhook-certificate-issues) for issues

## Namespace Scope

```mermaid
graph TB
    subgraph "Cluster Scope"
        Account[Account<br/>Cluster-scoped]
        Contact[Contact<br/>Cluster-scoped]
    end
    
    subgraph "Namespace A"
        MonitorA[Monitor]
        MWA[MaintenanceWindow]
        MGA[MonitorGroup]
        SIA[SlackIntegration]
    end
    
    subgraph "Namespace B"
        MonitorB[Monitor]
        MWB[MaintenanceWindow]
        MGB[MonitorGroup]
        SIB[SlackIntegration]
    end
    
    MonitorA --> Account
    MonitorA --> Contact
    MWA --> Account
    MGA --> Account
    SIA --> Account
    MonitorB --> Account
    MonitorB --> Contact
    MWB --> Account
    MGB --> Account
    SIB --> Account
    
    MWA -.references.-> MonitorA
    MWB -.references.-> MonitorB
    
    MWA -.cannot reference.-> MonitorB
    MWB -.cannot reference.-> MonitorA
    
    style Account fill:#e1f5ff
```

**Key Points:**
- Account and Contact are cluster-scoped (shared across namespaces)
- Monitor, MaintenanceWindow, MonitorGroup, and SlackIntegration are namespace-scoped
- Cross-namespace references are not supported for namespaced resources
- MaintenanceWindow can only reference Monitors in the same namespace

## Status Fields

```mermaid
graph TD
    Account[Account.Status<br/>ready, email, alertContacts]
    Contact[Contact.Status<br/>ready, id]
    Monitor[Monitor.Status<br/>ready, id, type, status,<br/>heartbeatURL + publish target fields]
    MW[MaintenanceWindow.Status<br/>ready, id, monitorCount]
    MG[MonitorGroup.Status<br/>ready, id, monitorCount, lastReconciled]
    SI[SlackIntegration.Status<br/>ready, id, type]
```

This project does **not** define a `status.conditions` array on these CRDs; status is represented by resource-specific fields (primarily `ready` and IDs).

## See Also

- [Troubleshooting Guide](troubleshooting.md) - Diagnose and fix common issues
- [API Reference](api-reference.md) - Complete CRD field documentation
- [Getting Started](getting-started.md) - Quick start tutorial
