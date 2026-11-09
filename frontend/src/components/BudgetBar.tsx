// One budget as a labelled progress bar. The level is spelled out in words as
// well as colour (ok / hampir habis / lewat), so it never relies on colour alone.
import { formatRp } from '../lib/format';
import type { BudgetStatus } from '../lib/types';

export const levelText: Record<BudgetStatus['level'], string> = {
  ok: 'Aman',
  warning: 'Hampir habis',
  over: 'Lewat anggaran',
};

export function BudgetBar({ b, compact }: { b: BudgetStatus; compact?: boolean }) {
  const width = Math.min(100, b.percent);
  return (
    <div className={'budget-bar level-' + b.level + (compact ? ' compact' : '')}>
      <div className="bb-top">
        <span className="bb-name"><span className={'dot swatch-' + b.categoryColor} />{b.categoryName}</span>
        <span className="num bb-figures">
          {formatRp(b.spent)} <span className="muted">/ {formatRp(b.amount)}</span>
        </span>
      </div>
      <div className="bb-track" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.min(b.percent, 100)}
        aria-label={`${b.categoryName}: ${b.percent}% terpakai, ${levelText[b.level]}`}>
        <span style={{ width: `${width}%` }} />
      </div>
      {!compact && (
        <div className="bb-foot">
          <span className="bb-level">{levelText[b.level]} · {b.percent}%</span>
          <span className="num">{b.remaining >= 0 ? `Sisa ${formatRp(b.remaining)}` : `Lebih ${formatRp(-b.remaining)}`}</span>
        </div>
      )}
    </div>
  );
}
