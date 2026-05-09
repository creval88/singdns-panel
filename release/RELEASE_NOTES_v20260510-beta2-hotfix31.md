# v20260510-beta2-hotfix31

本版本修复一键安装命令用于更新已有面板时，磁盘二进制已替换但 systemd 仍运行旧进程，导致页面版本号仍显示上一版的问题。

## 修复
- `scripts/install-from-github.sh` 检测到已有 `singdns-panel` systemd 服务时，改为执行发布包内 `upgrade.sh`，确保更新后服务重启。
- 发布包 `install.sh` 在已有服务运行时会执行 `systemctl restart singdns-panel`，不再只执行 `systemctl enable --now`。
- 安装/升级后会检查服务是否处于 active 状态。

## 立即处理已遇到的问题
如果你已经执行过 hotfix30 的一键命令但页面还显示 29，可以先手动重启：

```bash
sudo systemctl restart singdns-panel
```

然后刷新页面。也可以直接重新执行一键安装命令升级到 hotfix31。
