import { useNavigate } from 'react-router-dom';
import {
  SectionBox,
  SimpleTable,
  StatusLabel,
  StatusLabelProps,
} from '@kinvolk/headlamp-plugin/lib/components/common';
import { VMClass } from './index';

// Maps VM phase to Headlamp status label type.
function phaseToStatus(phase: string): StatusLabelProps['status'] {
  switch (phase) {
    case 'Running':
      return 'success';
    case 'Provisioning':
    case 'Pending':
      return 'warning';
    case 'Failed':
      return 'error';
    case 'Stopped':
      return '';
    default:
      return '';
  }
}

export default function VMListPage() {
  const navigate = useNavigate();
  const [vms, error] = VMClass.useList();

  return (
    <SectionBox
      title="Virtual Machines"
      headerProps={{
        actions: [
          <button
            key="create"
            onClick={() => navigate('/vm/create')}
            style={{
              padding: '8px 16px',
              borderRadius: '4px',
              border: 'none',
              background: '#1976d2',
              color: '#fff',
              cursor: 'pointer',
              fontSize: '14px',
            }}
          >
            + Create VM
          </button>,
        ],
      }}
    >
      {error ? (
        <div>Error loading VMs: {error.message}</div>
      ) : (
        <SimpleTable
          columns={[
            {
              label: 'Name',
              getter: (vm: any) => vm.metadata.name,
            },
            {
              label: 'Status',
              getter: (vm: any) => (
                <StatusLabel status={phaseToStatus(vm.status?.phase || '')}>
                  {vm.status?.phase || 'Unknown'}
                </StatusLabel>
              ),
            },
            {
              label: 'Cores',
              getter: (vm: any) => vm.spec?.cores || '-',
            },
            {
              label: 'Memory',
              getter: (vm: any) => vm.spec?.memory || '-',
            },
            {
              label: 'Image',
              getter: (vm: any) => vm.spec?.disk?.image || '-',
            },
            {
              label: 'GPU',
              getter: (vm: any) => vm.spec?.gpu?.count || 0,
            },
            {
              label: 'Internal IP',
              getter: (vm: any) => vm.status?.internalIP || '-',
            },
            {
              label: 'Age',
              getter: (vm: any) => {
                if (!vm.metadata.creationTimestamp) return '-';
                const created = new Date(vm.metadata.creationTimestamp);
                const now = new Date();
                const diffMs = now.getTime() - created.getTime();
                const diffMins = Math.floor(diffMs / 60000);
                if (diffMins < 60) return `${diffMins}m`;
                const diffHours = Math.floor(diffMins / 60);
                if (diffHours < 24) return `${diffHours}h`;
                return `${Math.floor(diffHours / 24)}d`;
              },
            },
          ]}
          data={vms || []}
          rowsPerPage={[25, 50, 100]}
          onRowClick={(vm: any) => navigate(`/vm/${vm.metadata.name}`)}
        />
      )}
    </SectionBox>
  );
}
