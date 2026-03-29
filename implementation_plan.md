# Architecture Plan: MongoDB Migration + Authentication

## Overview

Two parallel tracks:
1. **Replace PostgreSQL** with a MongoDB Replica Set (HA, persistent storage)
2. **Add Authentication** via a new `authservice` with JWT tokens

---

## 1. MongoDB on Kubernetes

### Why MongoDB (vs alternatives)

| Database | Replication | K8s Fit | Complexity |
|---|---|---|---|
| **MongoDB** | Built-in Replica Set | Good (StatefulSet) | Medium |
| CockroachDB | Automatic (Raft) | Excellent (operator) | Low — easiest HA |
| PostgreSQL + Patroni | Leader election | Good (operator) | High |
| Cassandra | Peer-to-peer | Good | High |

> **Recommendation:** MongoDB Replica Set is the right choice for your use case. No sidecar needed — MongoDB handles replication natively via its own Replica Set protocol. A sidecar would only be needed for MongoDB Operator patterns, which is overkill here.

---

### StatefulSet Design (3 pods)

```
mongo-0  ← Primary (reads + writes)
mongo-1  ← Secondary (reads + replication)
mongo-2  ← Secondary (reads + replication)
```

**Resources needed:**

| K8s Resource | Purpose |
|---|---|
| `StatefulSet` (3 replicas) | Stable pod names + ordered startup |
| Headless Service (`clusterIP: None`) | DNS for pod-to-pod replica set communication |
| ClusterIP Service | Single stable endpoint for apps to connect |
| PVC per pod (via `volumeClaimTemplates`) | Independent persistent storage per pod |
| ConfigMap | MongoDB config (`mongod.conf`) |
| Secret | MongoDB root credentials |
| Init Container (or Job) | Bootstrap replica set: runs `rs.initiate()` |

**Stable DNS names (headless service):**
```
mongo-0.mongo-headless.<namespace>.svc.cluster.local
mongo-1.mongo-headless.<namespace>.svc.cluster.local
mongo-2.mongo-headless.<namespace>.svc.cluster.local
```

**Replica Set Connection String (for apps):**
```
mongodb://mongo-0.mongo-headless:27017,mongo-1.mongo-headless:27017,mongo-2.mongo-headless:27017/?replicaSet=rs0&authSource=admin
```

**Storage:**
- Each pod gets its own PVC (e.g., 5Gi each) via `volumeClaimTemplates`
- StorageClass must support `ReadWriteOnce` (standard EBS on AWS)

---

## 2. Cartservice Migration (PostgreSQL → MongoDB)

### Schema Change

| Current (PostgreSQL) | New (MongoDB) |
|---|---|
| `carts` table with `user_id`, `product_id`, `quantity` | `carts` collection with document per user |

**MongoDB cart document structure:**
```json
{
  "_id": "user-session-id",
  "items": [
    { "productId": "OLJCESPC7Z", "quantity": 2 },
    { "productId": "66VCHSJNUP", "quantity": 1 }
  ],
  "updatedAt": "2026-03-29T..."
}
```

This is actually a **better fit for MongoDB** than PostgreSQL — a cart is naturally a document, not a relational join.

### Code Changes Required in cartservice
- Replace `Npgsql` NuGet package with `MongoDB.Driver`
- Rewrite [PostgresCartStore.cs](file:///c:/Users/ashbi/OneDrive/Desktop/chriss/HipsterShop/src/cartservice/src/cartstore/PostgresCartStore.cs) → `MongoCartStore.cs`
- Update env vars: remove `DB_HOST/USER/PASS/NAME`, add `MONGO_URI`

---

## 3. Authentication Service

### Architecture Decision: Separate `authservice` ✅

Create a new Go (or Node.js) microservice `authservice` rather than adding auth to the frontend. This is the correct microservices pattern — auth is a cross-cutting concern.

```
Browser → Gateway → authservice  (login/signup/validate)
                 → frontend      (pages, uses auth token)
                 → cartservice   (protected, validates JWT)
```

### Auth Flow

```
SIGNUP:
  POST /api/auth/signup { email, password }
  → authservice hashes password (bcrypt, cost=12)
  → stores user in MongoDB users collection
  → returns JWT token (httpOnly cookie)

LOGIN:
  POST /api/auth/login { email, password }
  → authservice fetches user, compares bcrypt hash
  → issues JWT (signed with secret, 24h expiry)
  → returns as httpOnly cookie

PROTECTED REQUEST:
  GET /api/cart/get (Cookie: token=<jwt>)
  → gateway forwards to cartservice
  → cartservice (or gateway) validates JWT
  → uses userId from JWT claims
```

### MongoDB Users Collection
```json
{
  "_id": "uuid",
  "email": "user@example.com",
  "passwordHash": "$2a$12$...",  ← bcrypt, NEVER store plaintext
  "createdAt": "2026-03-29T...",
  "name": "John Doe"
}
```

### Security Best Practices
- Passwords: **bcrypt** with cost factor 12 (industry standard)
- Tokens: **JWT** signed with `HS256` or `RS256`, stored in **httpOnly + Secure cookie** (not localStorage — prevents XSS)
- Token expiry: 24h access token, optionally a 7d refresh token
- Rate limiting: add at the gateway level on `/api/auth/*` routes
- HTTPS: enforce at HAProxy/gateway level (terminate TLS there)

### Token Validation Strategy

**Option A — Authservice validates (simplest):**
Each protected service calls `authservice` to validate token on every request. Simple but adds latency.

**Option B — Shared secret validation (recommended):**
Each service validates the JWT locally using a shared secret (stored as K8s Secret). No extra network call.

---

## 4. Gateway Route Changes

Add new HTTPRoutes to [kgateway.yaml](file:///c:/Users/ashbi/OneDrive/Desktop/chriss/HipsterShop/kubernetes-manifests/kgateway.yaml):

| External path | Rewrites to | Service |
|---|---|---|
| `POST /api/auth/login` | `/login` | authservice:8081 |
| `POST /api/auth/signup` | `/signup` | authservice:8081 |
| `POST /api/auth/logout` | `/logout` | authservice:8081 |
| `GET /api/auth/me` | `/me` | authservice:8081 |

---

## 5. Frontend Changes for Login

The frontend needs:
1. **New HTML templates**: `login.html`, `signup.html`
2. **New routes** in [main.go](file:///c:/Users/ashbi/OneDrive/Desktop/chriss/HipsterShop/src/frontend/main.go): `GET /login`, `POST /login`, `GET /signup`, `POST /signup`
3. **New RPC calls** in [rpc.go](file:///c:/Users/ashbi/OneDrive/Desktop/chriss/HipsterShop/src/frontend/rpc.go): POST to `/api/auth/login`, `/api/auth/signup`
4. **Session middleware update**: read userId from JWT cookie instead of generating random session ID
5. **Navbar**: show "Login" / user email + "Logout" based on auth state

---

## 6. Observability & Failure Handling

### MongoDB Pod Failure Scenarios

| Failure | What happens | Recovery |
|---|---|---|
| Secondary pod down | Primary continues serving; replica set runs with 2/3 nodes | Kubernetes restarts pod; it rejoins and resyncs |
| Primary pod down | Replica set triggers **automatic election** in ~10s; a secondary becomes primary | App connection string uses all 3 hosts, reconnects automatically |
| 2+ pods down | No primary can be elected (need majority = 2/3); reads/writes fail | Manual intervention or pod restart |
| PVC corrupt | Only that pod's data affected; other pods have copies | Restore from another pod or backup |

> **Key:** With 3 nodes, you can lose 1 and remain fully operational. That's the practical HA guarantee.

### Health Checks
- MongoDB readiness probe: TCP check on port 27017
- App-level circuit breaker: if MongoDB unreachable, return 503 rather than hanging

---

## 7. PostgreSQL vs MongoDB Trade-offs

| | PostgreSQL | MongoDB |
|---|---|---|
| **Data model** | Relational (tables, joins) | Document (JSON-like) |
| **HA in K8s** | Needs Patroni/operator | Built-in Replica Set |
| **Cart data fit** | Poor (multiple rows per cart) | Excellent (nested document) |
| **Schema changes** | Requires migrations | Schemaless, flexible |
| **ACID transactions** | Yes, full | Yes (since 4.0, multi-doc) |
| **Query language** | SQL (powerful) | Aggregation pipeline |
| **Replication** | Primary-standby (Patroni) | Replica Set (built-in) |
| **Operational complexity** | Higher in K8s | Medium |
| **Best for** | Complex relational data | Document/hierarchical data |

**Verdict for HipsterShop:** MongoDB is the better fit because the primary data (shopping carts) is document-shaped, and the built-in Replica Set is simpler to operate in Kubernetes than PostgreSQL HA solutions.

---

## Implementation Order (when you're ready)

1. Deploy MongoDB StatefulSet + headless service + PVCs
2. Init replica set (`rs.initiate()`)
3. Migrate cartservice (replace Npgsql → MongoDB.Driver)
4. Build and deploy `authservice`
5. Add auth HTTPRoutes to kgateway
6. Update frontend (login/signup pages, JWT handling)
7. Remove PostgreSQL StatefulSet
