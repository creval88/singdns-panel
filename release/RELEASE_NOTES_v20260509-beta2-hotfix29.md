# v20260509-beta2-hotfix29

本版本面向 Debian 测试机安装验证，重点是提升面板安全默认值、远程升级可靠性、配置中心保存保护，以及一键安装发布链路。

## 主要更新
- 默认初始化配置改为生成随机 session key 和随机初始管理员密码，并输出一次性登录信息。
- 强化登录 session 过期处理和 CSRF 同源校验。
- 配置中心保存接口返回备份、风险、校验和变更摘要，前端保存前会先校验并提示高风险改动。
- Dashboard 增加配置有效性、备份数量、Clash API 状态、CPU/内存等健康信息。
- 面板本地/远程升级增加 preflight 检查、步骤日志和失败详情，减少静默失败。
- 修正 MosDNS 安装权限，避免上传目录使用过宽权限。
- 增加 `internal/webassets` 与 `web` 镜像一致性检查脚本。

## 发布包
- `singdns-panel-20260509-beta2-hotfix29-amd64.tar.gz`
- `singdns-panel-20260509-beta2-hotfix29-arm64.tar.gz`
- `updates/latest.json` 的 beta 渠道已指向本版本。

## 一键安装
```bash
curl -fsSL https://raw.githubusercontent.com/creval88/singdns-panel/main/scripts/install-from-github.sh | sudo bash
```

指定架构示例：
```bash
curl -fsSL https://raw.githubusercontent.com/creval88/singdns-panel/main/scripts/install-from-github.sh \
  | sudo CHANNEL=beta ARCH=amd64 bash
```

安装完成后注意查看终端输出的初始管理员密码。
