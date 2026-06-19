# Mini Wallet — Microservices Digital Wallet Platform

A production-oriented digital wallet platform built with Go, demonstrating backend engineering skills around distributed systems, event-driven architecture, real-time notifications, and DevOps practices.

---

## Architecture Overview

```
                        ┌─────────────────────┐
                        │       Client        │
                        │   Web / Mobile App  │
                        └──────────┬──────────┘
                                   │
                                   ▼
                        ┌─────────────────────┐
                        │   Traefik Gateway   │
                        │  Reverse Proxy · Routing · TLS
                        └──────┬──────────────┘
                               │
          ┌────────────────────┼─────────────────────┐
          │                    │                     │
          ▼                    ▼                     ▼
  ┌───────────────┐   ┌────────────────┐   ┌────────────────┐   ┌────────────────┐
  │  Auth Service │   │ Wallet Service │   │  Notification  │   │  Docs Service  │
  │               │   │               │   │    Service     │   │                │
  │ Login/Register│   │ Balance        │   │ Event Consumer │   │ Swagger/OpenAPI│
  │ JWT · Sessions│   │ Transfers      │   │ SSE · Firebase │   │ API Explorer   │
  │ Device Mgmt   │   │ Transactions   │   │ OneSignal      │   │                │
  └───────┬───────┘   └────────┬───────┘   └───────▲────────┘   └────────────────┘
          │                    │                   │
          └─────────┬──────────┘                   │
                    │ publishes events              │ consumes events
                    ▼                               │
          ┌─────────────────────┐                  │
          │   Message Broker    │──────────────────┘
          │  (Event-Driven Bus) │
          └─────────────────────┘
                    │
        ┌───────────┼───────────┐
        ▼           ▼           ▼
      [SSE]    [Firebase]  [OneSignal]
```

---

## Services

### Auth Service

Handles user identity, session management, and security event publishing.

- User registration and login
- JWT-based authentication
- Session and device management with new device detection
- Publishes security events for downstream consumption

**Example events published:**
```
auth.user.logged_in
auth.user.new_device
auth.user.password_changed
```

---

### Wallet Service

Manages financial operations and wallet lifecycle.

- Wallet creation and balance tracking
- Money transfers between wallets
- Transaction history
- Publishes financial events for real-time notification

**Example events published:**
```
wallet.balance_changed
wallet.transfer.created
wallet.transfer.completed
```

---

### Notification Service

Consumes events from the broker and delivers real-time notifications to users via multiple channels.

- **SSE (Server-Sent Events)** — real-time push to web clients
- **Firebase (FCM)** — push notifications for mobile
- **OneSignal** — cross-platform push delivery
- Hub-based SSE architecture with Redis Pub/Sub for multi-instance sync

**Example notifications delivered:**
```
New login detected on an unknown device
Wallet balance updated
Transfer completed successfully
Security alert triggered
```

---

### Docs Service

Centralized API documentation for all services.

- Swagger/OpenAPI documentation
- Interactive API testing interface
- Per-service endpoint documentation

---

## Communication Strategy

Services communicate asynchronously via a message broker for event-driven flows, with zero direct HTTP calls between domains. However, low-latency, point-to-point internal queries (such as fetching user context or identity checks) are strictly handled via synchronous **gRPC** calls (e.g., Notification Service calling Auth Service).

**Example flow:**
```
1. User logs in on a new device
2. Auth Service publishes  →  auth.user.new_device
3. Notification Service consumes the event
4. User receives real-time alert via SSE and push notification
```

This pattern improves:
- **Scalability** — services scale independently
- **Fault tolerance** — producer is unaffected if the consumer is down
- **Service isolation** — no tight coupling between domains
- **Async processing** — heavy notification work offloaded from the request path

---

## API Gateway

**Traefik** is used as the API gateway and handles:

- Reverse proxying and request routing
- Load balancing across service instances
- Centralized entry point for all client traffic
- TLS/HTTPS termination
- Service discovery via Kubernetes Services and CRDs (IngressRoute, Middlewares)

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go |
| API style | REST, gRPC, SSE |
| Authentication | JWT |
| Message Broker | RabbitMQ |
| Real-time | SSE + Redis Pub/Sub |
| Push Notifications | Firebase (FCM), OneSignal |
| Containerization | Docker, Docker Compose |
| Orchestration | Kubernetes (Kind), Helm v3 |
| API Gateway / Ingress | Traefik v3 |
| API Docs | Swagger / OpenAPI |
| CI | GitHub Actions |

---

## DevOps & Infrastructure

### Dockerized Infrastructure

All services are fully containerized with Docker and orchestrated via Docker Compose. Each service runs in an isolated container with shared networking and environment-based configuration.

```
Benefits:
  - Consistent environments across dev and prod
  - Easy multi-service orchestration
  - Service isolation and independent restarts
  - Straightforward horizontal scaling
```

### Kubernetes & Helm Orchestration
The production-ready deployment is fully orchestrated using Kubernetes (via Kind for local development) and packaged with Helm v3.
- **Microservices Directory Structure:** Templates are strictly isolated per microservice domain inside the Helm chart (`templates/auth-service`, `templates/wallet-service`, etc.).
- **Traefik Ingress Controller:** Handles ingress traffic routing inside the cluster on standard web ports (`80`/`443`) using Traefik `IngressRoute` and `Middleware` CRDs.
- **Local Multi-Cluster Port Precedence:** Local setups require port `80` to be completely free (ensure other local control planes or local proxies are stopped before booting).

### Continuous Integration

A CI pipeline is configured to run on every push:

- Automated build verification for all services
- Test execution per service
- Service-level validation before merge

---

## Setup & Installation

Follow these steps to spin up the entire microservices architecture locally on your machine.

### Prerequisites
- Docker & Docker Compose installed.
- **Kind** (Kubernetes in Docker) and **Helm v3** installed.
- Ensure ports `80` and `443` are **completely free** on your host system. Stop any active local Nginx, Apache, or alternative Kind control planes (e.g., Palphone) before proceeding:
  ```bash
  sudo lsof -i :80
  ```

### 1. Local Domain Configuration
Add the following local routing rules to your `/etc/hosts` file:

```plaintext
127.0.0.1 api.wallet.local
127.0.0.1 traefik.wallet.local
```

### 2. Bootstrapping the Kubernetes Cluster
We leverage a single-command routine via the Makefile to wipe any old state, provision a fresh Kind cluster mapped to ports 80/443, inject the Traefik Ingress controller with updated schemas, and fully deploy the packaged Helm chart.

Run the following command from the project root:

```bash
make k8s-cluster-rebuild
```

### 3. Verifying System Health
Check the orchestration status to ensure all multi-instance deployments (auth, wallet, notification, docs) along with the Traefik pods are fully running:

```bash
make k8s-status
```

### 4. Testing Endpoints
Once all pods report a `1/1 Running` status, you can hit the production endpoints directly via your host browser or Postman without specifying any port:

- **Auth Microservice Live Check:** http://api.wallet.local/auth/live
- **Traefik Gateway Dashboard:** http://traefik.wallet.local/dashboard/

---

## Project Structure

```
mini-wallet/
├── auth/                 # Auth service (Go)
├── wallet/               # Wallet service (Go)
├── notification/         # Notification service (Go)
├── docs/                 # Swagger docs service (Go)
├── deployment/           # 📂 Infrastructure and Orchestration
│   └── k8s/
│       ├── kind-config.yaml
│       ├── traefik-values.yaml
│       └── mini-wallet-chart/  # 📑 Production Helm Chart (Organized per service)
├── docker-compose.yml
└── .github/
    └── workflows/        # CI pipeline definitions (including Helm linter)
```

---

## Engineering Highlights

This project reflects hands-on experience with:

- **Microservices architecture** — domain-driven service boundaries with clear ownership
- **Event-driven design** — decoupled services communicating via a broker
- **Real-time delivery** — Hub-based SSE with Redis Pub/Sub for horizontal scalability
- **Multi-channel notifications** — SSE for web, FCM and OneSignal for mobile
- **Secure authentication** — JWT lifecycle, device fingerprinting, new device detection
- **API gateway patterns** — centralized routing and TLS with Traefik
- **Containerized deployments** — fully Docker-native with Compose orchestration
- **CI pipelines** — automated build and test validation

---

## Observability & Dashboards

We provide role‑based dashboards to monitor system health, debug issues, and support customer inquiries. All services expose structured logs (JSON) and HTTP metrics via OpenTelemetry → Loki / Prometheus → Grafana.

### HTTP Metrics (Prometheus)

- Request rate (req/s) – total & per service
- Average latency (ms) – total & per service
- Error rate split by HTTP status class:
    - `4xx` (client errors – bad request, auth failures)
    - `5xx` (server errors – internal faults)
- Per‑route metrics: rate, latency, error rate
- Latency percentiles (p50, p95, p99) using histograms

### Grafana Dashboards

#### For Developers (three dashboards)

| Dashboard | Purpose |
|-----------|---------|
| **Dev Metrics Dashboard** | Request rate, latency, error rate (4xx/5xx) by service & route; latency percentiles. |
| **Dev All Logs Dashboard** | All JSON logs filtered by `app`, `layer` (main, http, service, repository, mysql) and `level` (DEBUG, INFO, WARN, ERROR). |
| **Dev Request Logs Dashboard** | Filter logs by a specific `request_id` – trace a single transaction across services. |

#### For Support (one dashboard)

| Dashboard | Purpose |
|-----------|---------|
| **Support Logs Dashboard** | Simplified view: only `main`, `http`, `service` layers; only `INFO`, `WARN` levels. No DEBUG/ERROR noise. Supports `request_id` and `app` filters. |

### Workflow with Dashboards

1. Developer notices high error rate or latency in **Dev Metrics**.
2. Opens **Dev Request Logs** with the failing `request_id` to see the full cross‑service trace.
3. Dives into **Dev All Logs** for deeper debugging (e.g., repository or MySQL layer).
4. If a user reports an issue, support looks up the `request_id` in **Support Logs** – minimal technical details, fast answer.

All dashboard JSON definitions are versioned in the repository (see `grafana/` folder).

---

## Author

Backend engineer focused on Go, distributed systems, microservices, and scalable backend infrastructure.
