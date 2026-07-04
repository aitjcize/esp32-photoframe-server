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
  [0, 7], // day of week (0 and 7 both mean Sunday, as in Vixie cron)
];

function parseField(
  field: string,
  lo: number,
  hi: number,
  isDow: boolean
): Set<number> | null {
  // The firmware caps each field at 31 chars (cron.c fields[3][32]).
  if (field === '' || field.length > 31) return null;
  const out = new Set<number>();
  for (const term of field.split(',')) {
    // A range may only follow a number, never '*' (the firmware rejects "*-5").
    const m = term.match(/^(?:\*|(\d+)(?:-(\d+))?)(?:\/(\d+))?$/);
    if (!m) return null;
    let start: number;
    let end: number;
    let step = 1;
    if (m[1] === undefined) {
      // '*'
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
    if (start < lo || end > hi || start > end) return null;
    for (let v = start; v <= end; v += step) out.add(v);
  }
  // Day-of-week: 7 is an alias for Sunday (Vixie semantics), valid anywhere
  // including range ends, so "5-7" = Fri-Sun and "0-7" = every day.
  if (isDow && out.has(7)) {
    out.delete(7);
    out.add(0);
  }
  return out;
}

export function parseCron(expr: string): CronRule | null {
  if (typeof expr !== 'string') return null;
  // The firmware rejects rules of 64+ chars (CRON_RULE_MAX_LEN) and splits
  // fields only on spaces/tabs.
  if (expr.length >= 64) return null;
  const fields = expr.replace(/^[ \t]+|[ \t]+$/g, '').split(/[ \t]+/);
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
    // No minimum-delay guard: rotations never run ahead of their scheduled
    // minute (the firmware scans from the next whole minute), so an imminent
    // match is a legitimate target.
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
  // Advanced mode owns the card whenever raw is non-null — an emptied field
  // must compile to an (invalid) empty rule, not the hidden builder state.
  if (card.raw !== null) return [card.raw.trim()];
  const dow = daysToCron(card.days);

  if (card.mode === 'interval') {
    if (card.unit === 'minutes') {
      const min = card.every > 1 ? `*/${card.every}` : '*';
      return [`${min} ${hourRange(card.fromHour, card.toHour)} ${dow}`];
    }
    // Always emit an explicit "a-b/n" range with a step: a bare "8/2" means
    // "8 through 23 step 2" to cron, not the single hour 8.
    const hr =
      card.fromHour <= 0 && card.toHour >= 23
        ? card.every > 1
          ? `*/${card.every}`
          : '*'
        : `${card.fromHour}-${card.toHour}/${card.every}`;
    return [`0 ${hr} ${dow}`];
  }

  const byMinute = new Map<number, number[]>();
  for (const t of card.times) {
    const [h, m] = t.split(':').map(Number);
    // Guard against cleared/half-typed time inputs ("" or "12" leave m
    // undefined, and Number.isNaN(undefined) is false).
    if (!Number.isInteger(h) || !Number.isInteger(m)) continue;
    if (h < 0 || h > 23 || m < 0 || m > 59) continue;
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

// ----------------------------------------------------------------------------
// Backward compatibility with firmware that predates rotate_cron and still uses
// a single rotate_interval (seconds).
// ----------------------------------------------------------------------------

// Convert a legacy rotation interval (seconds) into a single 3-field cron rule.
// Mirrors the firmware's cron_from_legacy_interval mapping.
export function intervalToCron(seconds: number): string {
  if (seconds >= 3600 && seconds % 3600 === 0) {
    const hours = seconds / 3600;
    if (hours <= 1) return '0 * *';
    if (hours >= 24) return '0 0 *';
    return `0 */${hours} *`;
  }
  if (seconds >= 60 && 3600 % seconds === 0) {
    // Integer division, mirroring the firmware's cron_from_legacy_interval:
    // seconds/60 can be fractional (e.g. 90s -> 1.5), which is invalid cron.
    return `*/${Math.floor(seconds / 60)} * *`;
  }
  return '0 * *';
}

// Best-effort inverse: if the schedule is a single every-day interval rule,
// return the equivalent seconds so old firmware (which only understands
// rotate_interval) can still be driven. Returns null for anything that can't be
// represented as a fixed interval.
export function cronToInterval(rules: string[]): number | null {
  if (!rules || rules.length !== 1) return null;
  const f = rules[0].trim().split(/\s+/);
  if (f.length !== 3) return null;
  const [min, hour, dow] = f;
  if (dow !== '*') return null;
  let m = min.match(/^\*\/(\d+)$/);
  if (m && hour === '*') return parseInt(m[1], 10) * 60;
  m = hour.match(/^\*\/(\d+)$/);
  if (min === '0' && m) return parseInt(m[1], 10) * 3600;
  if (min === '0' && hour === '*') return 3600;
  if (min === '0' && hour === '0') return 86400;
  return null;
}

function cronDowToDays(field: string): DaysSpec {
  if (field === '*') return 'everyday';
  if (field === '1-5') return 'weekdays';
  if (field === '0,6' || field === '6,0') return 'weekends';
  const set = parseField(field, 0, 7, true);
  if (!set || set.size === 7) return 'everyday';
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
  const rangeHour = hourF.match(/^(\d+)-(\d+)$/);

  // Minutes interval: "*/n" (or plain "*" = every minute) with an hour window
  if ((stepMin || minF === '*') && (hourF === '*' || rangeHour)) {
    card.mode = 'interval';
    card.unit = 'minutes';
    card.every = stepMin ? parseInt(stepMin[1], 10) : 1;
    if (rangeHour) {
      card.fromHour = parseInt(rangeHour[1], 10);
      card.toHour = parseInt(rangeHour[2], 10);
    } else {
      card.fromHour = 0;
      card.toHour = 23;
    }
    return card;
  }

  // Hours interval: "0 * *" (hourly), "0 */n *", "0 a-b/n *" or "0 a-b *"
  if (minF === '0' && (hourF === '*' || stepHour || rangeHour)) {
    card.mode = 'interval';
    card.unit = 'hours';
    if (stepHour) {
      card.every = parseInt(stepHour[3], 10);
      card.fromHour = stepHour[1] !== undefined ? parseInt(stepHour[1], 10) : 0;
      card.toHour = stepHour[2] !== undefined ? parseInt(stepHour[2], 10) : 23;
    } else {
      card.every = 1;
      if (rangeHour) {
        card.fromHour = parseInt(rangeHour[1], 10);
        card.toHour = parseInt(rangeHour[2], 10);
      } else {
        card.fromHour = 0;
        card.toHour = 23;
      }
    }
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
