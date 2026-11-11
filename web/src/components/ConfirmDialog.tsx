import { FormEvent, ReactNode, useCallback, useEffect, useRef, useState } from 'react';

// Confirmación accesible para operaciones sensibles (DISENO_FRONTEND §3.2 y §5):
// <dialog> nativo con showModal() contiene el foco, cierra con Escape y devuelve
// el foco al control que la abrió. Reemplaza window.confirm y window.prompt.
export interface ConfirmOptions {
  title: string;
  message: string;
  confirmLabel?: string;
  danger?: boolean;
  /** Si está presente, pide un valor (por ejemplo una capacidad). */
  input?: { label: string; type?: 'text' | 'number'; initial?: string; min?: number; max?: number; minLength?: number };
}

interface Pending extends ConfirmOptions {
  resolve: (value: string | null) => void;
}

/** Devuelve el diálogo a renderizar y una función que resuelve con el valor o null si se cancela. */
export function useConfirm(): [ReactNode, (options: ConfirmOptions) => Promise<string | null>] {
  const [pending, setPending] = useState<Pending | null>(null);
  const ask = useCallback((options: ConfirmOptions) => new Promise<string | null>((resolve) => setPending({ ...options, resolve })), []);
  const close = useCallback((value: string | null) => {
    setPending((current) => { current?.resolve(value); return null; });
  }, []);
  return [pending ? <ConfirmDialog key={pending.title + pending.message} options={pending} onClose={close} /> : null, ask];
}

function ConfirmDialog({ options, onClose }: { options: ConfirmOptions; onClose: (value: string | null) => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  const opener = useRef<Element | null>(document.activeElement);
  const [value, setValue] = useState(options.input?.initial ?? '');

  useEffect(() => {
    const dialog = ref.current;
    if (dialog && !dialog.open) dialog.showModal();
    const previous = opener.current;
    return () => { if (previous instanceof HTMLElement) previous.focus(); };
  }, []);

  function submit(event: FormEvent) {
    event.preventDefault();
    onClose(options.input ? value.trim() : '');
  }

  return (
    <dialog ref={ref} className="confirm-dialog" aria-labelledby="confirm-title" onCancel={(e) => { e.preventDefault(); onClose(null); }}>
      <form method="dialog" onSubmit={submit}>
        <span className="confirm-mark" aria-hidden="true">{options.danger ? '!' : '?'}</span>
        <h2 id="confirm-title">{options.title}</h2>
        <p>{options.message}</p>
        {options.input && (
          <label>
            {options.input.label}
            <input
              autoFocus
              required
              type={options.input.type ?? 'text'}
              min={options.input.min}
              max={options.input.max}
              minLength={options.input.minLength}
              value={value}
              onChange={(e) => setValue(e.target.value)}
            />
          </label>
        )}
        <div className="confirm-actions">
          <button type="button" className="button-secondary" onClick={() => onClose(null)} autoFocus={!options.input}>Volver</button>
          <button type="submit" className={options.danger ? 'button-danger' : undefined}>{options.confirmLabel ?? 'Confirmar'}</button>
        </div>
      </form>
    </dialog>
  );
}
