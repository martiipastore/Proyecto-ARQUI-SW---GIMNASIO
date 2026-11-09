import { FormEvent, useState } from 'react';
import { ApiError, request } from '../api/client';
import { homeFor, useSession } from '../auth/session';
import { navigate } from '../router';

// Login (auth.v1). El mismo mensaje para correo inexistente y contraseña
// incorrecta: el backend tampoco distingue los casos.
export function LoginPage() {
  const { login, notice, clearNotice } = useSession();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    clearNotice();
    setMessage('');
    setBusy(true);
    try {
      const user = await login(email.trim(), password);
      setPassword('');
      navigate(homeFor(user.role));
    } catch (error) {
      setPassword('');
      if (error instanceof ApiError && error.status === 401) {
        setMessage('Correo o contraseña incorrectos.');
      } else if (error instanceof ApiError && error.status === 429) {
        setMessage('Demasiados intentos. Esperá un minuto y volvé a probar.');
      } else {
        setMessage(error instanceof Error ? error.message : 'No se pudo ingresar.');
      }
    } finally {
      setBusy(false);
    }
  }

  async function resend() {
    if (!email.trim()) {
      setMessage('Ingresá tu correo para reenviar el enlace.');
      return;
    }
    setBusy(true);
    try {
      await request('/api/v1/auth/resend-activation', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email.trim() }),
      });
      setMessage('Si tenés una activación pendiente, vas a recibir un nuevo enlace por correo.');
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'No se pudo solicitar el enlace.');
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="page narrow">
      <h1>Ingresar</h1>
      {(notice || message) && <p role="status" aria-live="polite">{notice || message}</p>}
      <form className="club-card" onSubmit={submit}>
        <label>
          Correo electrónico
          <input type="email" autoComplete="username" required maxLength={254} value={email} onChange={(e) => setEmail(e.target.value)} />
        </label>
        <label>
          Contraseña
          <input type="password" autoComplete="current-password" required maxLength={72} value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
        <button type="submit" disabled={busy}>{busy ? 'Ingresando…' : 'Ingresar'}</button>
        <button type="button" disabled={busy} onClick={resend}>Reenviar enlace de activación</button>
      </form>
    </main>
  );
}
