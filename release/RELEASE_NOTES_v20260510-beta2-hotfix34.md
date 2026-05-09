# v20260510-beta2-hotfix34

本版本修复 MosDNS 安装后误报成功但服务未运行、手动启动只显示 `exit status 5` 而缺少诊断信息的问题。

## 修复
- MosDNS 在线安装和离线上传安装会在返回成功前校验 `mosdns.service` 是否仍处于 active。
- 安装脚本会检查 `/cus/mosdns/config_custom.yaml` 是否存在，缺失时直接失败并列出配置目录文件。
- MosDNS 启动失败时，安装脚本会输出 `systemctl status mosdns` 和最近 journal 日志。
- 面板手动启动/停止/重启服务失败时，返回 stderr、systemd status 和 journal 片段，避免只看到 `exit status 5`。

## 升级后操作
升级到本版本后，重新点击 **系统设置 -> 安装 / 更新 MosDNS**。如果仍失败，错误详情中应包含具体的 systemd 或 MosDNS 配置错误。
