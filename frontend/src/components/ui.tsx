// Shared building blocks: modal, toast, confirm, field, empty state, etc.
import { createContext, useCallback, useContext, useEffect, useId, useRef, useState, type ReactNode } from 'react';
import { navigate } from '../lib/router';
import { digitsOnly, groupDigits } from '../lib/format';
import { Icon, type IconName } from './Icon';

/* ---------- Link (client-side navigation) ---------- */
export function Link({ to, children, className, onClick, ...rest }: { to: string; children: ReactNode; className?: string } & React.AnchorHTMLAttributes<HTMLAnchorElement>) {
  return (
    <a
      href={to}
      className={className}
      {...rest}
      onClick={(e) => {
        // The caller's handler (e.g. closing a modal) runs first, then the
        // client-side navigation; spreading it would replace the navigation.
        onClick?.(e);
        if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
        e.preventDefault();
        navigate(to);
      }}
    >
      {children}
    </a>
  );
}

/* ---------- Modal (native <dialog>: focus trap, Esc and inert background for free) ---------- */
export function Modal({ open, onClose, title, children }: { open: boolean; onClose: () => void; title: string; children: ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) {
      d.showModal();
      // showModal() moves focus to the first focusable element (the close
      // button), overriding React's autoFocus, so place focus afterwards.
      const target =
        d.querySelector<HTMLElement>('[data-autofocus]') ??
        d.querySelector<HTMLElement>('.modal-body input:not([type=hidden]), .modal-body select, .modal-body textarea');
      target?.focus();
    }
    if (!open && d.open) d.close();
  }, [open]);
  return (
    <dialog
      ref={ref}
      className="modal"
      aria-labelledby={titleId}
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
      onMouseDown={(e) => {
        if (e.target === ref.current) onClose(); // Click on the backdrop.
      }}
    >
      {open && (
        <>
          <div className="modal-head">
            <h2 id={titleId}>{title}</h2>
            <button type="button" className="icon-btn" onClick={onClose} aria-label="Tutup">
              <Icon name="close" />
            </button>
          </div>
          <div className="modal-body">{children}</div>
        </>
      )}
    </dialog>
  );
}

/* ---------- Toasts ---------- */
type Toast = { id: number; text: string; tone: 'ok' | 'error' };
const ToastCtx = createContext<(text: string, tone?: Toast['tone']) => void>(() => {});
export const useToast = () => useContext(ToastCtx);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const push = useCallback((text: string, tone: Toast['tone'] = 'ok') => {
    const id = Date.now() + Math.random();
    setToasts((t) => [...t.slice(-2), { id, text, tone }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), tone === 'error' ? 5200 : 3200);
  }, []);
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="toasts" role="status" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} className={'toast' + (t.tone === 'error' ? ' toast-error' : '')}>
            <Icon name={t.tone === 'error' ? 'alert' : 'check'} size={18} />
            {t.text}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

/* ---------- Confirm dialog (replaces window.confirm) ---------- */
type ConfirmOpts = { title: string; body: ReactNode; confirmLabel: string; danger?: boolean };
const ConfirmCtx = createContext<(o: ConfirmOpts) => Promise<boolean>>(async () => false);
export const useConfirm = () => useContext(ConfirmCtx);

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<(ConfirmOpts & { resolve: (v: boolean) => void }) | null>(null);
  const ask = useCallback((o: ConfirmOpts) => new Promise<boolean>((resolve) => setState({ ...o, resolve })), []);
  const close = (v: boolean) => {
    state?.resolve(v);
    setState(null);
  };
  return (
    <ConfirmCtx.Provider value={ask}>
      {children}
      <Modal open={!!state} onClose={() => close(false)} title={state?.title ?? ''}>
        <div style={{ color: 'var(--ink-2)', marginBottom: 20 }}>{state?.body}</div>
        <div className="modal-actions">
          <button type="button" className="btn btn-ghost" onClick={() => close(false)}>
            Batal
          </button>
          <button type="button" className={'btn ' + (state?.danger ? 'btn-danger' : 'btn-primary')} onClick={() => close(true)} data-autofocus>
            {state?.confirmLabel}
          </button>
        </div>
      </Modal>
    </ConfirmCtx.Provider>
  );
}

/* ---------- Form pieces ---------- */
export function Field({ label, error, hint, children, htmlFor }: { label: string; error?: string; hint?: string; children: ReactNode; htmlFor?: string }) {
  return (
    <div className="field">
      <label htmlFor={htmlFor}>{label}</label>
      {children}
      {error ? (
        <span className="field-error" role="alert">
          {error}
        </span>
      ) : hint ? (
        <span className="field-hint">{hint}</span>
      ) : null}
    </div>
  );
}

export function AmountInput({ id, value, onChange, invalid, autoFocus }: { id?: string; value: number; onChange: (v: number) => void; invalid?: boolean; autoFocus?: boolean }) {
  return (
    <div className="amount-input" style={invalid ? { borderColor: 'var(--danger)' } : undefined}>
      <span>Rp</span>
      <input
        id={id}
        inputMode="numeric"
        autoComplete="off"
        placeholder="0"
        value={groupDigits(value)}
        onChange={(e) => onChange(digitsOnly(e.target.value))}
        aria-invalid={invalid || undefined}
        autoFocus={autoFocus}
        data-autofocus={autoFocus || undefined}
      />
    </div>
  );
}

export function PasswordInput({ id, value, onChange, autoComplete, invalid }: { id: string; value: string; onChange: (v: string) => void; autoComplete: string; invalid?: boolean }) {
  const [show, setShow] = useState(false);
  return (
    <div className="pw-wrap">
      <input
        id={id}
        className="input"
        type={show ? 'text' : 'password'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        autoComplete={autoComplete}
        aria-invalid={invalid || undefined}
        required
      />
      <button type="button" className="icon-btn" onClick={() => setShow((s) => !s)} aria-label={show ? 'Sembunyikan kata sandi' : 'Tampilkan kata sandi'}>
        <Icon name={show ? 'eyeOff' : 'eye'} size={18} />
      </button>
    </div>
  );
}

export const SLOTS = ['c1', 'c2', 'c3', 'c4', 'c5', 'c6', 'c7', 'c8', 'c0'];

export function ColorPicker({ value, onChange }: { value: string; onChange: (c: string) => void }) {
  return (
    <div className="color-pick" role="group" aria-label="Pilih warna">
      {SLOTS.map((c, i) => (
        <button
          key={c}
          type="button"
          className={'swatch-' + c}
          aria-pressed={value === c}
          aria-label={`Warna ${i + 1}`}
          onClick={() => onChange(c)}
        />
      ))}
    </div>
  );
}

export function Segmented<T extends string>({ value, options, onChange, big, label }: { value: T; options: { value: T; label: string; tone?: string }[]; onChange: (v: T) => void; big?: boolean; label: string }) {
  return (
    <div className={'segmented' + (big ? ' big' : '')} role="group" aria-label={label}>
      {options.map((o) => (
        <button key={o.value} type="button" aria-pressed={value === o.value} data-tone={o.tone} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

/* ---------- Feedback ---------- */
export function Empty({ icon, title, children, action }: { icon: IconName; title: string; children?: ReactNode; action?: ReactNode }) {
  return (
    <div className="empty">
      <div className="empty-art">
        <Icon name={icon} size={28} />
      </div>
      <h3>{title}</h3>
      {children && <p>{children}</p>}
      {action}
    </div>
  );
}

export function ErrorNote({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="alert alert-danger" role="alert">
      <Icon name="alert" size={18} />
      <div style={{ flex: 1 }}>{message}</div>
      {onRetry && (
        <button type="button" className="btn btn-sm btn-ghost" onClick={onRetry}>
          Coba lagi
        </button>
      )}
    </div>
  );
}

export function Skeleton({ h = 16, w = '100%', r }: { h?: number; w?: number | string; r?: number }) {
  return <div className="skeleton" style={{ height: h, width: w, borderRadius: r }} />;
}

/** First letter for an icon tile. */
export function initial(s: string) {
  return (s.trim()[0] ?? '?').toUpperCase();
}
