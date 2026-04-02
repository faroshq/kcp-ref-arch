# NeoCloud Console

The web console for the cloud platform, built on [Headlamp](https://headlamp.dev) with custom plugins that reshape it from a Kubernetes dashboard into the NeoCloud portal.

## What It Does

- Replaces Kubernetes-oriented navigation with cloud platform concepts (workspaces, VMs, clusters)
- Provides create/list/detail views for platform compute resources
- Integrates with kcp workspaces as the tenant boundary

## Plugins

| Plugin | Purpose |
|--------|---------|
| `platform-shell` | NeoCloud branding: custom logo, workspace picker instead of cluster chooser, hides K8s-native navigation |
| `platform-compute` | VirtualMachine views: list, create form, detail page with status/conditions |
| `platform-kubernetes` | KubernetesCluster views: list and detail pages |

## Quick Start

```bash
# Build and run NeoCloud console
make run-console

# Opens at http://localhost:4466/
```

## Development

```bash
# Install plugin dependencies
make install

# Build plugins
make build

# Clean build artifacts
make clean
```

Each plugin is a standard Headlamp plugin under `plugins/`:

```
plugins/
  platform-shell/           NeoCloud branding, navigation filtering, workspace picker
    src/index.tsx
    package.json
  platform-compute/         VirtualMachine CRUD
    src/
      index.tsx             Route + sidebar registration
      VMListPage.tsx        VM list with status, cores, memory, GPU columns
      VMCreatePage.tsx      Create form (cores, memory, disk, image, GPU, SSH key)
      VMDetailPage.tsx      Detail view with spec, status, conditions, related resources
    package.json
  platform-kubernetes/      KubernetesCluster views
    src/
      index.tsx             Route + sidebar registration
      KCListPage.tsx        Cluster list
      KCDetailPage.tsx      Cluster detail
    package.json
```

## Docker

The Dockerfile builds all plugins and copies them into the Headlamp base image:

```bash
docker build -t platform-console:latest .
docker run -p 4466:4466 platform-console:latest
```

The platform server reverse-proxies NeoCloud at `/console` when started with `--console-addr localhost:4466`.

## How It Connects

```
Browser → platform:9443/console → reverse proxy → NeoCloud:4466
                                                      │
                                                      ▼
                                                kcp API server
                                                (workspace-scoped)
```

NeoCloud talks directly to the kcp API server using the tenant's Bearer token. Each workspace appears as an isolated environment with only platform APIs visible.
