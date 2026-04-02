import { useNavigate } from 'react-router-dom';
import {
  SectionBox,
  SimpleTable,
  StatusLabel,
} from '@kinvolk/headlamp-plugin/lib/components/common';
import { KCClass } from './index';

export default function KCListPage() {
  const navigate = useNavigate();
  const [clusters, error] = KCClass.useList();

  return (
    <SectionBox title="Kubernetes Clusters">
      {error ? (
        <div>Error loading clusters: {error.message}</div>
      ) : (
        <SimpleTable
          columns={[
            {
              label: 'Name',
              getter: (kc: any) => kc.metadata.name,
            },
            {
              label: 'Status',
              getter: (kc: any) => {
                const available = kc.status?.conditions?.find(
                  (c: any) => c.type === 'Available'
                );
                return (
                  <StatusLabel status={available?.status === 'True' ? 'success' : 'warning'}>
                    {available?.status === 'True' ? 'Available' : 'Pending'}
                  </StatusLabel>
                );
              },
            },
            {
              label: 'Version',
              getter: (kc: any) => kc.spec?.version || '-',
            },
            {
              label: 'Nodes',
              getter: (kc: any) => kc.spec?.nodeCount || '-',
            },
          ]}
          data={clusters || []}
          rowsPerPage={[25, 50]}
          onRowClick={(kc: any) => navigate(`/kc/${kc.metadata.name}`)}
        />
      )}
    </SectionBox>
  );
}
