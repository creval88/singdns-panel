# SingDNS Panel 部署说明

## 快速方式
如果源码已经在机器上，直接执行：
```bash
sudo bash deploy/install.sh
```

下面是手动部署步骤。

## 1. 安装 Go
```bash
apt update
apt install -y golang-go sudo
```

## 2. 创建用户
```bash
useradd -r -s /usr/sbin/nologin -d /opt/singdns-panel panel || true
mkdir -p /opt/singdns-panel
chown -R panel:panel /opt/singdns-panel
```

## 3. 部署代码
```bash
cp -r singdns-panel /opt/singdns-panel/app
cd /opt/singdns-panel/app
mkdir -p logs
cp configs/panel.example.json configs/panel.json
```

## 4. 生成密码哈希
```bash
go run ./cmd/server hash-password '你的密码'
```
把输出填到 `configs/panel.json` 的 `password_hash`。

## 5. 安装控制脚本
```bash
install -m 755 scripts/sbctl.sh /usr/local/bin/sbctl.sh
install -m 755 scripts/mdctl.sh /usr/local/bin/mdctl.sh
```

## 6. 配 sudoers
```bash
cp deploy/sudoers.singdns-panel /etc/sudoers.d/singdns-panel
chmod 440 /etc/sudoers.d/singdns-panel
visudo -c
```

## 7. 编译
```bash
cd /opt/singdns-panel/app
go mod tidy
go build -o /opt/singdns-panel/singdns-panel ./cmd/server
chown panel:panel /opt/singdns-panel/singdns-panel
```

## 8. 安装 systemd
将 `deploy/singdns-panel.service` 复制到：
`/etc/systemd/system/singdns-panel.service`

然后：
```bash
systemctl daemon-reload
systemctl enable --now singdns-panel
systemctl status singdns-panel --no-pager
```

## 9. 访问
```txt
http://10.0.0.8:9999
```

## 10. 建议
- 仅内网开放 9999
- 若要外网访问，务必反代并加 TLS
- 建议配防火墙仅允许局域网
