import {
  registerRoute,
  registerSidebarEntry,
} from '@kinvolk/headlamp-plugin/lib';
import { K8s } from '@kinvolk/headlamp-plugin/lib';

import KCListPage from './KCListPage';
import KCDetailPage from './KCDetailPage';

// ============================================================================
// Platform Kubernetes Plugin
//
// Provides KubernetesCluster management UI.
// ============================================================================

export const KCClass = K8s.makeCRClass({
  apiName: 'kubernetesclusters',
  isNamespaced: false,
  group: 'compute.cloud.platform',
  version: 'v1alpha1',
});

// --- Routes ---

registerRoute({
  path: '/kc',
  exact: true,
  name: 'KubernetesClusters',
  component: () => <KCListPage />,
});

registerRoute({
  path: '/kc/:name',
  exact: true,
  name: 'KubernetesClusterDetail',
  component: () => <KCDetailPage />,
});

// --- Sidebar ---

registerSidebarEntry({
  name: 'platform-compute-kc',
  label: 'Kubernetes Clusters',
  url: '/kc',
  parent: 'platform-compute',
});
