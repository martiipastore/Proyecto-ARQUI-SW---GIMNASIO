import { FormEvent, useState } from 'react';
import { ApiError, Problem } from '../../api/client';
import { useSession } from '../../auth/session';
import { day, newIdempotencyKey, range } from './types';

// Pantalla de recepción: ingreso por DNI (booking.v1, HU-22, D-36 a D-39). La
// usa el administrador (D-82). Verde: pasa; rojo: no pasa, con el motivo.
interface CheckIn {
  resultado: 'VERDE';
  mensaje: string;
  nombre: string;
  membresia: { estado: string; vence: string | null };
  clase: { id: string; actividad: string; inicio: string; fin: string };
  puntos: number;
}

interface Outcome {
  ok: boolean;
  title: string;
  detail: string;
}

export function ReceptionPage() {
  const { authRequest } = useSession();
  const [dni, setDni] = useState('');
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setOutcome(null);
    try {
      const res = await authRequest<CheckIn>('/api/v1/check-ins', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': newIdempotencyKey() },
        body: JSON.stringify({ dni }),
      });
      const c = res.data.clase;
      const vence = res.data.membresia.vence ? ` · Membresía vigente hasta ${res.data.membresia.vence}` : '';
      const puntos = res.data.puntos > 0 ? ` · +${res.data.puntos} puntos` : '';
      setOutcome({ ok: true, title: `${res.data.mensaje}, ${res.data.nombre}`, detail: `${c.actividad} ${day(c.inicio)} ${range(c)}${vence}${puntos}` });
    } catch (error) {
      const problem = error instanceof ApiError ? (error.problem as (Problem & { nombre?: string }) | undefined) : undefined;
      if (problem?.status === 404) {
        setOutcome({ ok: false, title: 'DNI no registrado', detail: 'Verificá el número e intentá de nuevo.' });
      } else {
        setOutcome({ ok: false, title: problem?.nombre ?? 'No puede pasar', detail: error instanceof Error ? error.message : 'No se pudo registrar el ingreso.' });
      }
    } finally {
      setDni('');
      setBusy(false);
    }
  }

  return (
    <main className="page reception">
      <h1>Recepción</h1>
      <form className="inline-form" onSubmit={submit}>
        <label>DNI
          <input inputMode="numeric" pattern="[0-9]{7,8}" autoFocus autoComplete="off" value={dni} onChange={(e) => setDni(e.target.value.replace(/\D/g, '').slice(0, 8))} required />
        </label>
        <button type="submit" disabled={busy || dni.length < 7}>Ingresar</button>
      </form>
      {outcome && (
        <div className={outcome.ok ? 'checkin checkin-ok' : 'checkin checkin-denied'} role="status">
          <p className="checkin-title">{outcome.title}</p>
          <p>{outcome.detail}</p>
        </div>
      )}
    </main>
  );
}
