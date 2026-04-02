# Build plugins
FROM node:22-alpine AS plugin-builder

WORKDIR /build

# Build each plugin
COPY plugins/platform-shell/ plugins/platform-shell/
RUN cd plugins/platform-shell && npm install && npm run build

COPY plugins/platform-compute/ plugins/platform-compute/
RUN cd plugins/platform-compute && npm install && npm run build

COPY plugins/platform-kubernetes/ plugins/platform-kubernetes/
RUN cd plugins/platform-kubernetes && npm install && npm run build

# Final image — Headlamp with platform plugins baked in
FROM ghcr.io/headlamp-k8s/headlamp:latest

# Copy built plugins into the plugins directory (main.js + package.json)
COPY --from=plugin-builder /build/plugins/platform-shell/dist /headlamp/plugins/platform-shell/
COPY --from=plugin-builder /build/plugins/platform-shell/package.json /headlamp/plugins/platform-shell/package.json
COPY --from=plugin-builder /build/plugins/platform-compute/dist /headlamp/plugins/platform-compute/
COPY --from=plugin-builder /build/plugins/platform-compute/package.json /headlamp/plugins/platform-compute/package.json
COPY --from=plugin-builder /build/plugins/platform-kubernetes/dist /headlamp/plugins/platform-kubernetes/
COPY --from=plugin-builder /build/plugins/platform-kubernetes/package.json /headlamp/plugins/platform-kubernetes/package.json
