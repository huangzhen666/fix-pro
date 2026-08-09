# FixPro Admin Web

React 19 + TypeScript + Vite + Ant Design 管理后台。

```powershell
npm install
npm run admin:dev
```

开发环境把 `/api` 代理至 `http://localhost:8080`。当前 Bootstrap Basic Auth 只用于工程连通验证，正式身份模块完成后替换为短期 Access Token + HttpOnly Refresh Token。

页面按路由懒加载；服务端状态使用 TanStack Query，少量会话状态使用 Zustand。后续 API 类型应由后端 OpenAPI 生成，避免重复维护 DTO。
