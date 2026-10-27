---
name: spx-arco-design-radio
description: "Arco Design Radio component API. Use for single selection, Radio.Group, button-style radio, and vertical radio groups."
user-invocable: false
---

# Radio 单选框

```tsx
import { Radio } from '@arco-design/web-react';

// 单选组
<Radio.Group defaultValue="a" onChange={(value) => console.log(value)}>
  <Radio value="a">选项A</Radio>
  <Radio value="b">选项B</Radio>
  <Radio value="c">选项C</Radio>
</Radio.Group>

// 按钮样式
<Radio.Group type="button" defaultValue="a">
  <Radio value="a">选项A</Radio>
  <Radio value="b">选项B</Radio>
  <Radio value="c">选项C</Radio>
</Radio.Group>

// 使用 options
<Radio.Group
  type="button"
  options={['北京', '上海', '广州']}
/>
```

## Radio.Group Props

| 属性 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `value` / `defaultValue` | `any` | — | 选中值 |
| `type` | `'radio' \| 'button'` | `'radio'` | 样式类型 |
| `size` | `'mini' \| 'small' \| 'default' \| 'large'` | — | 尺寸（button 模式） |
| `direction` | `'horizontal' \| 'vertical'` | `'horizontal'` | 排列方向 |
| `options` | `(string \| { label, value, disabled })[]` | — | 选项列表 |
| `onChange` | `(value: any) => void` | — | 变化回调 |
