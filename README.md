# HipsterShop - Microservices E-Commerce Platform
.
A cloud-native, Kubernetes-deployed microservices e-commerce platform showcasing modern DevOps practices.

## Quick Overview

**HipsterShop** is a 12-microservice application with MongoDB database, deployed on Kubernetes using Helm charts and Kustomize. It demonstrates:
- Service-to-service communication via Gateway API
- Server-side rendering (SSR) frontend
- JWT + session cookie authentication
- Database-per-service architecture
- Horizontal Pod Autoscaling (HPA)
- Stateful MongoDB replica set

## Architecture

```
External User (Browser)
    ↓
Kubernetes Gateway (kgateway) - Port 80
    ↓
Frontend Service:80 (SSR Go app)
    ├─ Calls Gateway internally for data
    ├─ /api/products → ProductCatalogService
    ├─ /api/cart → CartService
    ├─ /api/currency → CurrencyService
    ├─ /api/checkout → CheckoutService
    └─ /api/assistant → AssistantService (Gemini AI)
    
MongoDB Replica Set (3 pods)
    ├─ auth_db (Authentication)
    ├─ cart_db (Shopping cart)
    ├─ catalog_db (Products)
    ├─ order_db (Orders)
    ├─ payment_db (Payments)
    └─ notification_db (Emails)
```

## 12 Microservices

| Service | Language | Port | Database |
|---------|----------|------|----------|
| Frontend | Go | 8080 | - |
| AuthService | Go | 8081 | auth_db |
| ProductCatalogService | Go | 3550 | catalog_db |
| CartService | C# | 7070 | cart_db |
| CheckoutService | Go | 5050 | order_db |
| PaymentService | Node.js | 50051 | payment_db |
| ShippingService | - | 50051 | - |
| CurrencyService | Node.js | 7000 | - |
| EmailService | Python | 8080 | - |
| AdService | Java | 9555 | - |
| RecommendationService | Python | 8080 | - |
| AssistantService | Python | 8080 | - |

## Kubernetes Deployment

### Prerequisites
- Kubernetes cluster (1.24+)
- Helm 3.x
- kubectl configured
- kgateway controller installed

### Deploy with Helm

```bash
# Create namespace and deploy
helm install hipstershop ./Helm -n hipster --create-namespace

# Check deployment status
kubectl get pods -n hipster
kubectl get jobs -n hipster

# Port forward to access frontend
kubectl port-forward -n hipster svc/frontend 8080:80
# Visit: http://localhost:8080
```

### Deploy with Kustomize

```bash
kubectl apply -k kubernetes-manifests/
```

## Key Kubernetes Components

### Deployments (12 microservices)
- Replicas: 2 each (High availability)
- Rolling update strategy: maxUnavailable=1, maxSurge=1
- Health checks: readinessProbe + livenessProbe

### StatefulSet (MongoDB)
- Replicas: 3 (Replica set rs0)
- Persistent volumes: 5Gi per pod
- Storage class: nfs

### Services
- Type: ClusterIP (internal communication)
- DNS-based service discovery
- Service addresses injected via ConfigMaps

### Headless Service
- MongoDB replica set coordination
- Direct pod-to-pod communication

### ConfigMaps
- `hipster-config`: Application settings
- `service-addresses`: Microservice discovery
- `service-ports`: Port configuration
- `mongodb-config`: Database names and collections

### Secrets
- `app-secrets`: JWT_SECRET, Gemini API key
- `mongodb-users`: Per-database credentials
- `mongodb-root`: Root credentials

### Gateway API
- `Gateway`: kgateway class, listens on port 80
- `HTTPRoutes`: Path-based routing to services
- URL rewriting for internal API paths

### Jobs (Initialization)
1. `mongo-init-replicaset` - Initialize replica set and create users
2. `mongo-seed-products` - Load sample products
3. `mongo-migrate-shopdb-data` - Migrate legacy data

### HPA
- Frontend auto-scales 2-5 replicas
- Target: 70% CPU, 80% memory utilization

## Authentication Flow

**Two Cookie Types:**
1. **session-id** (UUID): Track guest users
2. **auth_token** (JWT): Authenticate logged-in users

**Flow:**
```
1. User visits → Gets session-id cookie (UUID)
2. User logs in → Gets auth_token cookie (JWT)
3. Frontend middleware validates JWT signature
4. Protected routes require isAuthenticated=true
5. UserID extracted from JWT claims
```

## Data Flow (Example: View Products)

```
Browser Request: GET /
    ↓
Frontend Handler (homeHandler)
    ├─ Calls: fe.getProducts()
    ├─ Internally: GET /api/products (to Gateway)
    ├─ Gateway routes: /api/products → ProductCatalogService:3550
    ├─ ProductCatalogService queries: catalog_db.products
    ├─ Returns: JSON product list
    ├─ Frontend renders: HTML with products embedded
    └─ Sends: Complete HTML to browser
    ↓
Browser receives: Pre-rendered HTML (SSR)
    └─ Network tab shows: Only GET / (API calls invisible!)
```

## Server-Side Rendering (SSR)

- Frontend renders HTML on backend (not browser)
- All API calls happen internally (pod-to-pod)
- Browser receives complete HTML
- No JavaScript API calls visible in DevTools
- Benefits: SEO-friendly, faster perceived load

## Environment Variables

```bash
# Frontend
GATEWAY_ADDR=hipstershop-gateway.hipster.svc.cluster.local:80
JWT_SECRET=team4hipstershopsecret
PORT=8080

# AssistantService
GEMINI_API_KEY=<your-key>
GEMINI_MODEL=gemini-2.5-flash

# CartService
MONGO_URI=mongodb://cart_user:password@mongodb-0,mongodb-1,mongodb-2/cart_db?replicaSet=rs0
```

## Directory Structure

```
HipsterShop/
├── Helm/                         # Helm charts
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
│       ├── configmaps.yaml
│       ├── secrets.yaml
│       ├── namespace.yaml
│       ├── services/             # Service deployments
│       └── database/             # MongoDB templates
├── kubernetes-manifests/         # Raw Kubernetes YAML
│   ├── base/
│   ├── services/
│   ├── database/
│   ├── gateway/                  # Gateway API routes
│   ├── HPA/                       # Autoscaling
│   └── kustomization.yaml
├── src/                          # Source code
│   ├── frontend/                 # Go (SSR)
│   ├── authservice/              # Go
│   ├── cartservice/              # C#
│   ├── assistantservice/         # Python (Gemini AI)
│   ├── paymentservice/           # Node.js
│   └── ...12 services total
└── image-scanning/               # Security scanning
```

## Common Commands

```bash
# View deployments
kubectl get deployments -n hipster
kubectl get statefulsets -n hipster
kubectl get services -n hipster
kubectl get hpa -n hipster

# Check logs
kubectl logs -f -n hipster deployment/frontend
kubectl logs -f -n hipster job/mongo-seed-products

# Scale manually
kubectl scale deployment frontend --replicas=5 -n hipster

# Debug pod
kubectl exec -it -n hipster pod/frontend-abc123 -- /bin/sh

# Port forward
kubectl port-forward -n hipster svc/frontend 8080:80
kubectl port-forward -n hipster svc/mongodb 27017:27017

# Delete deployment
helm uninstall hipstershop -n hipster
```

## Key Features

 **Microservices**: 12 independent services  
 **Kubernetes-Native**: StatefulSets, Jobs, HPA, Gateway API  
 **High Availability**: Multiple replicas, replica set  
 **Security**: Database-per-service, JWT auth, Secrets management  
 **Scalability**: HPA for frontend, horizontal scaling  
 **Modern Stack**: Helm, Kustomize, Gateway API, Gemini AI  
 **Observability**: Health checks, logging, resource management  

## Architecture Highlights

### Service Discovery
- Kubernetes DNS: `servicename.namespace.svc.cluster.local`
- ConfigMap-based service address injection
- Automatic load balancing via Service ClusterIP

### Communication Pattern
- **External**: Browser → Gateway → Frontend (Server-side rendering)
- **Internal**: Frontend → Gateway → Microservices (Pod-to-pod)
- **Database**: All services → MongoDB replica set

### Database Pattern
- Database-per-service architecture
- Each service has own database + restricted user
- SCRAM-SHA-256 authentication
- MongoDB replica set for durability

### Deployment Strategy
- RollingUpdate: maxUnavailable=1, maxSurge=1
- Zero-downtime deployments
- Graceful pod termination (terminationGracePeriodSeconds)


