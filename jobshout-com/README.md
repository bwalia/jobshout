# JobShout.com

AI-native global employment marketplace (Rust + Next.js + Swift).

This package lives in the JobShout ecosystem monorepo but is **deployed separately**
from the Go agent platform.

```bash
make docker-up && make migrate && make seed
make api   # :8088
make web   # :3010
```

See [docs/architecture.md](docs/architecture.md) and [docs/development.md](docs/development.md).
