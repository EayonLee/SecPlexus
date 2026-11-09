import { describe, expect, test } from "vitest";

import { zDisplayName, zEmail, zPassword, zUsername } from "../../src/spx/api/iam/schemas";

// 这些样例集与 Go 侧 identity 包的测试用例一一对应。
// 任何一端放宽或收紧规则，此测试都会失败，从而阻止前后端判定分歧。

const usernames = [
  // [输入, 期望通过]
  ["ab", false],
  ["abc", true],
  [`a${"b".repeat(38)}`, true],
  [`a${"b".repeat(39)}`, false],
  ["1abc", false],
  ["_abc", false],
  ["-abc", false],
  [".abc", false],
  ["abc-", false],
  ["abc.", false],
  ["a-b", true],
  ["a_b", true],
  ["a.b", true],
  ["a--b", false],
  ["a__b", false],
  ["a..b", false],
  ["a.-b", false],
  ["a b", false],
  ["用户abc", false],
  ["a@b", false],
  ["AbC", true],
  ["  abc  ", false],
  ["", false],
] as const;

const emails = [
  ["", true],
  ["analyst@example.com", true],
  ["analyst+soc@example.com", true],
  ["a.b@mail.example.co", true],
  [`${"a".repeat(64)}@example.com`, true],
  [`${"a".repeat(65)}@example.com`, false],
  ["analyst.example.com", false],
  ["@example.com", false],
  ["analyst@", false],
  ["Analyst <analyst@example.com>", false],
  ["ana lyst@example.com", false],
  ["a@example.com, b@example.com", false],
  ["analyst@example.com.", false],
] as const;

const passwords = [
  ["12345678901", false],
  ["123456789012", true],
  ["admin", false],
  ["这是一个足够长的安全密码", true],
  ["a".repeat(128), true],
  ["a".repeat(129), false],
  ["", false],
] as const;

const displayNames = [
  ["a", true],
  ["张三", true],
  ["张".repeat(100), true],
  ["张".repeat(101), false],
  ["", false],
  ["   ", false],
] as const;

describe("前后端校验规则一致性", () => {
  test("用户名", () => {
    for (const [value, valid] of usernames) {
      expect(zUsername.safeParse(value).success, `username ${JSON.stringify(value)}`).toBe(valid);
    }
  });

  test("邮箱", () => {
    for (const [value, valid] of emails) {
      expect(zEmail.safeParse(value).success, `email ${JSON.stringify(value)}`).toBe(valid);
    }
  });

  test("密码", () => {
    for (const [value, valid] of passwords) {
      expect(zPassword.safeParse(value).success, `password length ${value.length}`).toBe(valid);
    }
  });

  test("显示名称", () => {
    for (const [value, valid] of displayNames) {
      expect(zDisplayName.safeParse(value).success, `display name length ${[...value].length}`).toBe(valid);
    }
  });

  test("显示名称被 trim", () => {
    const parsed = zDisplayName.safeParse("  张三  ");
    expect(parsed.success).toBe(true);
    expect(parsed.success && parsed.data).toBe("张三");
  });
});
