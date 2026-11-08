import { formatDate, formatRp } from '../lib/format';
import type { Transaction } from '../lib/types';
import { Icon } from './Icon';
import { initial } from './ui';

/** One transaction line. Identity is carried by the category tile + its name, never colour alone. */
export function TxRow({ t, showDate = true, actions }: { t: Transaction; showDate?: boolean; actions?: React.ReactNode }) {
  if (t.type === 'transfer_out' || t.type === 'transfer_in') return <TransferRow t={t} showDate={showDate} actions={actions} />;
  const income = t.type === 'income';
  return (
    <div className="tx-row">
      <div className={'tx-icon swatch-' + t.categoryColor} aria-hidden="true">
        {initial(t.categoryName)}
      </div>
      <div className="tx-main">
        <div className="tx-name">{t.name}</div>
        <div className="tx-meta">
          <span>{t.categoryName}</span>
          <span aria-hidden="true">·</span>
          <span>{t.walletName}</span>
          {showDate && (
            <>
              <span aria-hidden="true">·</span>
              <span>{formatDate(t.occurredOn)}</span>
            </>
          )}
          {t.recurringId && (
            <span className="badge badge-neutral" title="Dicatat otomatis dari jadwal rutin">
              <Icon name="repeat" size={12} /> Rutin
            </span>
          )}
          {t.transferId && (
            <span className="badge badge-neutral" title="Biaya dari transfer antar dompet">
              <Icon name="transfer" size={12} /> Transfer
            </span>
          )}
        </div>
      </div>
      <div className={'tx-amount ' + (income ? 'income-text' : 'expense-text')}>
        <span className="sr-only">{income ? 'Pemasukan' : 'Pengeluaran'} </span>
        {formatRp(income ? t.amount : -t.amount, true)}
      </div>
      {actions}
    </div>
  );
}

/**
 * A transfer reads as a route, "BCA → GoPay", whichever leg this row is. The
 * amount is neutral and unsigned: the money stayed the user's, and the route
 * already says which way it went.
 */
function TransferRow({ t, showDate, actions }: { t: Transaction; showDate: boolean; actions?: React.ReactNode }) {
  const out = t.type === 'transfer_out';
  const [fromName, toName] = out ? [t.walletName, t.counterpartName] : [t.counterpartName, t.walletName];
  const note = t.name && t.name !== 'Transfer' ? t.name : '';
  return (
    <div className="tx-row">
      <div className="tx-icon tx-icon-transfer" aria-hidden="true">
        <Icon name="transfer" size={18} />
      </div>
      <div className="tx-main">
        <div className="tx-name">
          {fromName} <span className="tx-arrow" aria-label="ke">→</span> {toName}
        </div>
        <div className="tx-meta">
          <span>{note || 'Transfer antar dompet'}</span>
          {showDate && (
            <>
              <span aria-hidden="true">·</span>
              <span>{formatDate(t.occurredOn)}</span>
            </>
          )}
          {out && !!t.transferFee && (
            <>
              <span aria-hidden="true">·</span>
              <span>biaya {formatRp(t.transferFee)}</span>
            </>
          )}
        </div>
      </div>
      <div className="tx-amount transfer-text">
        <span className="sr-only">Transfer </span>
        {formatRp(t.amount)}
      </div>
      {actions}
    </div>
  );
}
