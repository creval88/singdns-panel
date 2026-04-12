# v20260411-beta2-hotfix27

## 本次更新

### System / 网络设置
- 新增更安全的 network settings backend
- 支持 `ifupdown` 后端
- 支持 `dns-only` 模式
- 对相同配置跳过重启，避免无意义断网
- apply 失败自动回滚
- 持久化 `last-good` 回滚点

### Sing-box / 升级与配置
- 支持手动上传 core 并自动解压安装
- 修正上传 core 安装路径与 sudoers 对齐
- 修正 sing-box 默认路径与部署脚本保持一致
- 订阅视图拆分，保留手动导入状态
- 增加 IP forwarding 检查与更诚实的 monitor mode
- 保留 manual nodes draft，并与订阅节点合并展示

### 发布与安装链路
- beta manifest 更新到 `20260411-beta2-hotfix27`
- 一键安装脚本支持按 `channel + arch` 拉取对应发布包
- 发布包内包含：
  - `install.sh`
  - `upgrade.sh`
  - `uninstall.sh`
  - `panel.json`
  - `sudoers.singdns-panel`
  - `singdns-panel.service`
  - `bin/singdns-panel`

## 安装

### 新设备一键安装
```bash
curl -fsSL https://raw.githubusercontent.com/creval88/singdns-panel/main/scripts/install-from-github.sh | sudo bash
```

### 指定 stable / arm64
```bash
curl -fsSL https://raw.githubusercontent.com/creval88/singdns-panel/main/scripts/install-from-github.sh \
  | sudo CHANNEL=stable ARCH=arm64 bash
```

## 已核对
- `updates/latest.json` 已指向当前 beta release
- release asset 下载链路可达
- 发布包内关键安装文件齐全
- 一键安装脚本语法正常

## 注意
- stable 频道目前仍指向旧版本；当前最新更新主要在 beta
- 若需 100% 闭环确认，建议再做一次干净 Debian 环境实装验证
