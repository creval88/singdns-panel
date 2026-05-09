# v20260510-beta2-hotfix30

本版本修复系统设置页内联 JavaScript 语法错误导致的按钮无响应问题。

## 修复
- 修复系统页安装错误提示正则中 `i/o timeout` 未转义 `/` 导致整段脚本无法解析的问题。
- 受影响按钮包括：修改登录密码、安装/更新 Sing-box、安装/更新 MosDNS、面板升级、网络设置等系统页页面级操作。
- 增加模板内联 JavaScript 语法测试，覆盖 System、Sing-box、MosDNS、Dashboard、Logs、Config Center 等页面，避免同类问题再次进入发布包。

## 一键安装
```bash
curl -fsSL https://raw.githubusercontent.com/creval88/singdns-panel/main/scripts/install-from-github.sh | sudo bash
```

## 已知说明
- Sing-box / MosDNS 独立页面脚本语法本次验证通过。
- 如果浏览器仍加载旧 JS/HTML，请强制刷新页面或清理浏览器缓存。
