---
date: 2026-04-26
topic: tab-number-shortcuts
---

# Tab Number Shortcuts

## Problem Frame
当前标签页只能通过 Tab 键循环或鼠标点击切换，用户无法快速跳转到目标标签页。

## Requirements

**键盘快捷键**
- R1. 按数字键 1-4 直接跳转到对应标签页（1=Slots, 2=Subscriptions, 3=Settings, 4=Logs）
- R2. 当子组件（如输入框、overlay）激活时，数字键不应触发标签切换

**标签栏显示**
- R3. 标签名前显示数字前缀，格式如「1 Slots」「2 Subscriptions」「3 Settings」「4 Logs」

## Success Criteria
- 按数字键能立即跳转到对应标签页
- 标签栏显示数字提示，用户一眼可知快捷键

## Scope Boundaries
- 不改变现有 Tab 键循环行为
- 不改变鼠标点击切换行为

## Next Steps
-> `/ce:plan` for structured implementation planning
