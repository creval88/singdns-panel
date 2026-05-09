# v20260510-beta2-hotfix33

本版本修复系统设置中安装 Sing-box / MosDNS 时出现 `sudo: a terminal is required` 或 `sudo: a password is required` 的问题。

## 修复
- 组件安装不再在后端拼接大段含 `sudo` 的 shell。
- 新增固定 root helper：
  - `/usr/local/bin/singdns-panel-install-singbox.sh`
  - `/usr/local/bin/singdns-panel-enable-ip-forward.sh`
  - `/usr/local/bin/singdns-panel-install-mosdns.sh`
  - `/usr/local/bin/singdns-panel-install-mosdns-upload.sh`
- sudoers 精确放行这些 helper，面板服务以 `panel` 用户运行时也可以非交互安装组件。
- 修复在线安装 Sing-box、在线安装 MosDNS、开启 IP 转发、离线上传 MosDNS 的同类提权问题。
- 安装错误提示增加 sudoers 未放行的诊断建议。

## 升级后操作
升级到本版本后，重新进入 **系统设置 -> 组件安装**，再点击安装 Sing-box / MosDNS。
