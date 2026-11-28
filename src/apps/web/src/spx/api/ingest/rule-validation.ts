import { z } from "zod";

export const ruleTextLimits = { name: 100, vendor: 120, product: 120, description: 1000 };
export type RuleTextField = keyof typeof ruleTextLimits;

function ruleText(field: RuleTextField) {
  return z.string().transform((input, ctx) => {
    const multiline = field === "description";
    const text = multiline ? input.replace(/\r\n?/g, "\n") : input;
    const prohibited = multiline ? /(?![\n\t])[\p{Cc}\p{Zl}\p{Zp}]/u : /[\p{Cc}\p{Zl}\p{Zp}]/u;
    const whitespace = /\p{White_Space}/u;
    let first = 0;
    let last = text.length;
    while (first < last && whitespace.test(text[first])) first++;
    while (last > first && whitespace.test(text[last - 1])) last--;
    const clean = text.slice(first, last);
    const limit = ruleTextLimits[field];
    const tooLong = clean.length > limit && (clean.length > 2 * limit || [...clean].length > limit);
    const error = prohibited.test(text) ? "invalid_characters" : !clean ? "required" : tooLong ? "too_long" : undefined;
    if (!error) return clean;
    ctx.addIssue({ code: "custom", message: error });
    return z.NEVER;
  });
}

export const ruleMetadata = z.object({
  name: ruleText("name"),
  vendor: ruleText("vendor"),
  product: ruleText("product"),
  description: ruleText("description"),
});

export const ruleTextFields = ruleMetadata.keyof();

const jsonObject = z.object({});

/** Checks the browser-visible shape. Ingest still decides whether the object yields fields. */
export function isJSONLogSample(sample: string) {
  try {
    return jsonObject.safeParse(JSON.parse(sample)).success;
  } catch {
    return false;
  }
}
