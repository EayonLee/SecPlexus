import { TZDate } from "@date-fns/tz";
import { addDays } from "date-fns";

export function auditDateBounds(from: string | undefined, to: string | undefined, timeZone: string) {
  function midnight(day: string) {
    const [year, month, date] = day.split("-").map(Number);
    return new TZDate(year, month - 1, date, timeZone);
  }

  return {
    from: from ? new Date(+midnight(from)).toISOString() : undefined,
    to: to ? new Date(+addDays(midnight(to), 1)).toISOString() : undefined,
  };
}
