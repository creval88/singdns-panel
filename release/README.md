# 发布包说明

## 生成发布包
在源码目录执行：

```bash
bash release/build-release.sh
```

可选传入版本号：

```bash
bash release/build-release.sh v0.1.0
```

构建时会把这个版本号注入二进制，仪表盘可直接显示。

生成物位于：

```bash
dist/singdns-panel-release/
dist/singdns-panel-<version>-<arch>.tar.gz
```

## 安装
把压缩包传到 Debian 后：

```bash
tar xzf singdns-panel-<version>-<arch>.tar.gz
cd singdns-panel-release
sudo bash install.sh
```

## 升级
把新版本发布包解压后，在目录内执行：

```bash
sudo bash upgrade.sh
```

也可以把发布目录直接放到目标机的 `/opt/singdns-panel/updates/<版本目录>/`，再由面板内触发升级。
该模式不依赖 GitHub，只依赖本地发布包目录。

## 卸载
```bash
sudo bash uninstall.sh
```

默认会保留 `/opt/singdns-panel/app/configs` 和 `/opt/singdns-panel/app/logs`。
