import * as React from 'react';
import {
  Box, Typography, Paper, TextField, Button, MenuItem, Alert, CircularProgress,
} from '@mui/material';
import { useParams, useNavigate } from 'react-router-dom';
import { vmApi } from './api';

const OS_IMAGES = ['ubuntu-22.04', 'ubuntu-24.04', 'debian-12', 'flatcar'];

export const VMEditPage: React.FC = () => {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);
  const [error, setError] = React.useState('');
  const [original, setOriginal] = React.useState<Record<string, unknown> | null>(null);

  const [cores, setCores] = React.useState(2);
  const [memory, setMemory] = React.useState('4Gi');
  const [diskSize, setDiskSize] = React.useState('50Gi');
  const [diskImage, setDiskImage] = React.useState(OS_IMAGES[0]);
  const [gpuCount, setGpuCount] = React.useState(0);
  const [sshPublicKey, setSshPublicKey] = React.useState('');

  React.useEffect(() => {
    if (!name) return;
    vmApi.get(name)
      .then((vm) => {
        setOriginal(vm as unknown as Record<string, unknown>);
        const spec = (vm.spec || {}) as Record<string, unknown>;
        const disk = (spec.disk || {}) as Record<string, unknown>;
        const gpu = (spec.gpu || {}) as Record<string, unknown>;
        const ssh = (spec.ssh || {}) as Record<string, unknown>;
        setCores((spec.cores as number) || 2);
        setMemory((spec.memory as string) || '4Gi');
        setDiskSize((disk.size as string) || '50Gi');
        setDiskImage((disk.image as string) || OS_IMAGES[0]);
        setGpuCount((gpu.count as number) || 0);
        setSshPublicKey((ssh.publicKey as string) || '');
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [name]);

  const handleSave = async () => {
    if (!name || !original) return;
    setSaving(true);
    setError('');
    try {
      const updated = {
        ...original,
        spec: {
          cores,
          memory,
          disk: { size: diskSize, image: diskImage },
          ...(gpuCount > 0 && { gpu: { count: gpuCount } }),
          ...(sshPublicKey && { ssh: { publicKey: sshPublicKey } }),
        },
      };
      await vmApi.update(name, updated);
      navigate(`/vm/${name}`);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to update VM');
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <Box sx={{ display: 'flex', justifyContent: 'center', mt: 8 }}><CircularProgress /></Box>;
  if (error && !original) return <Typography color="error">{error}</Typography>;

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 3 }}>
        <Button variant="text" onClick={() => navigate(`/vm/${name}`)}>&larr; Back</Button>
        <Typography variant="h5">Edit {name}</Typography>
      </Box>
      <Paper sx={{ p: 3, maxWidth: 600 }}>
        {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}
        <TextField
          label="Name" fullWidth sx={{ mb: 2 }}
          value={name} disabled
        />
        <TextField
          label="CPU Cores" type="number" fullWidth sx={{ mb: 2 }}
          value={cores} onChange={(e) => setCores(Number(e.target.value))}
          slotProps={{ htmlInput: { min: 1, max: 64 } }}
        />
        <TextField
          label="Memory" fullWidth sx={{ mb: 2 }}
          value={memory} onChange={(e) => setMemory(e.target.value)}
          helperText="e.g. 4Gi, 8Gi, 16Gi"
        />
        <TextField
          label="Disk Size" fullWidth sx={{ mb: 2 }}
          value={diskSize} onChange={(e) => setDiskSize(e.target.value)}
          helperText="e.g. 50Gi, 100Gi"
        />
        <TextField
          label="OS Image" select fullWidth sx={{ mb: 2 }}
          value={diskImage} onChange={(e) => setDiskImage(e.target.value)}
        >
          {OS_IMAGES.map((img) => (
            <MenuItem key={img} value={img}>{img}</MenuItem>
          ))}
        </TextField>
        <TextField
          label="GPU Count" type="number" fullWidth sx={{ mb: 2 }}
          value={gpuCount} onChange={(e) => setGpuCount(Number(e.target.value))}
          slotProps={{ htmlInput: { min: 0, max: 8 } }}
        />
        <TextField
          label="SSH Public Key" fullWidth multiline rows={3} sx={{ mb: 3 }}
          value={sshPublicKey} onChange={(e) => setSshPublicKey(e.target.value)}
          placeholder="ssh-ed25519 AAAA..."
        />
        <Button
          variant="contained" fullWidth size="large"
          onClick={handleSave} disabled={saving}
        >
          {saving ? 'Saving...' : 'Save Changes'}
        </Button>
      </Paper>
    </Box>
  );
};
