import { useParams } from 'react-router-dom';
import {
  MainInfoSection,
  SectionBox,
  NameValueTable,
} from '@kinvolk/headlamp-plugin/lib/components/common';
import { KCClass } from './index';

export default function KCDetailPage() {
  const { name } = useParams<{ name: string }>();
  const [kc, error] = KCClass.useGet(name);

  if (error) {
    return <div>Error: {error.message}</div>;
  }
  if (!kc) {
    return <div>Loading...</div>;
  }

  return (
    <>
      <MainInfoSection resource={kc} title="Kubernetes Cluster" />

      <SectionBox title="Specification">
        <NameValueTable
          rows={[
            { name: 'Version', value: kc.spec?.version || '-' },
            { name: 'Node Count', value: kc.spec?.nodeCount || '-' },
          ]}
        />
      </SectionBox>

      {kc.status?.conditions && kc.status.conditions.length > 0 && (
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
              {kc.status.conditions.map((c: any, i: number) => (
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
    </>
  );
}
