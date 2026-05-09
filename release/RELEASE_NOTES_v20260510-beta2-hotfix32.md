# v20260510-beta2-hotfix32

本版本修复一键安装命令依赖 `raw.githubusercontent.com/main` 时可能读到 CDN 旧缓存的问题。

## 修复
- 一键安装脚本默认 manifest 改为 GitHub Release 最新资产：`releases/latest/download/latest.json`。
- 文档推荐命令改为下载 Release 资产里的 `install-from-github.sh`，避免 raw main 短时间缓存导致仍安装旧版本。
- 新生成的面板默认更新源也改为 `releases/latest/download/latest.json`。
- 保留 hotfix31 的已有服务检测：已安装面板时执行 `upgrade.sh`，确保服务重启到新版本。

## 推荐安装/升级命令
```bash
curl -fsSL https://github.com/creval88/singdns-panel/releases/latest/download/install-from-github.sh | sudo bash
```

如果刚执行过旧命令后仍显示旧版本，也可以先手动重启：

```bash
sudo systemctl restart singdns-panel
```
