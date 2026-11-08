/**
 * 展示时区解析。服务端始终以 UTC 存储和传输时间，时区只影响渲染。
 *
 * 语义值 `browser` 表示“跟随访问者浏览器”，由运行时解析为具体时区；
 * `utc` 是用户显式选择，不属于回落值。
 */

const semanticTimeZones = new Set(["utc", "browser"]);

function isValidIanaTimeZone(value: string): boolean {
  try {
    // Intl 在遇到未知时区时抛 RangeError，这是运行时唯一的权威判定。
    new Intl.DateTimeFormat("en-US", { timeZone: value });
    return true;
  } catch {
    return false;
  }
}

/** 判断值是否是服务端会接受的时区表达。 */
export function isSupportedTimeZone(value: string | null | undefined): boolean {
  if (value === null || value === undefined) {
    return false;
  }

  if (value === "") {
    return true;
  }

  return semanticTimeZones.has(value) || isValidIanaTimeZone(value);
}

/** 归一化为传给 next-intl 的具体时区名。 */
export function resolveTimeZone(input: {
  userTimeZone: string | null | undefined;
  browserTimeZone: string | null | undefined;
}): string {
  const { userTimeZone, browserTimeZone } = input;

  const usable = (value: string | null | undefined): string | null =>
    typeof value === "string" && value !== "" && isValidIanaTimeZone(value) ? value : null;

  if (typeof userTimeZone === "string" && userTimeZone !== "") {
    if (userTimeZone === "utc") {
      return "UTC";
    }

    if (userTimeZone === "browser") {
      return usable(browserTimeZone) ?? "UTC";
    }

    const explicit = usable(userTimeZone);
    if (explicit) {
      return explicit;
    }
  }

  return usable(browserTimeZone) ?? "UTC";
}
