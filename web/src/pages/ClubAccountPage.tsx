import { FormEvent, useEffect, useState } from 'react';
import { ApiError, request } from '../api/client';

function activationToken(): string {
  const token = new URLSearchParams(window.location.hash.slice(1)).get('activate') ?? '';
  if (token) window.history.replaceState(null, '', window.location.pathname + window.location.search);
  return token;
}
const initialActivationToken = activationToken();

interface MembershipSummary {
  activa: boolean;
  membresia_vigente: { tipo: { nombre: string }; fecha_vencimiento: string } | null;
  total: number;
  items: { id: string; tipo: { nombre: string; clases: number | null }; fecha_inicio: string; fecha_vencimiento: string; condicion: string }[];
}

export function ClubAccountPage() {
  const [activation, setActivation] = useState(initialActivationToken);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [existing, setExisting] = useState(false);
  const [token, setToken] = useState('');
  const [expiresAt, setExpiresAt] = useState(0);
  const [name, setName] = useState('');
  const [balance, setBalance] = useState<number | null>(null);
  const [memberships, setMemberships] = useState<MembershipSummary | null>(null);
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);

  function clearAccess() {
    setToken(''); setExpiresAt(0); setBalance(null); setMemberships(null); setName('');
  }

  useEffect(() => {
    if (!token || !expiresAt) return;
    const timer = window.setTimeout(() => {
      setToken(''); setExpiresAt(0); setBalance(null); setMemberships(null); setName('');
      setMessage('Tu acceso venció. Ingresá nuevamente.');
    }, Math.max(0, expiresAt - Date.now()));
    return () => window.clearTimeout(timer);
  }, [token, expiresAt]);

  async function refresh(accessToken: string) {
    const headers = { Authorization: `Bearer ${accessToken}` };
    const [user, account, membershipPage] = await Promise.all([
      request<{ name: string }>('/api/v1/users/me', { headers }),
      request<{ balance: number }>('/api/v1/benefits/me', { headers }),
      request<MembershipSummary>('/api/v1/memberships/me', { headers }),
    ]);
    setName(user.data.name);
    setBalance(account.data.balance);
    setMemberships(membershipPage.data);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setMessage('');
    if (activation && !existing && password !== confirmPassword) {
      setMessage('Las contraseñas no coinciden.');
      return;
    }
    setBusy(true);
    try {
      if (activation) {
        await request('/api/v1/auth/activate', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ token: activation, password: existing ? '' : password }),
        });
        setActivation(''); setPassword(''); setConfirmPassword('');
        setMessage('Cuenta confirmada. Ya podés ingresar. Los beneficios pendientes se procesan automáticamente.');
      } else {
        const session = await request<{ access_token: string; expires_in: number }>('/api/v1/auth/login', {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email, password }),
        });
        setToken(session.data.access_token); setPassword('');
        setExpiresAt(Date.now() + session.data.expires_in * 1000);
        await refresh(session.data.access_token);
      }
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) clearAccess();
      setMessage(error instanceof Error ? error.message : 'No se pudo completar la operación.');
    } finally { setBusy(false); }
  }

  return <main className="page club-account">
    <h1>{activation ? 'Activá tu cuenta' : 'Club de Beneficios'}</h1>
    <p className="muted">Tus puntos permanecen en tu cuenta, tengas o no una membresía activa.</p>
    {message && <p role="status" aria-live="polite">{message}</p>}
    {token ? <section className="club-card">
      <h2>{name ? `Hola, ${name}` : 'Tu cuenta'}</h2>
      <p>Puntos disponibles: <strong>{balance ?? 'Consultando…'}</strong></p>
      {memberships && <section aria-label="Membresías">
        <h3>Tu membresía</h3>
        <p>{memberships.activa && memberships.membresia_vigente
          ? `${memberships.membresia_vigente.tipo.nombre} vigente hasta el ${memberships.membresia_vigente.fecha_vencimiento}`
          : 'Sin membresía vigente. Tus puntos se conservan.'}</p>
        {memberships.items.length > 0 && <>
          <h3>Historial de membresías</h3>
          <ul>{memberships.items.map(m => <li key={m.id}>
            {m.tipo.nombre}: {m.fecha_inicio} al {m.fecha_vencimiento} — {m.condicion.toLocaleLowerCase('es-AR')}
            {m.tipo.clases !== null && ` · Cupo contratado: ${m.tipo.clases} clases`}
          </li>)}</ul>
          {memberships.total > memberships.items.length && <p>Se muestran las últimas {memberships.items.length} de {memberships.total} membresías.</p>}
        </>}
      </section>}
      <button disabled={busy} onClick={async () => {
        setBusy(true); try { await refresh(token); setMessage('Saldo actualizado.'); }
        catch (error) { if (error instanceof ApiError && error.status === 401) clearAccess(); setMessage(error instanceof Error ? error.message : 'No se pudo consultar el saldo.'); }
        finally { setBusy(false); }
      }}>Actualizar saldo</button>
      <button disabled={busy} onClick={async () => {
        setBusy(true);
        try { await request('/api/v1/auth/logout', { method: 'POST', headers: { Authorization: `Bearer ${token}` } }); setMessage('Sesión cerrada.'); }
        catch { setMessage('Acceso cerrado en este dispositivo.'); }
        finally { clearAccess(); setBusy(false); }
      }}>Cerrar sesión</button>
    </section> : <form className="club-card" onSubmit={submit}>
      {activation ? <label className="club-checkbox"><input type="checkbox" checked={existing} onChange={e => setExisting(e.target.checked)} />Ya tengo contraseña; solo quiero confirmar la vinculación.</label>
        : <label>Correo electrónico<input type="email" autoComplete="username" required maxLength={254} value={email} onChange={e => setEmail(e.target.value)} /></label>}
      {(!activation || !existing) && <label>{activation ? 'Establecé tu contraseña' : 'Contraseña'}<input type="password" autoComplete={activation ? 'new-password' : 'current-password'} required minLength={activation ? 12 : undefined} maxLength={72} value={password} onChange={e => setPassword(e.target.value)} /></label>}
      {activation && !existing && <label>Repetí la contraseña<input type="password" autoComplete="new-password" required minLength={12} maxLength={72} value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} /></label>}
      <button type="submit" disabled={busy}>{busy ? 'Procesando…' : activation ? 'Confirmar cuenta' : 'Ingresar'}</button>
      {!activation && <button type="button" disabled={busy} onClick={async () => {
        if (!email) { setMessage('Ingresá tu correo para reenviar el enlace.'); return; }
        setBusy(true);
        try { await request('/api/v1/auth/resend-activation', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email }) }); setMessage('Si tenés una activación pendiente, recibirás un nuevo enlace por correo.'); }
        catch (error) { setMessage(error instanceof Error ? error.message : 'No se pudo solicitar el enlace.'); }
        finally { setBusy(false); }
      }}>Reenviar enlace de activación</button>}
    </form>}
  </main>;
}
