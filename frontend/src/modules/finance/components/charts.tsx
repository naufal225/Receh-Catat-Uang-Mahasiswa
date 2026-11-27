// Hand-built SVG charts: one grouped bar chart and one pie, both interactive,
// keyboard reachable, and paired with a readable table so no value hides
// behind hover or colour.
import { useMemo, useRef, useState } from 'react';
import { formatRp, formatShort } from '../../../lib/format';
import type { Bucket, CategorySlice } from '../types';

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

/** One bucket, or a dragged run of neighbouring buckets folded into one period. */
function mergeBuckets(run: Bucket[]): Bucket {
  if (run.length === 1) return run[0];
  const first = run[0];
  const last = run[run.length - 1];
  return {
    key: `${first.key}..${last.key}`,
    label: `${first.label} – ${last.label}`,
    from: first.from,
    to: last.to,
    income: run.reduce((s, b) => s + b.income, 0),
    expense: run.reduce((s, b) => s + b.expense, 0),
  };
}

/** Click a bar to pick its period; drag across bars to pick all of them. */
export function BarChart({ buckets, selected, onSelect }: { buckets: Bucket[]; selected: Bucket | null; onSelect: (b: Bucket | null) => void }) {
  const [hover, setHover] = useState<number | null>(null);
  const [drag, setDrag] = useState<{ start: number; end: number } | null>(null);
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

  /** The bar under a screen x position, clamped to the first and last bar. */
  const indexAt = (svg: SVGSVGElement, clientX: number) => {
    const box = svg.getBoundingClientRect();
    const x = ((clientX - box.left) / box.width) * W;
    return Math.max(0, Math.min(buckets.length - 1, Math.floor((x - PAD.left) / band)));
  };
  /** Select the bars between two indexes; picking the current selection again clears it. */
  const pick = (a: number, b: number) => {
    const next = mergeBuckets(buckets.slice(Math.min(a, b), Math.max(a, b) + 1));
    onSelect(selected?.key === next.key ? null : next);
  };

  // The tooltip describes the run being dragged, otherwise the hovered bar.
  const lo = drag ? Math.min(drag.start, drag.end) : hover;
  const hi = drag ? Math.max(drag.start, drag.end) : hover;
  const run = lo !== null && hi !== null ? buckets.slice(lo, hi + 1) : [];
  const hb = run.length ? mergeBuckets(run) : null;
  const tipX = lo !== null && hi !== null ? ((PAD.left + (band * (lo + hi + 1)) / 2) / W) * 100 : 0;
  const tipY = hb ? (y(Math.max(...run.map((b) => Math.max(b.income, b.expense)))) / H) * 100 : 0;

  return (
    <div className="chart-wrap" ref={wrap}>
      <svg
        className="chart-svg"
        viewBox={`0 0 ${W} ${H}`}
        role="img"
        aria-label="Diagram batang pemasukan dan pengeluaran per periode"
        onPointerMove={(e) => {
          if (!drag) return;
          const end = indexAt(e.currentTarget, e.clientX);
          if (end !== drag.end) setDrag({ ...drag, end });
        }}
        onPointerUp={(e) => {
          if (!drag) return;
          pick(drag.start, indexAt(e.currentTarget, e.clientX));
          setDrag(null);
          setHover(null);
        }}
        onPointerCancel={() => setDrag(null)}
      >
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
          const isSel = drag
            ? i >= Math.min(drag.start, drag.end) && i <= Math.max(drag.start, drag.end)
            : !!selected && b.from >= selected.from && b.to <= selected.to;
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
              onPointerDown={(e) => {
                if (e.button !== 0) return;
                // Capture, so the drag keeps following even outside the chart.
                e.currentTarget.ownerSVGElement?.setPointerCapture(e.pointerId);
                setDrag({ start: i, end: i });
              }}
              // Mouse and touch go through the pointer events; this serves assistive tech.
              onClick={(e) => e.detail === 0 && pick(i, i)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  pick(i, i);
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
          <strong>{run.length > 1 || hb.from === hb.to ? hb.label : `${hb.label} (${hb.from.slice(5)} – ${hb.to.slice(5)})`}</strong>
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
/* Pie chart: category share. Every category gets its own slice under  */
/* its real name, however small its share.                             */
/* ------------------------------------------------------------------ */

export function PieChart({ slices, total, active, onActive, centerLabel }: { slices: CategorySlice[]; total: number; active: number | null; onActive: (id: number | null) => void; centerLabel: string }) {
  const R = 92;
  const C = 110;
  const arcs = useMemo(() => {
    let a = 0;
    return slices.map((s) => {
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
