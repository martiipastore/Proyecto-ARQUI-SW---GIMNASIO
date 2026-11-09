import { FormEvent, useState } from 'react';
import { ApiError, request } from '../api/client';
import { navigate } from '../router';

// Activación de un solo uso (auth.v1, ADR-017): elegir la primera contraseña o,
// si la cuenta ya tenía una, solo confirmar la vinculación.
export function ActivationPage({ token }: { token: string }) {
  const [existing, setExisting] = useState(false);
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [message, setMessage] = useState('');
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);

  if (!token) {
    return (
      <main className="page narrow">
        <h1>Activá tu cuenta</h1>
        <p>El enlace no es válido. Abrí el enlace completo que recibiste por correo o pedí uno nuevo desde el ingreso.</p>
        <button type="button" onClick={() => navigate('/login')}>Ir al ingreso</button>
      </main>
    );
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!existing && password !== confirmPassword) {
      setMessage('Las contraseñas no coinciden.');
      return;
    }
    setBusy(true);
    try {
      await request('/api/v1/auth/activate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token, password: existing ? '' : password }),
      });
      setDone(true);
      setMessage('Cuenta confirmada. Ya podés ingresar.');
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        setMessage('El enlace ya se usó o venció. Pedí uno nuevo desde el ingreso.');
      } else {
        setMessage(error instanceof Error ? error.message : 'No se pudo activar la cuenta.');
      }
    } finally {
      setPassword('');
      setConfirmPassword('');
      setBusy(false);
    }
  }

  return (
    <main className="page narrow">
      <h1>Activá tu cuenta</h1>
      {message && <p role="status" aria-live="polite">{message}</p>}
      {done ? (
        <button type="button" onClick={() => navigate('/login')}>Ir al ingreso</button>
      ) : (
        <form className="club-card" onSubmit={submit}>
          <label className="club-checkbox">
            <input type="checkbox" checked={existing} onChange={(e) => setExisting(e.target.checked)} />
            Ya tengo contraseña; solo quiero confirmar la vinculación.
          </label>
          {!existing && (
            <>
              <label>
                Elegí tu contraseña (12 a 72 caracteres)
                <input type="password" autoComplete="new-password" required minLength={12} maxLength={72} value={password} onChange={(e) => setPassword(e.target.value)} />
              </label>
              <label>
                Repetí la contraseña
                <input type="password" autoComplete="new-password" required minLength={12} maxLength={72} value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} />
              </label>
            </>
          )}
          <button type="submit" disabled={busy}>{busy ? 'Procesando…' : 'Confirmar cuenta'}</button>
        </form>
      )}
    </main>
  );
}
