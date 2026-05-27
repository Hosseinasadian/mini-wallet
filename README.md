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

Services communicate **asynchronously** via a message broker. No direct service-to-service HTTP calls — each service publishes events and the Notification Service consumes them independently.

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
- Service discovery via Docker labels

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go |
| API style | REST, SSE |
| Authentication | JWT |
| Message Broker | RabbitMQ |
| Real-time | SSE + Redis Pub/Sub |
| Push Notifications | Firebase (FCM), OneSignal |
| API Gateway | Traefik |
| Containerization | Docker, Docker Compose |
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

### Continuous Integration

A CI pipeline is configured to run on every push:

- Automated build verification for all services
- Test execution per service
- Service-level validation before merge

---

## Project Structure

```
mini-wallet/
├── auth/               # Auth service
├── wallet/             # Wallet service
├── notification/       # Notification service
├── docs/               # Swagger docs service
├── docker-compose.yml
└── .github/
    └── workflows/      # CI pipeline definitions
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

## Future Improvements

- Kubernetes deployment and Helm chart
- Distributed tracing (OpenTelemetry)
- Centralized logging (Loki / ELK)
- Rate limiting at the gateway layer
- gRPC for internal service communication
- Service mesh integration
- Monitoring and observability stack (Prometheus + Grafana)

---

## Author

Backend engineer focused on Go, distributed systems, microservices, and scalable backend infrastructure.
