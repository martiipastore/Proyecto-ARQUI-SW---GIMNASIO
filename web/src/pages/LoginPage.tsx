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
  const [messageTone, setMessageTone] = useState<'info' | 'error' | 'success'>('info');
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    clearNotice();
    setMessage('');
    setMessageTone('info');
    setBusy(true);
    try {
      const user = await login(email.trim(), password);
      setPassword('');
      navigate(homeFor(user.role));
    } catch (error) {
      setPassword('');
      setMessageTone('error');
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
      setMessageTone('error');
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
      setMessageTone('success');
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'No se pudo solicitar el enlace.');
      setMessageTone('error');
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="auth-page">
      <section className="auth-story" aria-labelledby="auth-story-title">
        <h1 id="auth-story-title">Volvé<br />al <em>ritmo.</em></h1>
        <p className="auth-story-copy">Un único ingreso para alumnos, profesores, nutricionistas y administración. Tu cuenta te lleva automáticamente al espacio correspondiente.</p>
        <div className="auth-route" aria-hidden="true">
          <span><b>01</b> Ingresá</span>
          <i />
          <span><b>02</b> Reservá</span>
          <i />
          <span><b>03</b> Entrená</span>
        </div>
      </section>

      <section className="auth-panel" aria-labelledby="login-title">
        <header className="auth-panel-header">
          <span className="auth-index">PORTAL ÚNICO</span>
          <h2 id="login-title">Ingresar</h2>
          <p>Usá el correo con el que activaste tu cuenta.</p>
        </header>

        {(notice || message) && (
          <div className="auth-message" data-tone={notice ? 'success' : messageTone} role="status" aria-live="polite">
            <span aria-hidden="true">{notice || messageTone === 'success' ? '✓' : messageTone === 'error' ? '!' : 'i'}</span>
            <p>{notice || message}</p>
          </div>
        )}

        <form className="auth-form" onSubmit={submit}>
          <label className="auth-field">
            <span>Correo electrónico</span>
            <input type="email" autoComplete="username" required maxLength={254} placeholder="nombre@correo.com" value={email} onChange={(e) => setEmail(e.target.value)} />
          </label>
          <label className="auth-field">
            <span>Contraseña</span>
            <input type="password" autoComplete="current-password" required maxLength={72} placeholder="Tu contraseña" value={password} onChange={(e) => setPassword(e.target.value)} />
          </label>
          <button className="button button-primary auth-submit" type="submit" disabled={busy}>
            {busy ? 'Ingresando…' : 'Ingresar'} <span aria-hidden="true">→</span>
          </button>
          <button className="auth-resend" type="button" disabled={busy} onClick={resend}>Reenviar enlace de activación</button>
        </form>

        <p className="auth-security"><span aria-hidden="true">◆</span> Acceso seguro para alumnos y equipo Metal Fitness.</p>
      </section>
    </main>
  );
}
