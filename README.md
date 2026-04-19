# Jobshout

Mission control center for AI teams. Create agents, build teams, assign projects, track work, and automate workflows.

## Architecture

```
jobshout/
  server/          Go API backend (chi, pgx, JWT)
  web/nextjs/      Next.js 14+ frontend (ShadCN, React Flow, dnd-kit)
  docker-compose.yml
```

## Quick Start

```bash
# Copy environment file
cp .env.example .env

# Start all services
docker compose up --build

# Services:
# - API:        http://localhost:8080
# - UI:         http://localhost:3001
# - PostgreSQL: localhost:5432
# - MinIO:      http://localhost:9000 (console: http://localhost:9001)
```

## Development

### Backend (Go)

```bash
cd server
go run ./cmd/server
```

### Frontend (Next.js)

```bash
cd web/nextjs
npm install
npm run dev
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | /api/v1/auth/register | Register new user |
| POST | /api/v1/auth/login | Login |
| POST | /api/v1/auth/refresh | Refresh JWT |
| GET | /api/v1/auth/me | Current user |
| GET/POST | /api/v1/agents | List/Create agents |
| GET/PUT/DELETE | /api/v1/agents/:id | Get/Update/Delete agent |
| GET/POST | /api/v1/projects | List/Create projects |
| GET/PUT/DELETE | /api/v1/projects/:id | Get/Update/Delete project |
| GET/POST | /api/v1/tasks | List/Create tasks |
| GET/PUT/DELETE | /api/v1/tasks/:id | Get/Update/Delete task |
| GET/PUT | /api/v1/organizations/:id | Get/Update org |
| PUT | /api/v1/organizations/:id/chart | Update org chart |
| GET | /api/v1/marketplace | List marketplace agents |
| POST | /api/v1/marketplace/:id/import | Import marketplace agent |
| GET | /api/v1/metrics/* | Various metrics endpoints |
| WS | /api/v1/ws | WebSocket for real-time updates |
