# 前端规范

## 技术栈

- 框架与语言：按项目实际情况填写（例如 Vue 3 + TypeScript）。
- 样式：Tailwind CSS；组件库：按项目实际情况填写。

## 约束

- 原型在 `doc/prototype/`，生产组件在 `src/`；原型是骨架，不是最终代码。
- 必须去 Mock：原型里的写死数据要改调真实接口，并补齐 Loading、空态与错误态。
- 关键按钮必须加防抖或提交 Loading 状态，防止重复提交。

## 命名

- 组件：PascalCase；页面：`src/views/`；可复用组件：`src/components/`。
