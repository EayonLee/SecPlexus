import { describe, expect, test, vi } from "vitest";

// 字体注册表会加载 next/font，node 环境解析不了；这里只关心取值集合，替换成等价的键表。
vi.mock("@/lib/fonts/registry", () => ({ fontKeys: ["geist", "inter"], fontOptions: [], fontVars: "" }));

import { PREFERENCE_REGISTRY, type PreferenceKey } from "../../src/lib/preferences/preferences-config";
import { PREFERENCE_TOGGLES } from "../../src/lib/preferences/preferences-toggles";

// 切换组声明顺序与文案，取值集合的唯一事实源仍是 PREFERENCE_REGISTRY。
// 有了这两条断言，新增或删除布局偏好时两边不会静默漂移。
describe("preference toggles", () => {
  // 取值集合必须一致；渲染顺序由面板决定（侧栏样式把「嵌入式」排在最前），不要求与注册表同序。
  test("每张切换组的取值与偏好注册表一致", () => {
    for (const toggle of PREFERENCE_TOGGLES) {
      const registered: readonly string[] = PREFERENCE_REGISTRY[toggle.preference].values;
      expect([...toggle.options.map((option) => option.value)].sort(), toggle.preference).toEqual(
        [...registered].sort(),
      );
    }
  });

  test("除字体外的每个布局偏好都有切换组", () => {
    const covered = new Set<string>(PREFERENCE_TOGGLES.map((toggle) => toggle.preference));
    const missing = (Object.keys(PREFERENCE_REGISTRY) as PreferenceKey[]).filter(
      (key) => key !== "font" && !covered.has(key),
    );
    expect(missing).toEqual([]);
  });
});
