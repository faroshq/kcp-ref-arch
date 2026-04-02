import { useParams } from 'react-router-dom';
import {
  MainInfoSection,
  SectionBox,
  NameValueTable,
  StatusLabel,
} from '@kinvolk/headlamp-plugin/lib/components/common';
import { VMClass } from './index';

export default function VMDetailPage() {
  const { name } = useParams<{ name: string }>();
  const [vm, error] = VMClass.useGet(name);

  if (error) {
    return <div>Error loading VM: {error.message}</div>;
  }
  if (!vm) {
    return <div>Loading...</div>;
  }

  const spec = vm.spec || {};
  const status = vm.status || {};

  return (
    <>
      <MainInfoSection
        resource={vm}
        title="Virtual Machine"
        extraInfo={[
          {
            name: 'Phase',
            value: (
              <StatusLabel
                status={
                  status.phase === 'Running'
                    ? 'success'
                    : status.phase === 'Failed'
                    ? 'error'
                    : 'warning'
                }
              >
                {status.phase || 'Unknown'}
              </StatusLabel>
            ),
          },
        ]}
      />

      <SectionBox title="Specification">
        <NameValueTable
          rows={[
            { name: 'CPU Cores', value: spec.cores },
            { name: 'Memory', value: spec.memory },
            { name: 'Disk Size', value: spec.disk?.size || '-' },
            { name: 'Disk Image', value: spec.disk?.image || '-' },
            { name: 'GPU Count', value: spec.gpu?.count ?? 0 },
            { name: 'SSH Public Key', value: spec.ssh?.publicKey ? '(configured)' : 'Not set' },
          ]}
        />
      </SectionBox>

      <SectionBox title="Status">
        <NameValueTable
          rows={[
            { name: 'Phase', value: status.phase || '-' },
            { name: 'Internal IP', value: status.internalIP || '-' },
            { name: 'SSH Endpoint', value: status.sshEndpoint || '-' },
            { name: 'Tunnel Endpoint', value: status.tunnelEndpoint || '-' },
            { name: 'Message', value: status.message || '-' },
          ]}
        />
      </SectionBox>

      {status.conditions && status.conditions.length > 0 && (
        <SectionBox title="Conditions">
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                <th style={{ textAlign: 'left', padding: '8px' }}>Type</th>
                <th style={{ textAlign: 'left', padding: '8px' }}>Status</th>
                <th style={{ textAlign: 'left', padding: '8px' }}>Reason</th>
                <th style={{ textAlign: 'left', padding: '8px' }}>Message</th>
              </tr>
            </thead>
            <tbody>
              {status.conditions.map((c: any, i: number) => (
                <tr key={i}>
                  <td style={{ padding: '8px' }}>{c.type}</td>
                  <td style={{ padding: '8px' }}>{c.status}</td>
                  <td style={{ padding: '8px' }}>{c.reason}</td>
                  <td style={{ padding: '8px' }}>{c.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </SectionBox>
      )}

      {status.relatedResources && Object.keys(status.relatedResources).length > 0 && (
        <SectionBox title="Related Resources">
          <NameValueTable
            rows={Object.entries(status.relatedResources).map(([key, ref]: [string, any]) => ({
              name: key,
              value: `${ref.gvk?.kind || ''} ${ref.namespace ? ref.namespace + '/' : ''}${ref.name}`,
            }))}
          />
        </SectionBox>
      )}
    </>
  );
}
