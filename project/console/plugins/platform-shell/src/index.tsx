import {
  registerAppLogo,
  registerClusterChooser,
  registerSidebarEntryFilter,
  registerRouteFilter,
} from '@kinvolk/headlamp-plugin/lib';
import { useEffect, useState } from 'react';

// ============================================================================
// Platform Shell Plugin
//
// Reshapes the base UI into the NeoCloud console:
// - Replaces the logo with NeoCloud branding
// - Replaces the cluster chooser with a workspace picker
// - Hides Kubernetes-native navigation (Pods, Deployments, Nodes, etc.)
// - Only shows platform resource navigation (added by other plugins)
// - Adds OIDC authentication flow via the platform proxy
// ============================================================================

// --- Auth helpers ---

const TOKEN_KEY = 'platform-oidc-token';
const AUTH_RESPONSE_KEY = 'platform-auth-response';

interface AuthResponse {
  idToken: string;
  refreshToken?: string;
  expiresAt: number;
  email: string;
  clusterName?: string;
}

function getStoredAuth(): AuthResponse | null {
  try {
    const raw = localStorage.getItem(AUTH_RESPONSE_KEY);
    if (!raw) return null;
    const auth: AuthResponse = JSON.parse(raw);
    // Check if token is expired (with 60s buffer).
    if (auth.expiresAt && auth.expiresAt * 1000 < Date.now() - 60000) {
      localStorage.removeItem(AUTH_RESPONSE_KEY);
      localStorage.removeItem(TOKEN_KEY);
      return null;
    }
    return auth;
  } catch {
    return null;
  }
}

function storeAuth(auth: AuthResponse) {
  localStorage.setItem(AUTH_RESPONSE_KEY, JSON.stringify(auth));
  localStorage.setItem(TOKEN_KEY, auth.idToken);
}

function clearAuth() {
  localStorage.removeItem(AUTH_RESPONSE_KEY);
  localStorage.removeItem(TOKEN_KEY);
}

function startOIDCLogin() {
  // Redirect to the platform proxy's /auth/authorize endpoint.
  // The callback will come back to /console/auth/callback.
  const callbackURL = window.location.origin + '/console/auth/callback';
  const authorizeURL = '/auth/authorize?redirect_uri=' + encodeURIComponent(callbackURL);
  window.location.href = authorizeURL;
}

function handleAuthCallback(): AuthResponse | null {
  // Check if this is an auth callback (URL has ?response= parameter).
  const params = new URLSearchParams(window.location.search);
  const responseParam = params.get('response');
  if (!responseParam) return null;

  try {
    const decoded = atob(responseParam.replace(/-/g, '+').replace(/_/g, '/'));
    const auth: AuthResponse = JSON.parse(decoded);
    storeAuth(auth);

    // Clean the URL (remove ?response= query param).
    window.history.replaceState({}, '', window.location.pathname);

    return auth;
  } catch (e) {
    console.error('Failed to parse auth callback response:', e);
    return null;
  }
}

// --- Branding ---

function PlatformLogo() {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
      <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor">
        <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
      </svg>
      <span style={{ fontSize: '16px', fontWeight: 600 }}>NeoCloud</span>
    </div>
  );
}

registerAppLogo(PlatformLogo);

// --- OIDC Login Gate ---
// On plugin load, check for auth callback or existing token.
// If neither, redirect to OIDC login.

(function initAuth() {
  // 1. Check if this is a callback from the OIDC provider.
  if (window.location.pathname === '/console/auth/callback' ||
      window.location.search.includes('response=')) {
    const auth = handleAuthCallback();
    if (auth) {
      // Redirect to console root after successful login.
      window.location.href = '/console';
      return;
    }
  }

  // 2. Check for existing valid token.
  const existing = getStoredAuth();
  if (existing) {
    // Token exists and is valid — configure Headlamp to use it.
    // Headlamp reads the token from the cluster config; we inject it
    // via the X-Authorization header approach or by setting the token
    // in Headlamp's cluster configuration.
    return;
  }

  // 3. No token and not a callback — start OIDC login.
  // Only redirect if we're on a /console path (avoid infinite loops).
  if (window.location.pathname.startsWith('/console') &&
      !window.location.pathname.includes('/auth/')) {
    startOIDCLogin();
  }
})();

// --- Workspace Chooser ---
// Replaces the default "cluster" chooser with a workspace-oriented picker.

registerClusterChooser(({ clusters, cluster, onSelect }) => {
  const auth = getStoredAuth();
  const [userEmail, setUserEmail] = useState(auth?.email || '');

  useEffect(() => {
    const stored = getStoredAuth();
    if (stored?.email) setUserEmail(stored.email);
  }, []);

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '12px', padding: '4px 12px' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
        <span style={{ fontSize: '13px', opacity: 0.7 }}>Workspace:</span>
        <select
          value={cluster || ''}
          onChange={(e) => onSelect(e.target.value)}
          style={{
            background: 'transparent',
            border: '1px solid rgba(255,255,255,0.3)',
            borderRadius: '4px',
            color: 'inherit',
            padding: '4px 8px',
            fontSize: '13px',
          }}
        >
          {clusters.map((c: string) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>
      </div>
      {userEmail && (
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <span style={{ fontSize: '12px', opacity: 0.6 }}>{userEmail}</span>
          <button
            onClick={() => {
              clearAuth();
              startOIDCLogin();
            }}
            style={{
              background: 'transparent',
              border: '1px solid rgba(255,255,255,0.2)',
              borderRadius: '4px',
              color: 'inherit',
              padding: '2px 8px',
              fontSize: '11px',
              cursor: 'pointer',
              opacity: 0.7,
            }}
          >
            Sign out
          </button>
        </div>
      )}
    </div>
  );
});

// --- Hide default Kubernetes navigation ---
// Only keep sidebar entries added by platform plugins (prefixed with "platform-").

const PLATFORM_PREFIXES = ['platform-'];

registerSidebarEntryFilter((entry) => {
  // Keep entries from platform plugins
  if (PLATFORM_PREFIXES.some((prefix) => entry.name.startsWith(prefix))) {
    return entry;
  }
  // Keep the cluster (workspace) overview
  if (entry.name === 'cluster') {
    return entry;
  }
  // Hide everything else (Workloads, Network, Storage, etc.)
  return null;
});

// --- Hide default Kubernetes routes ---
// Prevent navigation to built-in K8s resource pages.

const ALLOWED_ROUTE_PREFIXES = ['/vm', '/kc', '/c/'];

registerRouteFilter((route) => {
  const path = route.path || '';

  // Allow platform-specific routes
  if (ALLOWED_ROUTE_PREFIXES.some((prefix) => path.startsWith(prefix))) {
    return route;
  }
  // Allow root and cluster selection
  if (path === '/' || path === '/clusters') {
    return route;
  }
  // Allow settings and auth callback
  if (path.startsWith('/settings') || path.startsWith('/auth')) {
    return route;
  }
  // Hide everything else
  return null;
});
