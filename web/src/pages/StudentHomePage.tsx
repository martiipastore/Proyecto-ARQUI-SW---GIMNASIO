import { useCallback, useEffect, useState } from 'react';
import { ApiError } from '../api/client';
import { useSession } from '../auth/session';

// Inicio del alumno: membresía (memberships.v1) y saldo (benefits.v1). Las
// próximas reservas se agregan en la etapa 4.
interface MembershipSummary {
  activa: boolean;
  membresia_vigente: { tipo: { nombre: string }; fecha_vencimiento: string } | null;
  total: number;
  items: { id: string; tipo: { nombre: string; clases: number | null }; fecha_inicio: string; fecha_vencimiento: string; condicion: string }[];
}

interface Balance {
  balance: number;
  updatedAt: string;
}

export function StudentHomePage() {
  const { user, authRequest } = useSession();
  const [memberships, setMemberships] = useState<MembershipSummary | null>(null);
  const [balance, setBalance] = useState<number | null>(null);
  const [accountPending, setAccountPending] = useState(false);
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    setBusy(true);
    setMessage('');
    try {
      const page = await authRequest<MembershipSummary>('/api/v1/memberships/me');
      setMemberships(page.data);
      try {
        const account = await authRequest<Balance>('/api/v1/benefits/me');
        setBalance(account.data.balance);
        setAccountPending(false);
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) {
          setAccountPending(true);
        } else {
          throw error;
        }
      }
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'No se pudieron cargar tus datos.');
    } finally {
      setBusy(false);
    }
  }, [authRequest]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const current = memberships?.membresia_vigente;
  return (
    <main className="page">
      <h1>{user ? `Hola, ${user.name}` : 'Inicio'}</h1>
      {message && <p role="alert">{message}</p>}
      <section className="club-card" aria-label="Membresía">
        <h2>Tu membresía</h2>
        {memberships === null ? (
          <p className="muted">Cargando…</p>
        ) : memberships.activa && current ? (
          <p>{current.tipo.nombre} vigente hasta el {current.fecha_vencimiento}.</p>
        ) : (
          <p>No tenés una membresía vigente, así que todavía no podés reservar clases. Tus puntos se conservan.</p>
        )}
        {memberships && memberships.items.length > 0 && (
          <ul>
            {memberships.items.map((m) => (
              <li key={m.id}>
                {m.tipo.nombre}: {m.fecha_inicio} al {m.fecha_vencimiento} — {m.condicion.toLocaleLowerCase('es-AR')}
                {m.tipo.clases !== null && ` · Cupo contratado: ${m.tipo.clases} clases`}
              </li>
            ))}
          </ul>
        )}
      </section>
      <section className="club-card" aria-label="Club de Beneficios">
        <h2>Club de Beneficios</h2>
        {accountPending ? (
          <p>Tu cuenta de puntos se está creando. Volvé a consultar en unos minutos.</p>
        ) : (
          <p>Puntos disponibles: <strong>{balance ?? '…'}</strong></p>
        )}
      </section>
      <button type="button" disabled={busy} onClick={() => void refresh()}>{busy ? 'Actualizando…' : 'Actualizar'}</button>
    </main>
  );
}
