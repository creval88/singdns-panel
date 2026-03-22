# SingDNS Panel

Go 内网管理面板 MVP：管理 sing-box / mosdns。

## 当前已完成
- 登录页 / session
- Dashboard
- Sing-box 页面：状态、配置编辑、订阅、服务控制
- MosDNS 页面：状态、服务控制、自带面板跳转
- 日志页面
- 基础 API 路由
- systemd / journalctl 服务层
- `sbctl.sh` / `mdctl.sh` 非交互控制脚本
- 部署模板与 sudoers 模板

## 本地运行
```bash
cp configs/panel.example.json configs/panel.json
# 生成 bcrypt hash 后替换 password_hash
go run ./cmd/server hash-password 'your-password'
go mod tidy
go run ./cmd/server
```

默认监听 `:9999`。

## Debian 部署
看：`deploy/DEPLOY.md`

## 一键安装脚本
提供了：`deploy/install.sh`

在项目目录内执行：
```bash
sudo bash deploy/install.sh
```

它会：
- 安装 Go / rsync / sudo（如缺失）
- 复制项目到 `/opt/singdns-panel/app`
- 生成默认 `configs/panel.json`（如不存在）
- 编译单个二进制 `/opt/singdns-panel/singdns-panel`
- 安装 sudoers 与 systemd 服务
- 自动启动面板

## 新设备一键安装（从 GitHub 下载最新版本）
适用于**全新 Debian 设备**，无需先 clone 仓库：

```bash
curl -fsSL https://raw.githubusercontent.com/creval88/singdns-panel/main/scripts/install-from-github.sh | sudo bash
```

可选参数（按需）：

```bash
# stable 渠道 + arm64
curl -fsSL https://raw.githubusercontent.com/creval88/singdns-panel/main/scripts/install-from-github.sh \
  | sudo CHANNEL=stable ARCH=arm64 bash
```

脚本默认行为：
- 自动识别架构（amd64/arm64）
- 从 `updates/latest.json` 读取指定 channel+arch 的发布包
- 下载并校验 sha256（manifest 提供时）
- 解压后执行发布包内 `install.sh`

## 发布包
已增加 release 流程：
```bash
bash release/build-release.sh
```

会生成：
- `dist/singdns-panel-release/`
- `dist/singdns-panel-<version>-<arch>.tar.gz`

发布包内含：
- 预编译二进制（当前机器有 Go 时）
- `install.sh`
- `upgrade.sh`
- `uninstall.sh`
- 默认 `panel.json`
- systemd / sudoers / sbctl / mdctl

## 面板自升级（本地发布包模式）
现在支持**无 GitHub 依赖**的基础升级流：

1. 在构建机执行：`bash release/build-release.sh v0.1.0`
2. 把生成的发布包解压到目标机的 `/opt/singdns-panel/updates/<版本目录>/`
3. 在 `configs/panel.json` 里配置：
   - `panel_update.release_dir`: 本地升级包目录
   - `panel_update.upgrade_command`: 可选，自定义升级命令；留空则默认执行 `<release>/upgrade.sh`
4. 面板中的 **Sing-box → 版本与升级** 会显示检测结果，并可执行升级

默认 sudoers 已允许执行 `/opt/singdns-panel/updates/*/upgrade.sh`。

## 当前限制
- 远程拉取升级包、签名校验、灰度/回滚仍未实现
- 需要在 Debian 实机验证 panel upgrade 流程
