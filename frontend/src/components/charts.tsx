// Hand-built SVG charts: one grouped bar chart and one pie, both interactive,
// keyboard reachable, and paired with a readable table so no value hides
// behind hover or colour.
import { useMemo, useRef, useState } from 'react';
import { formatRp, formatShort } from '../lib/format';
import type { Bucket, CategorySlice } from '../lib/types';

/* ------------------------------------------------------------------ */
/* Grouped bar chart: income vs expense per time bucket.               */
/* ------------------------------------------------------------------ */

const W = 640;
const H = 260;
const PAD = { top: 12, right: 8, bottom: 30, left: 52 };

/** Axis top as four round steps (1, 2, 2.5 or 5 × 10^n), so every tick reads cleanly. */
function niceMax(v: number): number {
  if (v <= 0) return 100_000;
  const raw = v / 4;
  const exp = Math.pow(10, Math.floor(Math.log10(raw)));
  const n = raw / exp;
  const step = n <= 1 ? 1 : n <= 2 ? 2 : n <= 2.5 ? 2.5 : n <= 5 ? 5 : 10;
  return step * exp * 4;
}

/** Rounded top corners only, so a bar stays anchored flat on the baseline. */
function barPath(x: number, y: number, w: number, h: number, r: number) {
  if (h <= 0) return '';
  const rr = Math.min(r, w / 2, h);
  return `M${x},${y + h}V${y + rr}Q${x},${y} ${x + rr},${y}H${x + w - rr}Q${x + w},${y} ${x + w},${y + rr}V${y + h}Z`;
}

export function BarChart({ buckets, selected, onSelect }: { buckets: Bucket[]; selected: string | null; onSelect: (b: Bucket | null) => void }) {
  const [hover, setHover] = useState<number | null>(null);
  const wrap = useRef<HTMLDivElement>(null);
  const max = niceMax(Math.max(...buckets.map((b) => Math.max(b.income, b.expense)), 0));
  const plotW = W - PAD.left - PAD.right;
  const plotH = H - PAD.top - PAD.bottom;
  const band = plotW / buckets.length;
  const barW = Math.max(3, Math.min(18, band * 0.3));
  const gap = 2; // Surface gap between the two bars of a group.
  const y = (v: number) => PAD.top + plotH - (v / max) * plotH;
  const ticks = [0, 0.25, 0.5, 0.75, 1].map((f) => f * max);
  // Thin out x labels when buckets are dense.
  const labelEvery = buckets.length > 12 ? 2 : 1;

  const hb = hover !== null ? buckets[hover] : null;
  const tipX = hover !== null ? ((PAD.left + band * hover + band / 2) / W) * 100 : 0;
  const tipY = hb ? (y(Math.max(hb.income, hb.expense)) / H) * 100 : 0;

  return (
    <div className="chart-wrap" ref={wrap}>
      <svg className="chart-svg" viewBox={`0 0 ${W} ${H}`} role="img" aria-label="Diagram batang pemasukan dan pengeluaran per periode">
        {ticks.map((t) => (
          <g key={t}>
            <line x1={PAD.left} x2={W - PAD.right} y1={y(t)} y2={y(t)} stroke={t === 0 ? 'var(--viz-axis)' : 'var(--viz-grid)'} strokeWidth={1} />
            <text x={PAD.left - 8} y={y(t) + 4} textAnchor="end" className="num">
              {formatShort(t)}
            </text>
          </g>
        ))}
        {buckets.map((b, i) => {
          const cx = PAD.left + band * i + band / 2;
          const isSel = selected === b.key;
          return (
            <g
              key={b.key}
              className="bar-hit"
              tabIndex={0}
              role="button"
              aria-pressed={isSel}
              aria-label={`${b.label}: pemasukan ${formatRp(b.income)}, pengeluaran ${formatRp(b.expense)}`}
              onMouseEnter={() => setHover(i)}
              onMouseLeave={() => setHover(null)}
              onFocus={() => setHover(i)}
              onBlur={() => setHover(null)}
              onClick={() => onSelect(isSel ? null : b)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  onSelect(isSel ? null : b);
                }
              }}
            >
              {/* Hit area spans the whole band, far larger than the marks. */}
              <rect className={'bar-hover' + (isSel ? ' bar-selected' : '')} x={PAD.left + band * i + 1} y={PAD.top} width={band - 2} height={plotH} rx={6} />
              <path d={barPath(cx - barW - gap / 2, y(b.income), barW, PAD.top + plotH - y(b.income), 4)} fill="var(--viz-income)" />
              <path d={barPath(cx + gap / 2, y(b.expense), barW, PAD.top + plotH - y(b.expense), 4)} fill="var(--viz-expense)" />
              {i % labelEvery === 0 && (
                <text x={cx} y={H - 10} textAnchor="middle" style={isSel ? { fill: 'var(--ink)', fontWeight: 700 } : undefined}>
                  {b.label}
                </text>
              )}
            </g>
          );
        })}
      </svg>
      {hb && (
        <div className="tooltip" style={{ left: `${tipX}%`, top: `${tipY}%` }}>
          <strong>{hb.from === hb.to ? hb.label : `${hb.label} (${hb.from.slice(5)} – ${hb.to.slice(5)})`}</strong>
          <div>
            <span><i style={{ background: 'var(--viz-income)' }} />Pemasukan</span>
            <span className="num">{formatRp(hb.income)}</span>
          </div>
          <div>
            <span><i style={{ background: 'var(--viz-expense)' }} />Pengeluaran</span>
            <span className="num">{formatRp(hb.expense)}</span>
          </div>
          <div style={{ marginTop: 4, opacity: 0.8 }}>
            <span>Selisih</span>
            <span className="num">{formatRp(hb.income - hb.expense, true)}</span>
          </div>
        </div>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Pie chart: category share. Folded to <= 6 slices for readability;   */
/* the table beside it lists every category.                           */
/* ------------------------------------------------------------------ */

export function foldSlices(slices: CategorySlice[]): CategorySlice[] {
  if (slices.length <= 6) return slices;
  const rest = slices.slice(5).reduce((s, c) => s + c.total, 0);
  return [...slices.slice(0, 5), { categoryId: -1, name: 'Kategori lain', color: 'c0', total: rest }];
}

export function PieChart({ slices, total, active, onActive, centerLabel }: { slices: CategorySlice[]; total: number; active: number | null; onActive: (id: number | null) => void; centerLabel: string }) {
  const R = 92;
  const C = 110;
  const arcs = useMemo(() => {
    let a = 0;
    return foldSlices(slices).map((s) => {
      const sweep = total ? (s.total / total) * Math.PI * 2 : 0;
      const start = a;
      a += sweep;
      return { s, start, end: a };
    });
  }, [slices, total]);
  const pt = (ang: number) => [C + R * Math.sin(ang), C - R * Math.cos(ang)];
  const activeSlice = arcs.find((x) => x.s.categoryId === active)?.s;

  return (
    <svg viewBox="0 0 220 220" width="100%" style={{ maxWidth: 230 }} role="img" aria-label="Diagram lingkaran per kategori" className={active !== null ? 'pie-dim' : undefined}>
      {arcs.length === 1 ? (
        <circle cx={C} cy={C} r={R} className={'slice active swatch-' + arcs[0].s.color} style={{ fill: `var(--${arcs[0].s.color})` }} />
      ) : (
        arcs.map(({ s, start, end }) => {
          const [x1, y1] = pt(start);
          const [x2, y2] = pt(end);
          const large = end - start > Math.PI ? 1 : 0;
          return (
            <path
              key={s.categoryId}
              className={'slice' + (s.categoryId === active ? ' active' : '')}
              d={`M${C},${C}L${x1},${y1}A${R},${R} 0 ${large} 1 ${x2},${y2}Z`}
              fill={`var(--${s.color})`}
              stroke="var(--surface)"
              strokeWidth={2}
              tabIndex={0}
              role="button"
              aria-label={`${s.name}: ${formatRp(s.total)} (${Math.round((s.total / total) * 100)}%)`}
              onMouseEnter={() => onActive(s.categoryId)}
              onMouseLeave={() => onActive(null)}
              onFocus={() => onActive(s.categoryId)}
              onBlur={() => onActive(null)}
            />
          );
        })
      )}
      {/* A donut hole carries the headline figure. */}
      <circle cx={C} cy={C} r={54} fill="var(--surface)" />
      <text x={C} y={C - 6} textAnchor="middle" fontSize="11" fill="var(--muted)" className="pie-center">
        {activeSlice ? activeSlice.name.slice(0, 16) : centerLabel}
      </text>
      <text x={C} y={C + 14} textAnchor="middle" fontSize="15" fontWeight="800" fill="var(--ink)" className="pie-center">
        {formatShort(activeSlice ? activeSlice.total : total)}
      </text>
    </svg>
  );
}
