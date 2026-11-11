import { FormEvent, ReactNode, useState } from 'react';
import { ApiError, request } from '../api/client';
import { navigate } from '../router';

// Activación de un solo uso (auth.v1, ADR-017): elegir la primera contraseña o,
// si la cuenta ya tenía una, solo confirmar la vinculación.
function ActivationLayout({ children }: { children: ReactNode }) {
  return (
    <main className="auth-page">
      <section className="auth-story" aria-labelledby="activation-story-title">
        <h1 id="activation-story-title">Tu primer<br /><em>paso.</em></h1>
        <p className="auth-story-copy">Elegí tu contraseña una sola vez. Después ingresás con tu correo y el sistema te lleva a tu espacio.</p>
      </section>
      <section className="auth-panel" aria-labelledby="activation-title">
        <header className="auth-panel-header">
          <span className="auth-index">ACTIVACIÓN</span>
          <h2 id="activation-title">Activá tu cuenta</h2>
        </header>
        {children}
      </section>
    </main>
  );
}

export function ActivationPage({ token }: { token: string }) {
  const [existing, setExisting] = useState(false);
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [message, setMessage] = useState('');
  const [tone, setTone] = useState<'error' | 'success'>('error');
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);

  if (!token) {
    return (
      <ActivationLayout>
        <div className="auth-message" data-tone="error" role="alert"><span aria-hidden="true">!</span><p>El enlace no es válido. Abrí el enlace completo que recibiste por correo o pedí uno nuevo desde el ingreso.</p></div>
        <button type="button" className="button button-primary auth-submit" onClick={() => navigate('/login')}>Ir al ingreso <span aria-hidden="true">→</span></button>
      </ActivationLayout>
    );
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!existing && password !== confirmPassword) {
      setTone('error');
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
      setTone('success');
      setMessage('Cuenta confirmada. Ya podés ingresar.');
    } catch (error) {
      setTone('error');
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
    <ActivationLayout>
      {message && (
        <div className="auth-message" data-tone={tone} role="status" aria-live="polite">
          <span aria-hidden="true">{tone === 'success' ? '✓' : '!'}</span><p>{message}</p>
        </div>
      )}
      {done ? (
        <button type="button" className="button button-primary auth-submit" onClick={() => navigate('/login')}>Ir al ingreso <span aria-hidden="true">→</span></button>
      ) : (
        <form className="auth-form" onSubmit={submit}>
          <label className="auth-check">
            <input type="checkbox" checked={existing} onChange={(e) => setExisting(e.target.checked)} />
            Ya tengo contraseña; solo quiero confirmar la vinculación.
          </label>
          {!existing && (
            <>
              <label className="auth-field">
                <span>Elegí tu contraseña (12 a 72 caracteres)</span>
                <input type="password" autoComplete="new-password" required minLength={12} maxLength={72} value={password} onChange={(e) => setPassword(e.target.value)} />
              </label>
              <label className="auth-field">
                <span>Repetí la contraseña</span>
                <input type="password" autoComplete="new-password" required minLength={12} maxLength={72} value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} />
              </label>
            </>
          )}
          <button type="submit" className="button button-primary auth-submit" disabled={busy}>{busy ? 'Procesando…' : 'Confirmar cuenta'} <span aria-hidden="true">→</span></button>
        </form>
      )}
    </ActivationLayout>
  );
}
