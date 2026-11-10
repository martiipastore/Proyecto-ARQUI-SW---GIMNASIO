import { FormEvent, useCallback, useEffect, useState } from 'react';
import { ApiError } from '../../api/client';
import { useSession } from '../../auth/session';
import { navigate } from '../../router';
import { Person, ROLE_LABELS } from './types';

// Personas (users.v1, HU-01 y HU-02): búsqueda, filtros, paginación y alta.
interface PersonPage {
  items: Person[];
  total: number;
  pagina: number;
  tamano: number;
}

const EMPTY_FORM = { rol: 'ALUMNO', nombre: '', apellido: '', dni: '', email: '', telefono: '', fecha_nacimiento: '', matricula: '' };
const PAGE_SIZE = 20;

export function PeoplePage() {
  const { authRequest } = useSession();
  const [role, setRole] = useState('');
  const [state, setState] = useState('ACTIVO');
  const [query, setQuery] = useState('');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [data, setData] = useState<PersonPage | null>(null);
  const [message, setMessage] = useState('');
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState(EMPTY_FORM);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    const params = new URLSearchParams({ pagina: String(page), tamano: String(PAGE_SIZE) });
    if (role) params.set('rol', role);
    if (state) params.set('estado', state);
    if (search) params.set('q', search);
    try {
      const res = await authRequest<PersonPage>(`/api/v1/users?${params.toString()}`);
      setData(res.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'No se pudo cargar el listado.');
    }
  }, [authRequest, page, role, state, search]);

  useEffect(() => {
    void load();
  }, [load]);

  async function create(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setMessage('');
    try {
      const body: Record<string, string> = { ...form };
      if (body.rol !== 'NUTRICIONISTA') delete body.matricula;
      const res = await authRequest<Person>('/api/v1/users', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      setForm(EMPTY_FORM);
      setShowForm(false);
      navigate(`/admin/personas/${res.data.id}`);
    } catch (error) {
      if (error instanceof ApiError && error.problem?.code === 'PERSONA_DUPLICADA') {
        setMessage('Ya existe una persona con ese DNI o email.');
      } else {
        setMessage(error instanceof Error ? error.message : 'No se pudo dar de alta.');
      }
    } finally {
      setBusy(false);
    }
  }

  const field = (name: keyof typeof EMPTY_FORM) => ({
    value: form[name],
    onChange: (e: { target: { value: string } }) => setForm({ ...form, [name]: e.target.value }),
  });
  const totalPages = data ? Math.max(1, Math.ceil(data.total / PAGE_SIZE)) : 1;

  return (
    <main className="page">
      <h1>Personas</h1>
      {message && <p role="alert">{message}</p>}
      <form className="filters" onSubmit={(e) => { e.preventDefault(); setPage(1); setSearch(query.trim()); }}>
        <label>
          Buscar
          <input type="search" placeholder="DNI, nombre, apellido o email" maxLength={80} value={query} onChange={(e) => setQuery(e.target.value)} />
        </label>
        <label>
          Rol
          <select value={role} onChange={(e) => { setRole(e.target.value); setPage(1); }}>
            <option value="">Todos</option>
            <option value="ALUMNO">Alumnos</option>
            <option value="PROFESOR">Profesores</option>
            <option value="NUTRICIONISTA">Nutricionistas</option>
            <option value="ADMINISTRADOR">Administradores</option>
          </select>
        </label>
        <label>
          Estado
          <select value={state} onChange={(e) => { setState(e.target.value); setPage(1); }}>
            <option value="">Todos</option>
            <option value="ACTIVO">Activos</option>
            <option value="INACTIVO">Dados de baja</option>
          </select>
        </label>
        <button type="submit">Buscar</button>
        <button type="button" onClick={() => setShowForm(!showForm)}>{showForm ? 'Cancelar alta' : 'Nueva persona'}</button>
      </form>

      {showForm && (
        <form className="club-card" onSubmit={create} aria-label="Alta de persona">
          <h2>Nueva persona</h2>
          <label>
            Rol
            <select {...field('rol')}>
              <option value="ALUMNO">Alumno</option>
              <option value="PROFESOR">Profesor</option>
              <option value="NUTRICIONISTA">Nutricionista</option>
            </select>
          </label>
          <label>Nombre<input required maxLength={80} {...field('nombre')} /></label>
          <label>Apellido<input required maxLength={80} {...field('apellido')} /></label>
          <label>DNI<input required inputMode="numeric" pattern="[0-9]{7,8}" title="7 u 8 dígitos" {...field('dni')} /></label>
          <label>Email<input type="email" required maxLength={254} {...field('email')} /></label>
          <label>Teléfono (opcional)<input maxLength={30} {...field('telefono')} /></label>
          <label>
            Fecha de nacimiento{form.rol === 'ALUMNO' ? '' : ' (opcional)'}
            <input type="date" required={form.rol === 'ALUMNO'} min="1900-01-01" {...field('fecha_nacimiento')} />
          </label>
          {form.rol === 'NUTRICIONISTA' && <label>Matrícula (opcional)<input maxLength={40} {...field('matricula')} /></label>}
          <p className="muted">La persona recibe por correo un enlace para elegir su contraseña.</p>
          <button type="submit" disabled={busy}>{busy ? 'Guardando…' : 'Dar de alta'}</button>
        </form>
      )}

      {data === null ? (
        <p className="muted">Cargando…</p>
      ) : data.items.length === 0 ? (
        <p>No hay personas con esos filtros.</p>
      ) : (
        <table className="data-table">
          <thead>
            <tr><th>Apellido y nombre</th><th>DNI</th><th>Rol</th><th>Estado</th></tr>
          </thead>
          <tbody>
            {data.items.map((p) => (
              <tr key={p.id}>
                <td><a href={`#/admin/personas/${p.id}`}>{p.apellido ? `${p.apellido}, ${p.nombre}` : p.nombre}</a></td>
                <td>{p.dni ?? '—'}</td>
                <td>{ROLE_LABELS[p.rol] ?? p.rol}</td>
                <td>{p.estado === 'ACTIVO' ? (p.activacion_pendiente ? 'Activo (activación pendiente)' : 'Activo') : 'Dado de baja'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {data && data.total > PAGE_SIZE && (
        <nav className="pagination" aria-label="Páginas">
          <button type="button" disabled={page <= 1} onClick={() => setPage(page - 1)}>Anterior</button>
          <span>Página {page} de {totalPages}</span>
          <button type="button" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>Siguiente</button>
        </nav>
      )}
    </main>
  );
}
