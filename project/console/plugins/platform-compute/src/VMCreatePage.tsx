import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { SectionBox } from '@kinvolk/headlamp-plugin/lib/components/common';
import { K8s } from '@kinvolk/headlamp-plugin/lib';
import { VMClass } from './index';

const IMAGES = ['ubuntu-22.04', 'ubuntu-24.04', 'debian-12', 'flatcar'];

export default function VMCreatePage() {
  const navigate = useNavigate();
  const [name, setName] = useState('');
  const [cores, setCores] = useState(2);
  const [memory, setMemory] = useState('4Gi');
  const [diskSize, setDiskSize] = useState('50Gi');
  const [image, setImage] = useState('ubuntu-24.04');
  const [gpuCount, setGpuCount] = useState(0);
  const [sshKey, setSshKey] = useState('');
  const [error, setError] = useState('');
  const [creating, setCreating] = useState(false);

  async function handleCreate() {
    if (!name) {
      setError('Name is required');
      return;
    }

    setCreating(true);
    setError('');

    const resource: any = {
      apiVersion: 'compute.cloud.platform/v1alpha1',
      kind: 'VirtualMachine',
      metadata: { name },
      spec: {
        cores,
        memory,
        disk: { size: diskSize, image },
      },
    };

    if (gpuCount > 0) {
      resource.spec.gpu = { count: gpuCount };
    }
    if (sshKey) {
      resource.spec.ssh = { publicKey: sshKey };
    }

    try {
      await K8s.apply(resource);
      navigate('/vm');
    } catch (err: any) {
      setError(err.message || 'Failed to create VM');
      setCreating(false);
    }
  }

  const fieldStyle = {
    width: '100%',
    padding: '8px 12px',
    border: '1px solid #ccc',
    borderRadius: '4px',
    fontSize: '14px',
    boxSizing: 'border-box' as const,
  };

  const labelStyle = {
    display: 'block',
    marginBottom: '4px',
    fontWeight: 600,
    fontSize: '13px',
  };

  const rowStyle = { marginBottom: '16px' };

  return (
    <SectionBox title="Create Virtual Machine">
      <div style={{ maxWidth: '600px' }}>
        <div style={rowStyle}>
          <label style={labelStyle}>Name</label>
          <input
            style={fieldStyle}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="my-vm"
          />
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          <div style={rowStyle}>
            <label style={labelStyle}>CPU Cores</label>
            <input
              type="number"
              style={fieldStyle}
              value={cores}
              min={1}
              max={64}
              onChange={(e) => setCores(parseInt(e.target.value) || 1)}
            />
          </div>

          <div style={rowStyle}>
            <label style={labelStyle}>Memory</label>
            <input
              style={fieldStyle}
              value={memory}
              onChange={(e) => setMemory(e.target.value)}
              placeholder="4Gi"
            />
          </div>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          <div style={rowStyle}>
            <label style={labelStyle}>Disk Size</label>
            <input
              style={fieldStyle}
              value={diskSize}
              onChange={(e) => setDiskSize(e.target.value)}
              placeholder="50Gi"
            />
          </div>

          <div style={rowStyle}>
            <label style={labelStyle}>OS Image</label>
            <select style={fieldStyle} value={image} onChange={(e) => setImage(e.target.value)}>
              {IMAGES.map((img) => (
                <option key={img} value={img}>
                  {img}
                </option>
              ))}
            </select>
          </div>
        </div>

        <div style={rowStyle}>
          <label style={labelStyle}>GPU Count (0 = none)</label>
          <input
            type="number"
            style={fieldStyle}
            value={gpuCount}
            min={0}
            max={8}
            onChange={(e) => setGpuCount(parseInt(e.target.value) || 0)}
          />
        </div>

        <div style={rowStyle}>
          <label style={labelStyle}>SSH Public Key (optional)</label>
          <textarea
            style={{ ...fieldStyle, minHeight: '80px', fontFamily: 'monospace' }}
            value={sshKey}
            onChange={(e) => setSshKey(e.target.value)}
            placeholder="ssh-ed25519 AAAA..."
          />
        </div>

        {error && (
          <div style={{ color: '#d32f2f', marginBottom: '16px', fontSize: '14px' }}>{error}</div>
        )}

        <div style={{ display: 'flex', gap: '12px' }}>
          <button
            onClick={handleCreate}
            disabled={creating}
            style={{
              padding: '10px 24px',
              borderRadius: '4px',
              border: 'none',
              background: creating ? '#999' : '#1976d2',
              color: '#fff',
              cursor: creating ? 'default' : 'pointer',
              fontSize: '14px',
              fontWeight: 600,
            }}
          >
            {creating ? 'Creating...' : 'Create'}
          </button>
          <button
            onClick={() => navigate('/vm')}
            style={{
              padding: '10px 24px',
              borderRadius: '4px',
              border: '1px solid #ccc',
              background: 'transparent',
              cursor: 'pointer',
              fontSize: '14px',
            }}
          >
            Cancel
          </button>
        </div>
      </div>
    </SectionBox>
  );
}
