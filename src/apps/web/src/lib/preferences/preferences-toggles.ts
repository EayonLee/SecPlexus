/**
 * 布局偏好面板的切换组：顺序、标题与每个取值的文案都在这里。
 * 这是纯数据模块——不引入 React 或 next/font——因此单元测试可以直接断言它，
 * 而不用先把整个渲染树（含字体、next/font）加载进来。
 *
 * 取值集合必须与 PREFERENCE_REGISTRY 一致，由 tests/unit/preferences-toggles.test.ts 守住。
 */
export const PREFERENCE_TOGGLES = [
  {
    preference: "theme_mode",
    label: "themeMode",
    options: [
      { value: "light", label: "light" },
      { value: "dark", label: "dark" },
      { value: "system", label: "system" },
    ],
  },
  {
    preference: "content_layout",
    label: "pageLayout",
    options: [
      { value: "centered", label: "centered" },
      { value: "full-width", label: "fullWidth" },
    ],
  },
  {
    preference: "navbar_style",
    label: "navbarBehavior",
    options: [
      { value: "sticky", label: "sticky" },
      { value: "scroll", label: "scroll" },
    ],
  },
  {
    preference: "sidebar_variant",
    label: "sidebarStyle",
    options: [
      { value: "inset", label: "inset" },
      { value: "sidebar", label: "sidebar" },
      { value: "floating", label: "floating" },
    ],
  },
  {
    preference: "sidebar_collapsible",
    label: "collapseMode",
    options: [
      { value: "icon", label: "icon" },
      { value: "offcanvas", label: "offCanvas" },
    ],
  },
] as const;
