// Cron schedule helpers shared by the rotation-schedule builder.
//
// Mirrors the firmware engine (esp32-photoframe main/cron.{c,h}): simplified
// 3-field expressions "minute hour day-of-week", with *, a, a-b, */n, a-b/n and
// comma lists; day-of-week 0/7 = Sunday. Ported from the device webapp's
// cron.js (kept in sync).

export interface CronRule {
  minute: Set<number>;
  hour: Set<number>;
  dow: Set<number>;
}

export interface QuietHours {
  enabled: boolean;
  start: number; // minutes since midnight
  end: number;
}

export type DaysSpec = 'everyday' | 'weekdays' | 'weekends' | number[];

export interface ScheduleCard {
  days: DaysSpec;
  mode: 'interval' | 'times';
  every: number;
  unit: 'hours' | 'minutes';
  fromHour: number;
  toHour: number;
  times: string[];
  raw: string | null;
}

const FIELD_RANGES: [number, number][] = [
  [0, 59], // minute
  [0, 23], // hour
  [0, 6], // day of week
];

function parseField(
  field: string,
  lo: number,
  hi: number,
  isDow: boolean
): Set<number> | null {
  if (field === '') return null;
  const out = new Set<number>();
  for (const term of field.split(',')) {
    const m = term.match(/^(\*|\d+)(?:-(\d+))?(?:\/(\d+))?$/);
    if (!m) return null;
    let start: number;
    let end: number;
    let step = 1;
    if (m[1] === '*') {
      start = lo;
      end = hi;
    } else {
      start = parseInt(m[1], 10);
      end = m[2] !== undefined ? parseInt(m[2], 10) : start;
      if (m[2] === undefined && m[3] !== undefined) end = hi;
    }
    if (m[3] !== undefined) {
      step = parseInt(m[3], 10);
      if (step <= 0) return null;
    }
    if (isDow) {
      if (start === 7) start = 0;
      if (end === 7) end = 0;
    }
    if (start < lo || end > hi || start > end) return null;
    for (let v = start; v <= end; v += step) out.add(v);
  }
  return out;
}

export function parseCron(expr: string): CronRule | null {
  if (typeof expr !== 'string') return null;
  const fields = expr.trim().split(/\s+/);
  if (fields.length !== 3) return null;
  const sets: Set<number>[] = [];
  for (let i = 0; i < 3; i++) {
    const s = parseField(
      fields[i],
      FIELD_RANGES[i][0],
      FIELD_RANGES[i][1],
      i === 2
    );
    if (!s) return null;
    sets.push(s);
  }
  return { minute: sets[0], hour: sets[1], dow: sets[2] };
}

export function isValidCron(expr: string): boolean {
  return parseCron(expr) !== null;
}

function matches(r: CronRule, d: Date): boolean {
  if (!r.minute.has(d.getMinutes())) return false;
  if (!r.hour.has(d.getHours())) return false;
  return r.dow.has(d.getDay());
}

function inQuietWindow(d: Date, sleep: QuietHours | null): boolean {
  if (!sleep || !sleep.enabled) return false;
  const cur = d.getHours() * 60 + d.getMinutes();
  if (sleep.start > sleep.end) return cur >= sleep.start || cur < sleep.end;
  return cur >= sleep.start && cur < sleep.end;
}

const HORIZON_MIN = 8 * 24 * 60;
const MIN_DELAY_MS = 60 * 1000;

export function nextRuns(
  exprs: string[],
  from: Date = new Date(),
  count = 5,
  sleep: QuietHours | null = null
): Date[] {
  const rules = exprs.map(parseCron).filter((r): r is CronRule => r !== null);
  if (!rules.length) return [];
  const t0 = from.getTime();
  const startMs = Math.ceil((t0 + 1) / 60000) * 60000;
  const runs: Date[] = [];
  for (let i = 0; i < HORIZON_MIN && runs.length < count; i++) {
    const d = new Date(startMs + i * 60000);
    if (d.getTime() - t0 < MIN_DELAY_MS) continue;
    if (!rules.some((r) => matches(r, d))) continue;
    if (inQuietWindow(d, sleep)) continue;
    runs.push(d);
  }
  return runs;
}

export const DAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

export function newCard(): ScheduleCard {
  return {
    days: 'everyday',
    mode: 'interval',
    every: 12,
    unit: 'hours',
    fromHour: 0,
    toHour: 23,
    times: ['08:00'],
    raw: null,
  };
}

function daysToCron(days: DaysSpec): string {
  if (days === 'everyday') return '*';
  if (days === 'weekdays') return '1-5';
  if (days === 'weekends') return '0,6';
  if (Array.isArray(days)) {
    if (days.length === 0 || days.length === 7) return '*';
    return [...days].sort((a, b) => a - b).join(',');
  }
  return '*';
}

function hourRange(fromHour: number, toHour: number): string {
  if (fromHour <= 0 && toHour >= 23) return '*';
  return fromHour === toHour ? `${fromHour}` : `${fromHour}-${toHour}`;
}

export function compileCard(card: ScheduleCard): string[] {
  if (card.raw) return [card.raw.trim()];
  const dow = daysToCron(card.days);

  if (card.mode === 'interval') {
    if (card.unit === 'minutes') {
      const min = card.every > 1 ? `*/${card.every}` : '*';
      return [`${min} ${hourRange(card.fromHour, card.toHour)} ${dow}`];
    }
    const hr =
      card.fromHour <= 0 && card.toHour >= 23
        ? card.every > 1
          ? `*/${card.every}`
          : '*'
        : `${hourRange(card.fromHour, card.toHour)}/${card.every}`;
    return [`0 ${hr} ${dow}`];
  }

  const byMinute = new Map<number, number[]>();
  for (const t of card.times) {
    const [h, m] = t.split(':').map(Number);
    if (Number.isNaN(h) || Number.isNaN(m)) continue;
    if (!byMinute.has(m)) byMinute.set(m, []);
    byMinute.get(m)!.push(h);
  }
  const rules: string[] = [];
  for (const [m, hours] of byMinute) {
    const uniq = [...new Set(hours)].sort((a, b) => a - b);
    rules.push(`${m} ${uniq.join(',')} ${dow}`);
  }
  return rules.length ? rules : [`0 * ${dow}`];
}

export function compileCards(cards: ScheduleCard[]): string[] {
  return cards.flatMap(compileCard);
}

function cronDowToDays(field: string): DaysSpec {
  if (field === '*') return 'everyday';
  if (field === '1-5') return 'weekdays';
  if (field === '0,6' || field === '6,0') return 'weekends';
  const set = parseField(field, 0, 6, true);
  if (!set) return 'everyday';
  return [...set].sort((a, b) => a - b);
}

export function cardFromCron(expr: string): ScheduleCard {
  const card = newCard();
  const fields = expr.trim().split(/\s+/);
  if (fields.length !== 3 || !parseCron(expr)) {
    card.raw = expr.trim();
    return card;
  }
  const [minF, hourF, dowF] = fields;
  card.days = cronDowToDays(dowF);

  const stepMin = minF.match(/^\*\/(\d+)$/);
  const stepHour = hourF.match(/^(?:\*|(\d+)-(\d+))\/(\d+)$/);

  if (stepMin && (hourF === '*' || /^\d+-\d+$/.test(hourF))) {
    card.mode = 'interval';
    card.unit = 'minutes';
    card.every = parseInt(stepMin[1], 10);
    if (hourF === '*') {
      card.fromHour = 0;
      card.toHour = 23;
    } else {
      const [a, b] = hourF.split('-').map(Number);
      card.fromHour = a;
      card.toHour = b;
    }
    return card;
  }

  if (minF === '0' && stepHour) {
    card.mode = 'interval';
    card.unit = 'hours';
    card.every = parseInt(stepHour[3], 10);
    card.fromHour = stepHour[1] !== undefined ? parseInt(stepHour[1], 10) : 0;
    card.toHour = stepHour[2] !== undefined ? parseInt(stepHour[2], 10) : 23;
    return card;
  }

  if (/^\d+$/.test(minF) && /^[\d,]+$/.test(hourF)) {
    card.mode = 'times';
    const m = minF.padStart(2, '0');
    card.times = hourF
      .split(',')
      .map((h) => `${h.padStart(2, '0')}:${m}`)
      .sort();
    return card;
  }

  card.raw = expr.trim();
  return card;
}

export function cardsFromCron(
  exprs: string[] | undefined | null
): ScheduleCard[] {
  if (!exprs || !exprs.length) return [newCard()];
  return exprs.map(cardFromCron);
}

function daysLabel(days: DaysSpec): string {
  if (days === 'everyday') return 'every day';
  if (days === 'weekdays') return 'weekdays';
  if (days === 'weekends') return 'weekends';
  if (Array.isArray(days)) {
    if (days.length === 0 || days.length === 7) return 'every day';
    return days
      .slice()
      .sort((a, b) => a - b)
      .map((d) => DAY_LABELS[d])
      .join(', ');
  }
  return 'every day';
}

function hour12(h: number): string {
  const ampm = h < 12 ? 'AM' : 'PM';
  const hr = h % 12 === 0 ? 12 : h % 12;
  return `${hr} ${ampm}`;
}

function timeLabel(hhmm: string): string {
  const [h, m] = hhmm.split(':').map(Number);
  const ampm = h < 12 ? 'AM' : 'PM';
  const hr = h % 12 === 0 ? 12 : h % 12;
  return `${hr}:${String(m).padStart(2, '0')} ${ampm}`;
}

export function describeCard(card: ScheduleCard): string {
  if (card.raw) return `Custom: ${card.raw}`;
  const days = daysLabel(card.days);
  if (card.mode === 'interval') {
    const unit = card.unit === 'hours' ? 'hours' : 'minutes';
    const every =
      card.every === 1
        ? `every ${card.unit === 'hours' ? 'hour' : 'minute'}`
        : `every ${card.every} ${unit}`;
    const window =
      card.fromHour <= 0 && card.toHour >= 23
        ? ''
        : `, ${hour12(card.fromHour)}–${hour12(card.toHour)}`;
    return `Rotate ${every}${window}, ${days}`;
  }
  const times = card.times.map(timeLabel).join(', ');
  return `Rotate at ${times}, ${days}`;
}
