import {
  registerRoute,
  registerSidebarEntry,
} from '@kinvolk/headlamp-plugin/lib';
import { K8s } from '@kinvolk/headlamp-plugin/lib';

import VMListPage from './VMListPage';
import VMDetailPage from './VMDetailPage';
import VMCreatePage from './VMCreatePage';

// ============================================================================
// Platform Compute Plugin
//
// Provides VirtualMachine management UI:
// - List view with status, cores, memory, image, IP
// - Detail view with full spec, status, and KubeVirt VMI info
// - Create form for provisioning new VMs
// ============================================================================

// --- CRD Class ---
// This tells Headlamp how to interact with the VirtualMachine CRD.

export const VMClass = K8s.makeCRClass({
  apiName: 'virtualmachines',
  isNamespaced: false,
  group: 'compute.cloud.platform',
  version: 'v1alpha1',
});

// --- Routes ---

registerRoute({
  path: '/vm',
  exact: true,
  name: 'VirtualMachines',
  component: () => <VMListPage />,
});

registerRoute({
  path: '/vm/create',
  exact: true,
  name: 'CreateVirtualMachine',
  component: () => <VMCreatePage />,
});

registerRoute({
  path: '/vm/:name',
  exact: true,
  name: 'VirtualMachineDetail',
  component: () => <VMDetailPage />,
});

// --- Sidebar Navigation ---

registerSidebarEntry({
  name: 'platform-compute',
  label: 'Compute',
  icon: 'mdi:server',
  url: '/vm',
  parent: null,
});

registerSidebarEntry({
  name: 'platform-compute-vms',
  label: 'Virtual Machines',
  url: '/vm',
  parent: 'platform-compute',
});
