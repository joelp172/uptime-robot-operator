# Architecture

This document provides a visual overview of the Uptime Robot Operator architecture, including CRD relationships, controller interactions, and data flows.

## System Overview

```mermaid
graph TB
    subgraph "Kubernetes Cluster"
        subgraph "Custom Resources"
            Account[Account<br/>Cluster-scoped]
            Contact[Contact]
            Monitor[Monitor]
            MW[MaintenanceWindow]
            MG[MonitorGroup]
            SI[SlackIntegration]
            Ingress[Ingress]
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
    IC -->|watches| Ingress
    
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
    
    AC -->|creates| Events
    CC -->|creates| Events
    MC -->|creates| Events
    MWC -->|creates| Events
```

## CRD Dependency Graph

```mermaid
graph LR
    subgraph "Core Resources"
        Account["Account<br/>(Cluster-scoped)<br/>Stores API Key"]
    end
    
    subgraph "Alert Configuration"
        Contact[Contact<br/>References Account<br/>Uses existing contact ID]
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
        Ingress[Ingress<br/>Auto-creates Monitors<br/>from Kubernetes Ingress]
    end
    
    Monitor --> Account
    Monitor -.optional.-> Contact
    Contact --> Account
    MW --> Account
    MW --> Monitor
    MG --> Account
    SI --> Account
    Ingress --> Monitor
    Monitor --> MG
    
    style Account fill:#e1f5ff
    style Monitor fill:#fff4e1
    style Contact fill:#f0f0f0
    style MW fill:#ffe1f5
```

### Dependency Rules

1. **Account** must be ready before dependent resources can sync
2. **Contact** requires a ready Account and must reference an existing contact ID from the Account status
3. **Monitor** requires a ready Account; Contacts are optional but must be ready if referenced
4. **MaintenanceWindow** requires a ready Account; referenced Monitors must exist (but don't need to be ready)
5. **Ingress** controller auto-creates Monitor resources

## Reconciliation Flow

```mermaid
sequenceDiagram
    participant K8s as Kubernetes API
    participant Ctrl as Controller
    participant Cache as Controller Cache
    participant UR as UptimeRobot API
    
    K8s->>Ctrl: Resource created/updated
    Ctrl->>Cache: Get resource
    Ctrl->>Ctrl: Check dependencies<br/>(Account ready, Contacts ready)
    
    alt Dependencies not ready
        Ctrl->>K8s: Update status: not ready<br/>Requeue after 30s
    else Dependencies ready
        Ctrl->>Ctrl: Get API key from Secret
        Ctrl->>UR: List monitors (if ID unknown)
        
        alt Resource has adopt-id annotation
            Ctrl->>UR: Get existing monitor by ID
            Ctrl->>Ctrl: Store ID in status
        end
        
        alt Monitor exists in UptimeRobot
            Ctrl->>UR: Get current state
            Ctrl->>Ctrl: Compare desired vs current state
            
            alt Drift detected
                Ctrl->>UR: Update monitor
                Ctrl->>K8s: Update status + create Event
            else No drift
                Ctrl->>K8s: Update status (ready=true)
            end
        else Monitor does not exist
            Ctrl->>UR: Create monitor
            Ctrl->>K8s: Update status with ID
            Ctrl->>K8s: Create Event (monitor created)
        end
        
        Ctrl->>K8s: Requeue after syncInterval<br/>(default: 24h)
    end
```

## Drift Detection

The operator continuously monitors for configuration drift between Kubernetes and UptimeRobot:

```mermaid
graph TD
    Start[Reconcile Trigger] --> GetK8s[Get Kubernetes Spec]
    GetK8s --> GetUR[Get UptimeRobot State]
    GetUR --> Compare{Drift Detected?}
    
    Compare -->|No| UpdateStatus[Update Status: Ready]
    Compare -->|Yes| LogDrift[Log Drift Details]
    LogDrift --> UpdateUR[Update UptimeRobot]
    UpdateUR --> CreateEvent[Create Event]
    CreateEvent --> UpdateStatus
    
    UpdateStatus --> Schedule[Schedule Next Reconcile<br/>After syncInterval]
    
    style Compare fill:#ffe1e1
    style UpdateUR fill:#e1ffe1
```

### Drift Detection Frequency

Controlled by the `syncInterval` field (default: 24h):
- Lower values = faster drift detection, more API calls
- Higher values = less API load, slower drift detection

## Finalizer and Deletion Flow

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
            Ctrl->>K8s: List monitors with same ID<br/>(check for adoption)
            
            alt No other Monitor adopts this ID
                Ctrl->>UR: Delete monitor
            else Another Monitor adopted this ID
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
    
    subgraph "IngressController"
        IC[Watches: Ingress<br/>Creates: Monitor<br/>No Status Updates]
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
    Key -->|No| Error[401 Unauthorized<br/>Account not ready]
```

### Rate Limiting

UptimeRobot API has rate limits:
- Controllers respect rate limits automatically
- `syncInterval` controls reconciliation frequency
- Failed requests trigger exponential backoff
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

The operator uses webhooks for validation and defaulting:

```mermaid
graph LR
    K8s[Kubernetes API] -->|validates| Webhook[ValidatingWebhook]
    Webhook -->|cert-manager| Cert[TLS Certificate]
    
    Cert -->|auto-renewed| CM[cert-manager]
    Webhook -->|enforces| Rules[Validation Rules:<br/>- Account reference valid<br/>- Contact has ID<br/>- Monitor type valid]
    
    K8s -->|defaults| MutatingWebhook[MutatingWebhook]
    MutatingWebhook -->|sets defaults| Defaults[- syncInterval: 24h<br/>- prune: true<br/>- isDefault: false]
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
    end
    
    subgraph "Namespace A"
        MonitorA[Monitor]
        ContactA[Contact]
        MWA[MaintenanceWindow]
    end
    
    subgraph "Namespace B"
        MonitorB[Monitor]
        ContactB[Contact]
        MWB[MaintenanceWindow]
    end
    
    MonitorA --> Account
    ContactA --> Account
    MWA --> Account
    MonitorB --> Account
    ContactB --> Account
    MWB --> Account
    
    MWA -.references.-> MonitorA
    MWB -.references.-> MonitorB
    
    MonitorA -.cannot reference.-> ContactB
    MonitorB -.cannot reference.-> ContactA
    
    style Account fill:#e1f5ff
```

**Key Points:**
- Account is cluster-scoped (shared across namespaces)
- Monitor, Contact, MaintenanceWindow are namespace-scoped
- Cross-namespace references are not supported (except to Account)
- MaintenanceWindow can only reference Monitors in the same namespace

## Status Conditions

```mermaid
graph TD
    Start[Reconciliation Start] --> CheckAccount{Account Ready?}
    
    CheckAccount -->|No| NotReady1[Status: Ready=false<br/>Condition: AccountNotReady]
    CheckAccount -->|Yes| CheckContacts{Contacts Ready?}
    
    CheckContacts -->|No| NotReady2[Status: Ready=false<br/>Condition: ContactsNotReady]
    CheckContacts -->|Yes| CheckAPI{API Call Success?}
    
    CheckAPI -->|No| NotReady3[Status: Ready=false<br/>Condition: APIError]
    CheckAPI -->|Yes| Ready[Status: Ready=true<br/>ID populated]
    
    style NotReady1 fill:#ffe1e1
    style NotReady2 fill:#ffe1e1
    style NotReady3 fill:#ffe1e1
    style Ready fill:#e1ffe1
```

## See Also

- [Troubleshooting Guide](troubleshooting.md) - Diagnose and fix common issues
- [API Reference](api-reference.md) - Complete CRD field documentation
- [Getting Started](getting-started.md) - Quick start tutorial
