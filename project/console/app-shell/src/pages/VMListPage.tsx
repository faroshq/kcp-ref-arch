import * as React from 'react';
import {
  Box, Typography, Button, Paper, Table, TableBody, TableCell,
  TableContainer, TableHead, TableRow, Chip, CircularProgress, IconButton,
  Dialog, DialogTitle, DialogContent, DialogContentText, DialogActions,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { useNavigate } from 'react-router-dom';
import { vmApi, type K8sResource } from './api';

const statusColor: Record<string, 'success' | 'warning' | 'error' | 'default'> = {
  Running: 'success',
  Provisioning: 'warning',
  Pending: 'warning',
  Failed: 'error',
  Stopped: 'default',
};

export const VMListPage: React.FC = () => {
  const navigate = useNavigate();
  const [vms, setVms] = React.useState<K8sResource[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState('');
  const [deleteTarget, setDeleteTarget] = React.useState<string | null>(null);
  const [deleting, setDeleting] = React.useState(false);

  React.useEffect(() => {
    vmApi.list()
      .then(setVms)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await vmApi.delete(deleteTarget);
      setVms((prev) => prev.filter((v) => v.metadata.name !== deleteTarget));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to delete VM');
    } finally {
      setDeleting(false);
      setDeleteTarget(null);
    }
  };

  if (loading) return <Box sx={{ display: 'flex', justifyContent: 'center', mt: 8 }}><CircularProgress /></Box>;
  if (error) return <Typography color="error">{error}</Typography>;

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h5">Virtual Machines</Typography>
        <Button variant="contained" onClick={() => navigate('/vm/create')}>
          Create VM
        </Button>
      </Box>
      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Status</TableCell>
              <TableCell>Cores</TableCell>
              <TableCell>Memory</TableCell>
              <TableCell>Image</TableCell>
              <TableCell>GPU</TableCell>
              <TableCell>IP</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {vms.length === 0 ? (
              <TableRow>
                <TableCell colSpan={8} align="center">
                  <Typography color="text.secondary" sx={{ py: 4 }}>
                    No virtual machines found. Create one to get started.
                  </Typography>
                </TableCell>
              </TableRow>
            ) : (
              vms.map((vm) => {
                const spec = (vm.spec || {}) as Record<string, unknown>;
                const disk = (spec.disk || {}) as Record<string, unknown>;
                const gpu = (spec.gpu || {}) as Record<string, unknown>;
                const status = (vm.status || {}) as Record<string, unknown>;
                const phase = (status.phase as string) || 'Unknown';
                const internalIP = (status.internalIP as string) || '';
                return (
                  <TableRow
                    key={vm.metadata.name}
                    hover
                    sx={{ cursor: 'pointer' }}
                    onClick={() => navigate(`/vm/${vm.metadata.name}`)}
                  >
                    <TableCell>{vm.metadata.name}</TableCell>
                    <TableCell>
                      <Chip label={phase} size="small" color={statusColor[phase] || 'default'} />
                    </TableCell>
                    <TableCell>{spec.cores as number || '-'}</TableCell>
                    <TableCell>{spec.memory as string || '-'}</TableCell>
                    <TableCell>{disk.image as string || '-'}</TableCell>
                    <TableCell>{gpu.count as number || 0}</TableCell>
                    <TableCell>{internalIP || '-'}</TableCell>
                    <TableCell align="right" onClick={(e) => e.stopPropagation()}>
                      <IconButton size="small" onClick={() => navigate(`/vm/${vm.metadata.name}/edit`)}>
                        <EditIcon fontSize="small" />
                      </IconButton>
                      <IconButton size="small" color="error" onClick={() => setDeleteTarget(vm.metadata.name)}>
                        <DeleteIcon fontSize="small" />
                      </IconButton>
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
      </TableContainer>

      <Dialog open={!!deleteTarget} onClose={() => setDeleteTarget(null)}>
        <DialogTitle>Delete Virtual Machine</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to delete <strong>{deleteTarget}</strong>? This action cannot be undone.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteTarget(null)} disabled={deleting}>Cancel</Button>
          <Button onClick={handleDelete} color="error" variant="contained" disabled={deleting}>
            {deleting ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
